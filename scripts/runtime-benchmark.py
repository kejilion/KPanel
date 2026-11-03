#!/usr/bin/env python3
"""Small dependency-free HTTP GET benchmark for KPanel runtime checks."""

from __future__ import annotations

import argparse
import http.client
import json
import math
import queue
import re
import ssl
import sys
import threading
import time
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit


def positive_int(value: str) -> int:
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("must be at least 1")
    return parsed


def non_negative_int(value: str) -> int:
    parsed = int(value)
    if parsed < 0:
        raise argparse.ArgumentTypeError("must not be negative")
    return parsed


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Benchmark a fixed KPanel GET endpoint and emit one JSON result.",
    )
    parser.add_argument("--base-url", required=True, help="HTTP(S) origin without a path")
    parser.add_argument("--path", required=True, help="absolute endpoint path")
    parser.add_argument("--requests", type=positive_int, default=1000)
    parser.add_argument("--concurrency", type=positive_int, default=10)
    parser.add_argument("--warmup", type=non_negative_int, default=5)
    parser.add_argument("--timeout", type=positive_int, default=30)
    parser.add_argument("--expect-status", type=positive_int, default=200)
    parser.add_argument(
        "--cookie-file",
        type=Path,
        help="optional Netscape cookie jar created by curl",
    )
    args = parser.parse_args()
    if args.requests > 100_000:
        parser.error("--requests must not exceed 100000")
    if args.concurrency > 256:
        parser.error("--concurrency must not exceed 256")
    if args.concurrency > args.requests:
        parser.error("--concurrency must not exceed --requests")
    if args.warmup > 1000:
        parser.error("--warmup must not exceed 1000")
    return args


def validate_target(base_url: str, path: str) -> tuple[str, str, int, bool]:
    parsed = urlsplit(base_url)
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.hostname
        or parsed.username
        or parsed.password
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("--base-url must be an HTTP(S) origin without credentials or a path")
    request_path = urlsplit(path)
    if (
        not path.startswith("/")
        or path.startswith("//")
        or "\\" in path
        or any(ord(char) < 0x21 or ord(char) > 0x7E for char in path)
        or request_path.scheme
        or request_path.netloc
        or request_path.fragment
    ):
        raise ValueError("--path must be an absolute local path")
    use_tls = parsed.scheme == "https"
    port = parsed.port or (443 if use_tls else 80)
    return parsed.scheme, parsed.hostname, port, use_tls


def cookie_path_matches(request_path: str, cookie_path: str) -> bool:
    cookie_path = cookie_path or "/"
    if not request_path.startswith(cookie_path):
        return False
    return cookie_path.endswith("/") or len(request_path) == len(cookie_path) or request_path[len(cookie_path)] == "/"


def read_cookie_header(path: Path | None, scheme: str, host: str, request_path: str) -> str:
    if path is None:
        return ""
    if scheme != "https":
        raise ValueError("--cookie-file requires an HTTPS target")
    cookies: list[str] = []
    normalized_host = host.rstrip(".").lower()
    now = int(time.time())
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("#HttpOnly_"):
            line = line.removeprefix("#HttpOnly_")
        elif not line or line.startswith("#"):
            continue
        fields = line.split("\t")
        if len(fields) != 7:
            continue
        domain = fields[0].lower()
        if domain.startswith("."):
            domain = domain[1:]
        include_subdomains = fields[1].upper() == "TRUE"
        secure_only = fields[3].upper() == "TRUE"
        try:
            expires = int(fields[4])
        except ValueError:
            continue
        name, value = fields[5], fields[6]
        if (
            not domain
            or domain.startswith(".")
            or domain.endswith(".")
            or not (normalized_host == domain or (include_subdomains and normalized_host.endswith("." + domain)))
        ):
            continue
        if not cookie_path_matches(request_path, fields[2]):
            continue
        if secure_only and scheme != "https":
            continue
        if expires > 0 and expires <= now:
            continue
        if (
            not re.fullmatch(r"[!#$%&'*+.^_`|~0-9A-Za-z-]+", name)
            or any(ord(char) < 0x21 or ord(char) > 0x7E or char in '\\";, ' for char in value)
        ):
            continue
        cookies.append(f"{name}={value}")
    if not cookies:
        raise ValueError("cookie file did not contain any cookies")
    return "; ".join(cookies)


def safe_target_label(base_url: str, path: str) -> str:
    base = urlsplit(base_url)
    host = base.hostname or ""
    if ":" in host:
        host = f"[{host}]"
    port = base.port or (443 if base.scheme == "https" else 80)
    target_path = urlsplit(path).path or "/"
    return f"{base.scheme}://{host}:{port}{target_path}"


