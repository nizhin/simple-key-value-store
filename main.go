package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

func newHandler() http.Handler {
	store := make(map[string]string)
	var mu sync.RWMutex

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/")
		if key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}
		log.Printf("%s %s", r.Method, key)

		switch r.Method {
		case http.MethodGet:
			mu.RLock()
			value, ok := store[key]
			mu.RUnlock()
			if !ok {
				http.Error(w, "key not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"value": value})

		case http.MethodPut, http.MethodPost:
			var body struct {
				Value *string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Value == nil {
				http.Error(w, "expected JSON with a string value", http.StatusBadRequest)
				return
			}
			mu.Lock()
			store[key] = *body.Value
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		case http.MethodDelete:
			mu.Lock()
			_, ok := store[key]
			delete(store, key)
			mu.Unlock()
			if !ok {
				http.Error(w, "key not found", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.Header().Set("Allow", "GET, PUT, POST, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func main() {
	log.Println("Server listening on http://127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", newHandler()))
}
