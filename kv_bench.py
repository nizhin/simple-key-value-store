"""KV-store benchmark: write every key, then read every key back and verify its value.

Usage:
  python3 kv_bench.py                                  # set + get, 32 threads, 3000 keys
  python3 kv_bench.py --url http://127.0.0.1:8081      # talk to one node directly
  python3 kv_bench.py --threads 64 --keys 10000
  python3 kv_bench.py --procs 4 --threads 16           # 4 processes x 16 threads (avoids the GIL limit)
  python3 kv_bench.py --phases get                     # read + verify only (e.g. after a restart)
  python3 kv_bench.py --label router-3 --nodes 3 --csv results/results.csv
"""
import argparse
import csv
import http.client
import json
import multiprocessing as mp
import os
import queue
import threading
import time
from collections import Counter
from urllib.parse import urlparse

MAX_LOGGED_PROBLEMS = 5   # print the first few errors/mismatches, count the rest


class Client:
    """One keep-alive HTTP connection, owned by a single thread."""

    def __init__(self, host, port):
        self.host, self.port = host, port
        self.conn = None

    def request(self, method, path, body=None):
        """Returns (status, headers, body bytes). Reconnects once if the connection dropped."""
        for attempt in (1, 2):
            if self.conn is None:
                self.conn = http.client.HTTPConnection(self.host, self.port, timeout=10)
            try:
                headers = {'Content-Type': 'application/json'} if body is not None else {}
                self.conn.request(method, path, body=body, headers=headers)
                resp = self.conn.getresponse()
                data = resp.read()
                return resp.status, resp.headers, data
            except (http.client.HTTPException, OSError):
                self.conn.close()
                self.conn = None
                if attempt == 2:
                    raise


class PhaseStats:
    def __init__(self):
        self.lock = threading.Lock()
        self.latencies = []      # seconds, successful ops only
        self.mismatch = 0
        self.error = 0
        self.per_node = Counter()  # X-KV-Node header -> count
        self.logged = 0

    def problem(self, msg):
        with self.lock:
            self.logged += 1
            if self.logged <= MAX_LOGGED_PROBLEMS:
                print('  ' + msg)
            elif self.logged == MAX_LOGGED_PROBLEMS + 1:
                print('  (further errors/mismatches suppressed; see counts in the results)')


def do_op(client, op, key, value, stats):
    """Run one operation. Returns 'ok', 'mismatch' or 'error'."""
    path = '/' + key
    try:
        if op == 'set':
            status, headers, _ = client.request('PUT', path, json.dumps({'value': value}))
            if status != 204:
                stats.problem(f'set {key}: HTTP {status}')
                return 'error', headers
            return 'ok', headers
        status, headers, data = client.request('GET', path)
        if status != 200:
            stats.problem(f'get {key}: HTTP {status}')
            return 'error', headers
        got = json.loads(data).get('value')
        if got != value:
            stats.problem(f'get {key}: expected {value!r}, got {got!r}')
            return 'mismatch', headers
        return 'ok', headers
    except Exception as e:  # network errors, bad JSON, ...
        stats.problem(f'{op} {key}: {e}')
        return 'error', None


def worker(host, port, q, stats, barrier):
    client = Client(host, port)
    latencies, per_node, mismatch, error = [], Counter(), 0, 0
    barrier.wait()   # all workers start together
    while True:
        try:
            op, key, value = q.get_nowait()
        except queue.Empty:
            break
        t0 = time.perf_counter()
        result, headers = do_op(client, op, key, value, stats)
        latency = time.perf_counter() - t0
        if result == 'ok':
            latencies.append(latency)
            if headers is not None and headers.get('X-KV-Node'):
                per_node[headers.get('X-KV-Node')] += 1
        elif result == 'mismatch':
            mismatch += 1
        else:
            error += 1
    # Merge once at the end instead of locking on every request.
    with stats.lock:
        stats.latencies.extend(latencies)
        stats.per_node.update(per_node)
        stats.mismatch += mismatch
        stats.error += error


def run_process(url, items, threads, start_barrier):
    """One client process: runs `threads` worker threads over `items`.
    Returns (stats fields, start time, end time) so the parent can merge results."""
    parsed = urlparse(url)
    q = queue.Queue()
    for item in items:
        q.put(item)
    stats = PhaseStats()
    barrier = threading.Barrier(threads + 1)
    workers = [threading.Thread(target=worker, args=(parsed.hostname, parsed.port or 80, q, stats, barrier))
               for _ in range(threads)]
    for t in workers:
        t.start()
    start_barrier.wait()   # every process starts sending at the same moment
    barrier.wait()
    t0 = time.time()
    for t in workers:
        t.join()
    return stats.latencies, stats.per_node, stats.mismatch, stats.error, t0, time.time()


