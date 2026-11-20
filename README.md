# Simple key-value store
Run:
```sh
go run .
```

The server listens locally at http://127.0.0.1:8080. Keys and values are strings.

## Docker: three KV-store nodes

`compose.yaml` runs three identical containers of this same image. Each node is a
fully independent store with its own SQLite file in its own named volume, so the
nodes share no state.

| Service | Host address | Data volume |
| --- | --- | --- |
| `kv-store-1` | http://127.0.0.1:8081 | `kv-data-1` |
| `kv-store-2` | http://127.0.0.1:8082 | `kv-data-2` |
| `kv-store-3` | http://127.0.0.1:8083 | `kv-data-3` |

Build and start everything:
```sh
# Docker Desktop must be running
docker compose up --build -d
docker compose ps        # all three should be Up
```

Host ports are published only so a single node can be exercised before the
router exists. Nodes reach each other over the Compose network by service name
(`kv-store-1:8080`, `kv-store-2:8080`, `kv-store-3:8080`), which is the address
list the gateway will use. Once the gateway is added, only its port needs to be
published.

Talk to one node directly:
```sh
curl -i -X PUT http://127.0.0.1:8082/name \
  -H 'Content-Type: application/json' -d '{"value":"Anwar"}'
curl http://127.0.0.1:8082/name
# {"value":"Anwar"}
```
The same key written to different nodes returns different values, which confirms
the stores are independent.

Data survives a restart, `docker compose down`, and `docker compose up`, because
it lives in named volumes. `docker compose down -v` deletes the volumes and the
data.

### Adding or removing a node

Add a service to `compose.yaml` with its own port and volume, then re-run
`docker compose up -d --build`. Remove the service and volume for a smaller
cluster. The containers themselves need no changes; which keys belong to which
node is decided by the router, not by the store.

### Build the image alone
```sh
docker build -t simple-key-value-store .
docker run --rm -p 127.0.0.1:8080:8080 -v kv-data:/data simple-key-value-store
```

## API

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

## Storage

Data is stored in SQLite at `data/store.db`. Set the `DATA_FILE` environment
variable to choose another database path; containers use `/data/store.db`. The
`kv` table contains a primary-key `key` column and a non-null `value` column,
both strings. Concurrent HTTP requests share one database connection, which
serializes database work per node; atomic upserts and deletes handle concurrent
changes to the same key. SQLite transactions and rollback journaling preserve
writes and deletes across restarts. Compose keeps each node's file in a named
volume, so the data also survives container recreation and `docker compose down`.

Run tests with the race detector:
```sh
go test -race ./...
```

To run the Python benchmark, point `BASE_URL` at a node (the default is
`http://127.0.0.1:8080`, the port the gateway will use):
```sh
BASE_URL=http://127.0.0.1:8081 python3 kv_bench.py
```

# TODO
## Coding Assignment 3: Enhancing Key-Value Store with Consistent Hashing Across Multiple Instances
- [x] 1. Environment Setup
  - Previous KV store codebase is ready and functional.
- [ ] 2. Consistent Hashing Implementation
  - Distribute keys evenly among multiple KV stores using consistent hashing (own implementation or an existing library), and keep the system operational when the number of KV stores changes.
- [x] 3. Containerization (Optional)
  - Each KV store runs as its own container from the same image, with a separate named volume for its SQLite file and its own host port.
- [ ] 4. Testing and Measurement
  - Measure throughput and latency with 1, 2, and 3 KV stores, and document how they change at each stage.
- [ ] 5. Documentation and Submission
  - Report the implementation process, the challenges and how they were overcome, with tables or graphs of the measured throughput and latency, and submit it with the source code.

`kv_bench.py` uses three threads to write 300 keys, then read them back and
verify the values. It reports successes, errors, throughput, average latency,
and p99 for each phase, and keeps every latency sample. It needs the `requests`
package (the bundled `.venv` already has it).
