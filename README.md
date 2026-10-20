# Simple key-value store
Run:
```sh
go run .
```

The server listens locally at http://127.0.0.1:8080. Keys and values are strings.

Build and run Go version with Docker
```sh
docker build -t simple-key-value-store .
docker run --rm -p 127.0.0.1:8080:8080 -v kv-data:/data simple-key-value-store
```

i just made compose easier.
```sh
docker compose up --build -d
```

bring down with
```sh
docker compose down
```

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

Data is stored in SQLite at `data/store.db`. Set the `DATA_FILE` environment
variable to choose another database path. The `kv` table contains a primary-key
`key` column and a non-null `value` column, both strings.

Concurrent HTTP requests share one database connection
Docker uses `/data/store.db`; the named volume in Compose preserves it across
container recreation and `docker compose down`. `docker compose down -v` deletes
the volume and its data.

Run tests with the race detector:
```sh
go test -race ./...
```

To run the Python benchmark, keep the server running and use:
```sh
python3 benchmark.py
```

# TODO
## Coding Assignment 2: Docker file
- [ ] Base image
- [ ] Application files
- [ ] Dependencies
- [ ] Exposure
- [ ] Entry point

## Coding Assignment 1: Designing a Simple Single-Server Key-Value Store
- [x] Server implementation
  - Go's `net/http` serves requests on localhost:8080. SQLite stores string values by URL key. PUT/POST accepts JSON values, GET retrieves them, and DELETE removes them.
- [x] Concurrent requests
  - The HTTP server handles concurrent connections. One shared SQL connection serializes database operations; atomic upserts and deletes handle concurrent changes to the same key.
- [x] Persistence
  - SQLite transactions and rollback journaling preserve writes and deletes across restarts.
- [x] Error handling
  - `http.Error` returns explanations with 400 for invalid input, 404 for missing keys, and 405 for unsupported methods. Invalid writes leave the store unchanged; startup failures are logged.
- [x] Logging
  - `log.Printf` records attempted operations with method, key, and timestamp in the terminal. The listening address is logged at startup.

Tests cover operations, database errors, restart persistence, concurrent writes,
and 30 goroutines accessing the same key.
The race detector checks for unsafe concurrent memory access during tests.

`benchmark.py` uses three threads for 900 POST/GET/DELETE requests, verifies
values, and reports successes, failures, throughput, and latency. All samples
are retained. No extra Python packages are needed.
