package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startNode runs a real KV node (kvHandler + temp SQLite) on a random port.
// It returns the server and its address without "http://" (e.g. "127.0.0.1:54321"),
// which is the format newRouter and NODES expect.
func startNode(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	handler, err := openTestStore(t, filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, strings.TrimPrefix(srv.URL, "http://")
}

// startCluster starts n KV nodes and a router in front of them.
func startCluster(t *testing.T, n int) ([]*httptest.Server, []string, *router) {
	t.Helper()
	var servers []*httptest.Server
	var addrs []string
	for i := 0; i < n; i++ {
		srv, addr := startNode(t)
		servers = append(servers, srv)
		addrs = append(addrs, addr)
	}
	rt, err := newRouter(addrs, 150)
	if err != nil {
		t.Fatal(err)
	}
	return servers, addrs, rt
}

// do sends one request through a handler (here, the router) and records the response.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// directStatus asks a KV node for a key directly, bypassing the router.
func directStatus(t *testing.T, addr, key string) int {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/" + key)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// TestRouterRoutesToOwner: every key is stored on exactly the node the ring picks.
func TestRouterRoutesToOwner(t *testing.T) {
	_, addrs, rt := startCluster(t, 3)
	perNode := map[string]int{}
	for i := 0; i < 100; i++ {
		key := "key_" + strconv.Itoa(i)
		owner := rt.ring.Get(key)

		if rec := do(rt, "PUT", "/"+key, `{"value":"v"}`); rec.Code != http.StatusNoContent {
			t.Fatalf("PUT %s: got %d, want 204", key, rec.Code)
		}
		rec := do(rt, "GET", "/"+key, "")
		if rec.Code != http.StatusOK || rec.Body.String() != "{\"value\":\"v\"}\n" {
			t.Fatalf("GET %s: got %d %q", key, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-KV-Node"); got != owner {
			t.Fatalf("%s: X-KV-Node = %q, want %q", key, got, owner)
		}
		// Check the databases themselves: only the owner has the key.
		for _, addr := range addrs {
			want := http.StatusNotFound
			if addr == owner {
				want = http.StatusOK
			}
			if got := directStatus(t, addr, key); got != want {
				t.Fatalf("%s on %s: got %d, want %d", key, addr, got, want)
			}
		}
		perNode[owner]++
	}
	t.Logf("keys per node: %v", perNode)
}

// TestRouterBadRequests: empty key → 400, empty ring → 503.
func TestRouterBadRequests(t *testing.T) {
	_, addrs, rt := startCluster(t, 1)
	if rec := do(rt, "GET", "/", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty key: got %d, want 400", rec.Code)
	}
	rt.ring.Remove(addrs[0])
	if rec := do(rt, "GET", "/x", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("empty ring: got %d, want 503", rec.Code)
	}
}

// TestRouterNodeDown: a dead node gives 502 until it's removed from the ring,
// then its keys are routed to a live node. The health checker will automate the Remove.
func TestRouterNodeDown(t *testing.T) {
	servers, addrs, rt := startCluster(t, 3)

	// Find a key owned by node 0 (about 1 in 3 keys is).
	var key string
	for i := 0; ; i++ {
		key = "key_" + strconv.Itoa(i)
		if rt.ring.Get(key) == addrs[0] {
			break
		}
	}

	servers[0].Close() // node 0 goes down
	if rec := do(rt, "GET", "/"+key, ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("dead node: got %d, want 502", rec.Code)
	}

	rt.ring.Remove(addrs[0]) // what the health checker will do
	if rec := do(rt, "PUT", "/"+key, `{"value":"moved"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT after remove: got %d, want 204", rec.Code)
	}
	rec := do(rt, "GET", "/"+key, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"value\":\"moved\"}\n" {
		t.Fatalf("GET after remove: got %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-KV-Node"); got == addrs[0] || got == "" {
		t.Fatalf("key still routed to dead node: X-KV-Node = %q", got)
	}
}

// waitFor polls cond until it's true, failing the test after 2 seconds.
// The checker runs in the background, so the test can't know exactly when it acts.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRouterHealthCheck: a node failing /_health is removed from the ring,
// and added back once it answers again.
func TestRouterHealthCheck(t *testing.T) {
	// A fake node whose health we can switch on and off.
	var down atomic.Bool
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(flaky.Close)
	flakyAddr := strings.TrimPrefix(flaky.URL, "http://")
	_, healthyAddr := startNode(t)

	rt, err := newRouter([]string{flakyAddr, healthyAddr}, 150)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) }) // stop the checker when the test ends
	go rt.healthCheck(20*time.Millisecond, 2, stop)

	down.Store(true)
	waitFor(t, "flaky node removed", func() bool {
		return !slices.Contains(rt.ring.Nodes(), flakyAddr)
	})
	if got := rt.ring.Nodes(); !slices.Equal(got, []string{healthyAddr}) {
		t.Fatalf("ring after removal = %v, want only %s", got, healthyAddr)
	}

	down.Store(false)
	waitFor(t, "flaky node added back", func() bool {
		return slices.Contains(rt.ring.Nodes(), flakyAddr)
	})
}
