package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTestStore(t *testing.T, path string) (*kvHandler, error) {
	t.Helper()
	handler, err := newPersistentHandler(path)
	if err == nil {
		t.Cleanup(func() { handler.Close() })
	}
	return handler, err
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "store.db")
	handler, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, body string, status int, want string) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/name", strings.NewReader(body)))
		if response.Code != status || (want != "" && response.Body.String() != want) {
			t.Fatalf("%s: status %d, body %q; want %d, %q", method, response.Code, response.Body.String(), status, want)
		}
	}
	for _, value := range []string{"initial", "updated", ""} {
		request("PUT", fmt.Sprintf(`{"value":%q}`, value), 204, "")
		if err := handler.Close(); err != nil {
			t.Fatal(err)
		}
		handler, err = openTestStore(t, path)
		if err != nil {
			t.Fatal(err)
		}
		request("GET", "", 200, fmt.Sprintf("{\"value\":%q}\n", value))
	}
	request("DELETE", "", 204, "")
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	handler, err = openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	request("GET", "", 404, "")
}

func TestPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	handler, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("PUT", "/name", strings.NewReader(`{"value":"original"}`)))
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
	// Force real SQLite write failures while keeping reads available.
	if _, err := handler.db.Exec("PRAGMA query_only = ON"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ method, key string }{{"PUT", "name"}, {"POST", "new"}, {"DELETE", "name"}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(step.method, "/"+step.key, strings.NewReader(`{"value":"changed"}`)))
		if response.Code != 500 {
			t.Fatalf("%s: got %d, want 500", step.method, response.Code)
		}
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	handler, err = openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	for key, status := range map[string]int{"name": 200, "new": 404} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/"+key, nil))
		if response.Code != status || (status == 200 && response.Body.String() != "{\"value\":\"original\"}\n") {
			t.Fatalf("failed write changed data: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestInvalidDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	for _, data := range []string{`not a SQLite database`, `corrupted database contents`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := openTestStore(t, path); err == nil {
			t.Fatalf("accepted invalid database: %s", data)
		}
	}
}

func TestConcurrentPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	handler, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("PUT", fmt.Sprintf("/key_%d", i), strings.NewReader(`{"value":"saved"}`)))
			if response.Code != 204 {
				t.Errorf("PUT: got %d", response.Code)
			}
		}(i)
	}
	wg.Wait()
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := openTestStore(t, path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		response := httptest.NewRecorder()
		reloaded.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/key_%d", i), nil))
		if response.Code != 200 || response.Body.String() != "{\"value\":\"saved\"}\n" {
			t.Fatalf("key_%d lost after restart: %d %s", i, response.Code, response.Body.String())
		}
	}
}

func TestOperations(t *testing.T) {
	handler, err := openTestStore(t, filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		method string
		path   string
		body   string
		status int
		want   string
	}{
		{"GET", "/name", "", 404, ""},
		{"PUT", "/name", `{"value":"Tien"}`, 204, ""},
		{"GET", "/name", "", 200, `{"value":"Tien"}` + "\n"},
		{"PUT", "/name", `{"value":"updated"}`, 204, ""},
		{"GET", "/name", "", 200, `{"value":"updated"}` + "\n"},
		{"DELETE", "/name", "", 204, ""},
		{"GET", "/name", "", 404, ""},
		{"DELETE", "/name", "", 404, ""},
		{"PUT", "/name", `not JSON`, 400, ""},
		{"PUT", "/name", `{}`, 400, ""},
		{"PUT", "/name", `{"value":null}`, 400, ""},
		{"PUT", "/name", `{"value":123}`, 400, ""},
		{"GET", "/name", "", 404, ""},
		{"POST", "/name", `{"value":""}`, 204, ""},
		{"GET", "/name", "", 200, `{"value":""}` + "\n"},
		{"GET", "/", "", 400, ""},
		{"PATCH", "/name", "", 405, ""},
	}
	for _, step := range steps {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(step.method, step.path, strings.NewReader(step.body)))
		if response.Code != step.status {
			t.Fatalf("%s %s: got status %d, want %d", step.method, step.path, response.Code, step.status)
		}
		if step.want != "" && response.Body.String() != step.want {
			t.Fatalf("%s %s: got body %q, want %q", step.method, step.path, response.Body.String(), step.want)
		}
	}
}

func TestConcurrentOperations(t *testing.T) {
	handler, err := openTestStore(t, filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, method := range []string{http.MethodPut, http.MethodGet, http.MethodDelete} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, "/shared", strings.NewReader(`{"value":"test"}`)))
				if response.Code != http.StatusOK && response.Code != http.StatusNoContent && response.Code != http.StatusNotFound {
					t.Errorf("%s: unexpected status %d", method, response.Code)
				}
			}
		}()
	}
	wg.Wait()
}
