package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// router routes each request to the KV node that owns its key.
type router struct {
	ring    *Ring
	nodes   []string                          // every configured node, healthy or not
	proxies map[string]*httputil.ReverseProxy // node address ("kv1:8080") -> proxy to it
}

// newRouter builds a ring from the node addresses and one reverse proxy per node.
func newRouter(nodes []string, vnodes int) (*router, error) {
	// Go's defaults keep only 2 idle connections per host and 100 in total, so
	// under load the router keeps closing and reopening TCP connections to the
	// nodes (hundreds of sockets pile up in TIME_WAIT). Allow enough idle
	// connections that every in-flight request can reuse one.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 0 // 0 = no limit on the total; the per-host limit below applies
	transport.MaxIdleConnsPerHost = 256

	rt := &router{
		ring:    NewRing(vnodes),
		proxies: make(map[string]*httputil.ReverseProxy),
		nodes:   nodes,
	}
	for _, addr := range nodes {
		target, err := url.Parse("http://" + addr)
		if err != nil {
			return nil, err
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = transport
		rt.proxies[addr] = proxy
		rt.ring.Add(addr)
	}
	return rt, nil
}

// ServeHTTP forwards the request to the node that owns the key.
func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	switch r.URL.Path {
	case "/_health":
		w.WriteHeader(http.StatusOK)
		return
	case "/_ring":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]string{"nodes": rt.ring.Nodes()})
		return
	}

	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	node := rt.ring.Get(key)
	if node == "" {
		http.Error(w, "no nodes on the ring", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("X-KV-Node", node)
	rt.proxies[node].ServeHTTP(w, r)
}

// runRouter starts the router using the NODES env var, e.g. "kv1:8080,kv2:8080,kv3:8080".
func runRouter(addr string) {
	var nodes []string
	for _, n := range strings.Split(os.Getenv("NODES"), ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) == 0 {
		log.Fatal("NODES must list at least one KV node")
	}
	rt, err := newRouter(nodes, 150)
	if err != nil {
		log.Fatal(err)
	}
	go rt.healthCheck(time.Second, 3, nil) // remove a node after 3 failed checks
	log.Printf("Router listening on %s; nodes: %v", addr, nodes)
	log.Fatal(http.ListenAndServe(addr, rt))
}

// healthCheck pings every node's /_health every interval, forever.
// A node that fails failLimit checks in a row is removed from the ring,
// and it's added back as soon as a check succeeds.
func (rt *router) healthCheck(interval time.Duration, failLimit int, stop <-chan struct{}) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	fails := map[string]int{}   // node -> consecutive failed checks
	onRing := map[string]bool{} // node -> currently on the ring?
	for _, n := range rt.nodes {
		onRing[n] = true
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
			case <-stop:
				return
			case <-ticker.C:
		}
	
	for range ticker.C { // runs once per tick
		for _, node := range rt.nodes {
			if ping(client, node) {
				fails[node] = 0
				if onRing[node] == false {
					rt.ring.Add(node)
					onRing[node] = true
					log.Printf("node %s is back, added to ring", node)
				}
			} else {
				fails[node]++
				if onRing[node] == true && fails[node] >= failLimit {
					rt.ring.Remove(node)
					onRing[node] = false
					log.Printf("node %s is down, removed from ring", node)
				}
			}
		}
	}
}
}

// ping reports whether node answers GET /_health with 200.
func ping(client *http.Client, node string) bool {
	resp, err := client.Get("http://" + node + "/_health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
