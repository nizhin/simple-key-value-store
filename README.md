# Simple key-value store
Run:
```sh
go run .
```

The server listens locally at http://127.0.0.1:8080. Keys and values are strings.

Store a value (also accepts POST for the assignment benchmark):
```sh
curl -i -X PUT http://127.0.0.1:8080/name \
  -H 'Content-Type: application/json' -d '{"value":"Anwar"}'
```

Get a value:
```sh
curl http://127.0.0.1:8080/name
# {"value":"Anwar"}
```

Delete a value (DEL uses HTTP DELETE):
```sh
curl -i -X DELETE http://127.0.0.1:8080/name
```

PUT and DELETE return 204 on success. Missing keys return 404; invalid input
returns 400; unsupported methods return 405.

Data stays in memory and is lost when the server stops. A mutex protects the
store during concurrent requests. Operations are logged with timestamps.
Press Ctrl+C to stop.

Run tests with the race detector:
```sh
go test -race ./...
```

To run the Python benchmark, keep the server running and use:
```sh
python3 benchmark.py
```

# TODO

- [x] Server implementation
  - Go's `net/http` serves requests on localhost:8080. A `map[string]string` stores values by URL key. PUT/POST stores JSON values, GET retrieves them, and DELETE removes them.
- [x] Concurrent requests
  - The HTTP server handles concurrent connections. `sync.RWMutex` protects the map: GET uses a shared read lock; writes and deletes use an exclusive lock.
- [ ] Persistence
- [x] Error handling
  - `http.Error` returns explanations with 400 for invalid input, 404 for missing keys, and 405 for unsupported methods. Invalid writes leave the store unchanged; startup failures are logged.
- [x] Logging
  - `log.Printf` records attempted operations with method, key, and timestamp in the terminal. The listening address is logged at startup.

Tests cover operations, errors, and 30 goroutines accessing the same key.
The race detector checks for unsafe concurrent memory access during tests.

`benchmark.py` uses three threads for 900 POST/GET/DELETE requests, verifies
values, and reports successes, failures, throughput, and latency. All samples
are retained. No extra Python packages are needed.
