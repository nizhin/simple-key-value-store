package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOperations(t *testing.T) {
	handler := newHandler()
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
	handler := newHandler()
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
