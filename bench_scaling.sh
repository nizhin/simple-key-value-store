#!/usr/bin/env bash
# Measures throughput and latency as the number of KV nodes grows from 1 to 3.
#
# Configurations, each started fresh (volumes wiped):
#   direct    client -> kv1                (no router: single-store baseline)
#   router-1  client -> router -> kv1
#   router-2  client -> router -> kv1, kv2
#   router-3  client -> router -> kv1, kv2, kv3
#
# The client (kv_bench.py) runs in a container on the compose network, so
# Docker Desktop's host port forwarding doesn't cap the results.
#
# Usage:
#   ./bench_scaling.sh            # durable SQLite on Docker's disk  -> results/results-disk.csv
#   ./bench_scaling.sh tmpfs      # SQLite in RAM (see compose.tmpfs.yaml) -> results/results-tmpfs.csv
# Tunables (env vars): RUNS=3 KEYS=3000 PROCS=4 THREADS=16 READ_ROUNDS=20
set -euo pipefail
cd "$(dirname "$0")"

STORAGE=${1:-disk}
RUNS=${RUNS:-3}
KEYS=${KEYS:-3000}
PROCS=${PROCS:-4}
THREADS=${THREADS:-16}
READ_ROUNDS=${READ_ROUNDS:-20}
CSV=results/results-$STORAGE.csv
CLIENT_IMAGE=python:3.13-alpine

case $STORAGE in
  disk)  COMPOSE=(docker compose -f compose.yaml) ;;
  tmpfs) COMPOSE=(docker compose -f compose.yaml -f compose.tmpfs.yaml) ;;
  *) echo "usage: $0 [disk|tmpfs]" >&2; exit 1 ;;
esac

mkdir -p results
rm -f "$CSV"
export LOG_REQUESTS=false   # per-request logging would skew the numbers

# wait_until CMD...: retry CMD for up to 30s.
wait_until() {
  for _ in $(seq 60); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  echo "timed out waiting for: $*" >&2
  exit 1
}

ring_has() { # ring_has N: the router's ring lists exactly N nodes
  [ "$(curl -sf localhost:8080/_ring | grep -o 'kv[0-9]' | wc -l | tr -d ' ')" = "$1" ]
}

# run_config LABEL NODE_COUNT URL SERVICES...
run_config() {
  local label=$1 nodes=$2 url=$3; shift 3
  local node_list
  node_list=$(printf '%s:8080,' $(printf '%s\n' "$@" | grep '^kv') | sed 's/,$//')
  echo
  echo "=== $label ($STORAGE): services $*"
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1
  NODES=$node_list "${COMPOSE[@]}" up -d --build "$@" >/dev/null 2>&1
  for svc in "$@"; do
    case $svc in
      kv1) wait_until curl -sf localhost:8081/_health ;;
      kv2) wait_until curl -sf localhost:8082/_health ;;
      kv3) wait_until curl -sf localhost:8083/_health ;;
      router) wait_until ring_has "$nodes" ;;
    esac
  done
  local network
  network=$(docker inspect "$("${COMPOSE[@]}" ps -q "$1")" \
    --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')
  for run in $(seq "$RUNS"); do
    echo "--- run $run/$RUNS"
    docker run --rm --network "$network" -v "$PWD:/bench" -w /bench "$CLIENT_IMAGE" \
      python kv_bench.py --url "$url" --procs "$PROCS" --threads "$THREADS" \
      --keys "$KEYS" --read-rounds "$READ_ROUNDS" --prefix "run$run-" \
      --label "$label" --nodes "$nodes" --csv "$CSV"
  done
}

run_config direct   1 http://kv1:8080    kv1
run_config router-1 1 http://router:8080 kv1 router
run_config router-2 2 http://router:8080 kv1 kv2 router
run_config router-3 3 http://router:8080 kv1 kv2 kv3 router

"${COMPOSE[@]}" down -v >/dev/null 2>&1
echo
echo "Results written to $CSV"
