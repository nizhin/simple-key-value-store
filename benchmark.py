import json
import threading
import time
import urllib.error
import urllib.request


BASE_URL = "http://127.0.0.1:8080"
NUM_THREADS = 3
KEYS_PER_THREAD = 100
PRINT_INTERVAL = 3
REQUEST_TIMEOUT = 5


def kv_store_operation(method, key, value):
    data = json.dumps({"value": value}).encode() if method == "POST" else None
    request = urllib.request.Request(
        f"{BASE_URL}/{key}",
        data=data,
        headers={"Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(request, timeout=REQUEST_TIMEOUT) as response:
            if method == "GET" and json.load(response) != {"value": value}:
                raise ValueError("retrieved value does not match stored value")
        return True
    except (urllib.error.URLError, OSError, ValueError) as error:
        print(f"Error during {method} for '{key}': {error}")
        return False


def main():
    results = []
    results_lock = threading.Lock()
    start_event = threading.Event()
    stop_event = threading.Event()

    def worker(worker_id):
        start_event.wait()
        for i in range(KEYS_PER_THREAD):
            key = f"benchmark_{worker_id}_{i}"
            value = f"value_{worker_id}_{i}"
            for method in ("POST", "GET", "DELETE"):
                started = time.perf_counter()
                success = kv_store_operation(method, key, value)
                latency = time.perf_counter() - started
                with results_lock:
                    results.append((latency, success))

    def monitor():
        last_count = 0
        last_time = started
        while not stop_event.wait(PRINT_INTERVAL):
            with results_lock:
                samples = results[last_count:]
                last_count = len(results)
            now = time.perf_counter()
            if samples:
                throughput = sum(success for _, success in samples) / (now - last_time)
                latency = sum(latency for latency, _ in samples) / len(samples)
                print(
                    f"[Last {PRINT_INTERVAL} seconds] "
                    f"Successful throughput: {throughput:.2f} ops/sec, "
                    f"Avg latency (all attempts): {latency:.5f} sec/op"
                )
            last_time = now

    threads = [threading.Thread(target=worker, args=(i,)) for i in range(NUM_THREADS)]
    for thread in threads:
        thread.start()

    started = time.perf_counter()
    monitoring_thread = threading.Thread(target=monitor)
    monitoring_thread.start()
    start_event.set()
    for thread in threads:
        thread.join()
    elapsed = time.perf_counter() - started
    stop_event.set()
    monitoring_thread.join()

    total = len(results)
    succeeded = sum(success for _, success in results)
    failed = total - succeeded
    average_latency = sum(latency for latency, _ in results) / total if total else 0
    print("\nFinal Results:")
    print(f"Total operations: {total}")
    print(f"Succeeded: {succeeded}, Failed: {failed}")
    print(f"Total time: {elapsed:.2f} seconds")
    print(f"Successful throughput: {succeeded / elapsed:.2f} operations per second")
    print(f"Average latency (all attempts): {average_latency:.5f} seconds per operation")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
