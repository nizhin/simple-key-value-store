# Rust key-value store

A small Rust version of the Go server, using Axum for HTTP, Tokio for the
async runtime, and Serde for JSON.

Run with
```sh
cargo run --release
```

i just want to try rust to see if faster lol. why is rust slower >:(

go results:
Total time:
  Mean: 0.9602 s
  Sample variance: 0.0096428163 s^2

Successful throughput:
  Mean: 946.87 ops/s
  Sample variance: 8898.682 (ops/s)^2

Average latency:
  Mean: 0.0029664 s/op
  Sample variance: 5.2586776e-08 (s/op)^2


Total time:
  Mean: 1.0414 s
  Sample variance: 0.015457184 s^2

Successful throughput:
  Mean: 875.9568 ops/s
  Sample variance: 10292.812 (ops/s)^2

Average latency:
  Mean: 0.0032006 s/op
  Sample variance: 8.9658816e-08 (s/op)^2