package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type kvHandler struct {
	db *sql.DB
}

func newPersistentHandler(path string) (*kvHandler, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath // Windows drive letter in a file URI.
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	options := url.Values{}
	options.Add("_pragma", "busy_timeout(5000)")
	options.Add("_pragma", "journal_mode(DELETE)")
	options.Add("_pragma", "synchronous(EXTRA)")
	uri.RawQuery = options.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY NOT NULL, value TEXT NOT NULL)"); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize SQLite store: %w", err)
	}
	return &kvHandler{db: db}, nil
}

func (h *kvHandler) Close() error {
	return h.db.Close()
}

func (h *kvHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	log.Printf("%s %s", r.Method, key)
	databaseError := func(err error) {
		log.Printf("SQLite %s %q: %v", r.Method, key, err)
		http.Error(w, "database operation failed", http.StatusInternalServerError)
	}
	switch r.Method {
	case http.MethodGet:
		var value string
		err := h.db.QueryRowContext(r.Context(), "SELECT value FROM kv WHERE key = ?", key).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		if err != nil {
			databaseError(err)
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
		_, err := h.db.ExecContext(r.Context(), "INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, *body.Value)
		if err != nil {
			databaseError(err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		result, err := h.db.ExecContext(r.Context(), "DELETE FROM kv WHERE key = ?", key)
		if err != nil {
			databaseError(err)
			return
		}
		count, err := result.RowsAffected()
		if err != nil {
			databaseError(err)
			return
		}
		if count == 0 {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	path := os.Getenv("DATA_FILE")
	if path == "" {
		path = "data/store.db"
	}
	handler, err := newPersistentHandler(path)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Server listening on %s; SQLite data file: %s", addr, path)
	err = http.ListenAndServe(addr, handler)
	handler.Close()
	log.Fatal(err)
}