def run_phase(url, op, keys, threads, procs, prefix, rounds):
    # For 'set' the value is what we write; for 'get' it's what we expect back.
    items = [(op, f'{prefix}key_{i}', f'value_{i}') for _ in range(rounds) for i in range(keys)]
    chunks = [items[p::procs] for p in range(procs)]   # deal items out round-robin
    with mp.Manager() as manager:
        start_barrier = manager.Barrier(procs)
        with mp.Pool(procs) as pool:
            results = pool.starmap(run_process, [(url, c, threads, start_barrier) for c in chunks])
    stats = PhaseStats()
    for latencies, per_node, mismatch, error, _, _ in results:
        stats.latencies.extend(latencies)
        stats.per_node.update(per_node)
        stats.mismatch += mismatch
        stats.error += error
    # Wall-clock window from the first process starting to the last one finishing.
    elapsed = max(r[5] for r in results) - min(r[4] for r in results)
    return stats, elapsed


def percentile(sorted_values, p):
    if not sorted_values:
        return float('nan')
    return sorted_values[min(len(sorted_values) - 1, int(p * len(sorted_values)))]


def summarize(op, stats, elapsed):
    lats = sorted(stats.latencies)
    ok = len(lats)
    return {
        'phase': op,
        'ok': ok,
        'mismatch': stats.mismatch,
        'error': stats.error,
        'seconds': round(elapsed, 3),
        'throughput_ops': round(ok / elapsed, 1) if elapsed else 0,
        'avg_ms': round(1e3 * sum(lats) / ok, 3) if ok else float('nan'),
        'p50_ms': round(1e3 * percentile(lats, 0.50), 3),
        'p95_ms': round(1e3 * percentile(lats, 0.95), 3),
        'p99_ms': round(1e3 * percentile(lats, 0.99), 3),
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--url', default='http://127.0.0.1:8080', help='router or node URL')
    ap.add_argument('--procs', type=int, default=4, help='client processes')
    ap.add_argument('--threads', type=int, default=16, help='threads per client process')
    ap.add_argument('--keys', type=int, default=3000, help='keys per phase')
    ap.add_argument('--phases', nargs='+', choices=['set', 'get'], default=['set', 'get'])
    ap.add_argument('--read-rounds', type=int, default=5,
                    help='the get phase reads every key this many times (reads are fast, so '
                         'this keeps the phase long enough to measure)')
    ap.add_argument('--prefix', default='', help='key prefix, e.g. to keep runs apart')
    ap.add_argument('--label', default='', help='configuration name written to the CSV, e.g. router-3')
    ap.add_argument('--nodes', type=int, default=0, help='number of KV nodes, written to the CSV')
    ap.add_argument('--csv', help='append one row per phase to this CSV file')
    args = ap.parse_args()

    rows = []
    for op in args.phases:
        rounds = args.read_rounds if op == 'get' else 1
        print(f'{op}: {args.keys} keys x {rounds}, {args.procs} procs x {args.threads} threads -> {args.url}')
        stats, elapsed = run_phase(args.url, op, args.keys, args.threads, args.procs, args.prefix, rounds)
        row = summarize(op, stats, elapsed)
        rows.append(row)
        print(f"  ok={row['ok']} mismatch={row['mismatch']} error={row['error']} "
              f"time={row['seconds']:.2f}s throughput={row['throughput_ops']:.1f} ops/s "
              f"avg={row['avg_ms']:.2f} p50={row['p50_ms']:.2f} p95={row['p95_ms']:.2f} "
              f"p99={row['p99_ms']:.2f} ms")
        if stats.per_node:
            total = sum(stats.per_node.values())
            split = ', '.join(f'{n} {100 * c / total:.1f}%' for n, c in sorted(stats.per_node.items()))
            print(f'  per node: {split}')

    if args.csv:
        fields = ['label', 'nodes', 'procs', 'threads', 'keys'] + list(rows[0].keys())
        new_file = not os.path.exists(args.csv)
        os.makedirs(os.path.dirname(args.csv) or '.', exist_ok=True)
        with open(args.csv, 'a', newline='') as f:
            w = csv.DictWriter(f, fieldnames=fields)
            if new_file:
                w.writeheader()
            for row in rows:
                w.writerow({'label': args.label, 'nodes': args.nodes, 'procs': args.procs,
                            'threads': args.threads, 'keys': args.keys, **row})


if __name__ == '__main__':
    main()