def requests_per_second(completed: int, duration: float) -> float:
    return round(completed / duration, 2) if duration > 0 else 0.0


def percentile(values: list[float], fraction: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    rank = max(0, math.ceil(fraction * len(ordered)) - 1)
    return ordered[rank]


def connection(host: str, port: int, use_tls: bool, timeout: int) -> http.client.HTTPConnection:
    if use_tls:
        return http.client.HTTPSConnection(
            host,
            port,
            timeout=timeout,
            context=ssl.create_default_context(),
        )
    return http.client.HTTPConnection(host, port, timeout=timeout)


def single_request(
    conn: http.client.HTTPConnection,
    path: str,
    headers: dict[str, str],
) -> tuple[int, int, float]:
    started = time.perf_counter()
    conn.request("GET", path, headers=headers)
    response = conn.getresponse()
    payload = response.read()
    elapsed_ms = (time.perf_counter() - started) * 1000
    return response.status, len(payload), elapsed_ms


def main() -> int:
    args = parse_args()
    try:
        scheme, host, port, use_tls = validate_target(args.base_url, args.path)
        request_path = urlsplit(args.path).path or "/"
        cookie_header = read_cookie_header(args.cookie_file, scheme, host, request_path)
    except (OSError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 2

    headers = {
        "Accept": "application/json,text/html;q=0.9,*/*;q=0.8",
        "Connection": "keep-alive",
        "User-Agent": "KPanel-runtime-benchmark/1.0",
    }
    if cookie_header:
        headers["Cookie"] = cookie_header

    warmup_conn = connection(host, port, use_tls, args.timeout)
    try:
        for _ in range(args.warmup):
            single_request(warmup_conn, args.path, headers)
    except (OSError, http.client.HTTPException) as exc:
        print(f"warmup failed: {type(exc).__name__}", file=sys.stderr)
        return 1
    finally:
        warmup_conn.close()

    work: queue.Queue[int | None] = queue.Queue()
    for index in range(args.requests):
        work.put(index)
    for _ in range(args.concurrency):
        work.put(None)

    latencies: list[float] = []
    statuses: Counter[int] = Counter()
    error_types: Counter[str] = Counter()
    total_bytes = 0
    lock = threading.Lock()
    ready = threading.Barrier(args.concurrency + 1)

    def worker() -> None:
        nonlocal total_bytes
        conn = connection(host, port, use_tls, args.timeout)
        ready.wait()
        try:
            while True:
                item = work.get()
                if item is None:
                    break
                try:
                    status, payload_bytes, elapsed_ms = single_request(conn, args.path, headers)
                except (OSError, http.client.HTTPException) as exc:
                    with lock:
                        error_types[type(exc).__name__] += 1
                    conn.close()
                    conn = connection(host, port, use_tls, args.timeout)
                    continue
                with lock:
                    statuses[status] += 1
                    total_bytes += payload_bytes
                    latencies.append(elapsed_ms)
        finally:
            conn.close()

    threads = [threading.Thread(target=worker, daemon=True) for _ in range(args.concurrency)]
    for thread in threads:
        thread.start()
    ready.wait()
    started = time.perf_counter()
    for thread in threads:
        thread.join()
    duration = time.perf_counter() - started

    completed = sum(statuses.values())
    unexpected = sum(count for status, count in statuses.items() if status != args.expect_status)
    result = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "target": safe_target_label(args.base_url, args.path),
        "requests": args.requests,
        "concurrency": args.concurrency,
        "warmup": args.warmup,
        "durationSeconds": round(duration, 6),
        "requestsPerSecond": requests_per_second(completed, duration),
        "completed": completed,
        "responseBytes": total_bytes,
        "statusCodes": dict(sorted(statuses.items())),
        "errors": dict(sorted(error_types.items())),
        "latencyMs": {
            "min": round(min(latencies), 3) if latencies else 0.0,
            "p50": round(percentile(latencies, 0.50), 3),
            "p95": round(percentile(latencies, 0.95), 3),
            "p99": round(percentile(latencies, 0.99), 3),
            "max": round(max(latencies), 3) if latencies else 0.0,
            "mean": round(sum(latencies) / len(latencies), 3) if latencies else 0.0,
        },
    }
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 1 if error_types or unexpected or completed != args.requests else 0


if __name__ == "__main__":
    raise SystemExit(main())
