#!/usr/bin/env python3
"""
AIM Benchmark Tool -- Load & Stress Testing
===========================================
基于 dev-tool/aim_test.py 的 REST/WS 客户端，提供并发压测能力。

Usage:
  python benchmark.py register --users 100
  python benchmark.py login --users 100
  python benchmark.py friend-chain --users 50
  python benchmark.py ws-message --users 20 --messages-per-user 100
  python benchmark.py mixed --users 50 --duration 30

通用参数:
  --users N         并发用户数
  --rps N           目标每秒请求数（默认: 不限）
  --duration N      持续时间（秒），默认 0 表示跑完即止
  --ramp-up N       渐进加压时间（秒），默认 0
  --output PATH     输出 JSON 报告路径
"""

import sys
import os
import time
import json
import uuid
import signal
import argparse
import threading
import concurrent.futures
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Optional, Callable, List, Dict, Any

# Add dev-tool to path so we can import from aim_test
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# Force UTF-8 output to avoid Windows GBK encoding errors with box-drawing chars.
# This is equivalent to PYTHONIOENCODING=utf-8 but applied programmatically.
if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

from aim_test import (
    RESTClient, WSClient, TokenManager, APIError,
    ws_pb2, GATEWAY_HTTP, GATEWAY_WS, _state_file,
)
import aim_test as _aim_test
import logging as _logging

# QUIET 是进程级别的静默开关，被各 cmd_* 入口根据 --quiet 设置。
# True 时抽默场景中除 "Scenario / [n/4] / 最终报告" 以外的调试/错误输出。
QUIET = False


def _vprint(*args, **kwargs):
    """Verbose-only print; 静默模式下不输出。"""
    if not QUIET:
        print(*args, **kwargs)


def _apply_quiet(quiet: bool):
    """全局应用静默设置：关闭 WSClient 调试输出、屏蔽 websocket-client 与 urllib3 的 logger。"""
    global QUIET
    QUIET = bool(quiet)
    _aim_test.set_verbose(not QUIET)
    if QUIET:
        # websocket-client 在连接失败/超时时会通过 logger 输出堆栈跟踪
        _logging.getLogger("websocket").setLevel(_logging.CRITICAL)
        _logging.getLogger("urllib3").setLevel(_logging.CRITICAL)
        _logging.getLogger("requests").setLevel(_logging.CRITICAL)


# 哨兵值：task_fn 返回此值时，_worker_loop 跳过自动指标记录（由 task_fn 自行管理）
_SKIP_METRICS = object()


# ===============================================================================
# Metrics Collector
# ===============================================================================

@dataclass
class MetricsSnapshot:
    """Immutable snapshot of current metrics."""
    total: int = 0
    success: int = 0
    errors: int = 0
    latencies: List[float] = field(default_factory=list)
    error_types: Dict[str, int] = field(default_factory=dict)
    start_time: float = 0.0

    @property
    def error_rate(self) -> float:
        if self.total == 0:
            return 0.0
        return self.errors / self.total

    @property
    def elapsed(self) -> float:
        return time.time() - self.start_time

    @property
    def qps(self) -> float:
        if self.elapsed <= 0:
            return 0.0
        return self.total / self.elapsed

    def latency_stats(self) -> Dict[str, float]:
        if not self.latencies:
            return {"min": 0, "p50": 0, "p90": 0, "p95": 0, "p99": 0, "max": 0}
        sorted_lat = sorted(self.latencies)
        n = len(sorted_lat)
        return {
            "min": sorted_lat[0] * 1000,
            "p50": sorted_lat[int(n * 0.50)] * 1000,
            "p90": sorted_lat[int(n * 0.90)] * 1000,
            "p95": sorted_lat[int(n * 0.95)] * 1000,
            "p99": sorted_lat[min(int(n * 0.99), n - 1)] * 1000,
            "max": sorted_lat[-1] * 1000,
            "avg": (sum(sorted_lat) / n) * 1000,
        }


class MetricsCollector:
    """Thread-safe metrics collector."""

    def __init__(self):
        self._lock = threading.Lock()
        self._total = 0
        self._success = 0
        self._errors = 0
        self._latencies: List[float] = []
        self._error_types: Dict[str, int] = defaultdict(int)
        self._start_time = time.time()

    def record_success(self, latency_s: float):
        with self._lock:
            self._total += 1
            self._success += 1
            self._latencies.append(latency_s)

    def record_error(self, latency_s: float, error_type: str = "unknown"):
        with self._lock:
            self._total += 1
            self._errors += 1
            self._latencies.append(latency_s)
            self._error_types[error_type] += 1

    def snapshot(self) -> MetricsSnapshot:
        with self._lock:
            return MetricsSnapshot(
                total=self._total,
                success=self._success,
                errors=self._errors,
                latencies=list(self._latencies),
                error_types=dict(self._error_types),
                start_time=self._start_time,
            )

    def reset(self):
        with self._lock:
            self._total = 0
            self._success = 0
            self._errors = 0
            self._latencies.clear()
            self._error_types.clear()
            self._start_time = time.time()


# ===============================================================================
# Rate Limiter (Token Bucket)
# ===============================================================================

class RateLimiter:
    """Token bucket rate limiter with optional ramp-up."""

    def __init__(self, target_rps: float = 0, ramp_up_seconds: float = 0):
        self._target_rps = target_rps
        self._ramp_up = ramp_up_seconds
        self._rate = 0.0 if ramp_up_seconds > 0 else target_rps
        self._tokens = 0.0
        self._capacity = max(1.0, target_rps * 0.5) if target_rps > 0 else 1.0
        self._last_refill = time.monotonic()
        self._start_time = time.monotonic()
        self._lock = threading.Lock()

    @property
    def unlimited(self) -> bool:
        return self._target_rps <= 0

    def _update_rate(self):
        """Recalculate rate based on ramp-up progress."""
        if self._ramp_up <= 0 or self._target_rps <= 0:
            return
        elapsed = time.monotonic() - self._start_time
        if elapsed >= self._ramp_up:
            self._rate = self._target_rps
        else:
            self._rate = self._target_rps * (elapsed / self._ramp_up)

    def acquire(self) -> float:
        """Acquire one token. Returns wait time in seconds (0 = immediate)."""
        if self.unlimited:
            return 0.0

        with self._lock:
            self._update_rate()
            now = time.monotonic()
            elapsed = now - self._last_refill
            self._tokens = min(self._capacity, self._tokens + elapsed * self._rate)
            self._last_refill = now

            if self._tokens >= 1.0:
                self._tokens -= 1.0
                return 0.0
            else:
                # Calculate wait time
                wait = (1.0 - self._tokens) / self._rate if self._rate > 0 else 0.001
                self._tokens = 0.0
                return wait


# ===============================================================================
# Report Printer
# ===============================================================================

class ReportPrinter:
    """Real-time progress + final summary report."""

    @staticmethod
    def _format_duration(seconds: float) -> str:
        if seconds < 60:
            return f"{seconds:.1f}s"
        m, s = divmod(seconds, 60)
        return f"{int(m)}m{s:.0f}s"

    @staticmethod
    def _format_latency(ms: float) -> str:
        if ms < 1:
            return f"{ms*1000:.0f}us"
        elif ms < 1000:
            return f"{ms:.1f}ms"
        else:
            return f"{ms/1000:.2f}s"

    @staticmethod
    def _bar(current: int, total: int, width: int = 30) -> str:
        if total == 0:
            return "[" + " " * width + "]"
        filled = int(width * current / total)
        return "[" + "=" * filled + " " * (width - filled) + "]"

    @classmethod
    def progress_line(cls, snap: MetricsSnapshot, total_target: int = 0,
                      label: str = "") -> str:
        """Build a one-line progress string."""
        lat = snap.latency_stats()
        parts = []

        if label:
            parts.append(f"[{label}]")

        # Progress bar
        if total_target > 0:
            parts.append(cls._bar(snap.total, total_target))
            pct = snap.total / total_target * 100
            parts.append(f"{pct:.0f}%")

        # Counts
        parts.append(f"req={snap.total}")
        if snap.errors > 0:
            parts.append(f"err={snap.errors}")

        # QPS
        parts.append(f"qps={snap.qps:.1f}")

        # Latency
        parts.append(f"p50={cls._format_latency(lat['p50'])}")
        parts.append(f"p95={cls._format_latency(lat['p95'])}")
        parts.append(f"p99={cls._format_latency(lat['p99'])}")

        return "  ".join(parts)

    @classmethod
    def summary(cls, snap: MetricsSnapshot, title: str = "Benchmark Results"):
        """Build a multi-line summary report."""
        lat = snap.latency_stats()
        duration = snap.elapsed
        lines = []
        sep = "-" * 60
        lines.append(sep)
        lines.append(f"  {title}")
        lines.append(sep)

        # Overview
        lines.append(f"  Duration:        {cls._format_duration(duration)}")
        lines.append(f"  Total Requests:  {snap.total}")
        lines.append(f"  Success:         {snap.success}")
        lines.append(f"  Errors:          {snap.errors} ({snap.error_rate*100:.1f}%)")
        lines.append(f"  Avg QPS:         {snap.qps:.1f}")

        # Latency distribution
        lines.append(f"  -- Latency (ms) --")
        lines.append(f"  Min: {cls._format_latency(lat['min']):>8}   "
                     f"Avg: {cls._format_latency(lat['avg']):>8}   "
                     f"Max: {cls._format_latency(lat['max']):>8}")
        lines.append(f"  P50: {cls._format_latency(lat['p50']):>8}   "
                     f"P90: {cls._format_latency(lat['p90']):>8}   "
                     f"P95: {cls._format_latency(lat['p95']):>8}   "
                     f"P99: {cls._format_latency(lat['p99']):>8}")

        # Error breakdown
        if snap.error_types:
            lines.append(f"  -- Error Types --")
            for etype, count in sorted(snap.error_types.items(),
                                        key=lambda x: -x[1]):
                lines.append(f"  {etype}: {count}")

        # Latency histogram (ASCII)
        lines.append(f"  -- Latency Histogram --")
        histogram = cls._build_histogram(snap.latencies)
        for label, count, bar in histogram:
            lines.append(f"  {label:>8}  {bar}  ({count})")

        lines.append(sep)
        # Safely encode for Windows console
        return "\n".join(lines)

    @classmethod
    def _build_histogram(cls, latencies_s: List[float]) -> List[tuple]:
        """Build ASCII histogram buckets."""
        if not latencies_s:
            return []

        buckets = [
            (0, 0.001, "< 1ms"),
            (0.001, 0.005, "1-5ms"),
            (0.005, 0.010, "5-10ms"),
            (0.010, 0.025, "10-25ms"),
            (0.025, 0.050, "25-50ms"),
            (0.050, 0.100, "50-100ms"),
            (0.100, 0.250, "100-250ms"),
            (0.250, 0.500, "250-500ms"),
            (0.500, 1.000, "0.5-1s"),
            (1.000, 5.000, "1-5s"),
            (5.000, 30.000, "5-30s"),
            (30.000, float("inf"), "> 30s"),
        ]

        counts = [0] * len(buckets)
        for lat in latencies_s:
            for i, (lo, hi, _) in enumerate(buckets):
                if lo <= lat < hi:
                    counts[i] += 1
                    break

        max_count = max(counts) if counts else 1
        bar_width = 20

        result = []
        for (lo, hi, label), count in zip(buckets, counts):
            if count > 0:
                filled = int(bar_width * count / max_count)
                bar = "#" * filled + "." * (bar_width - filled)
            else:
                bar = " " * bar_width
            result.append((label, count, bar))
        return result

    @classmethod
    def to_json(cls, snap: MetricsSnapshot, title: str = "benchmark") -> dict:
        """Export metrics as JSON-serializable dict."""
        lat = snap.latency_stats()
        return {
            "title": title,
            "duration_s": snap.elapsed,
            "total_requests": snap.total,
            "success": snap.success,
            "errors": snap.errors,
            "error_rate": snap.error_rate,
            "avg_qps": snap.qps,
            "latency_ms": lat,
            "error_types": snap.error_types,
        }


# ===============================================================================
# Load Generator
# ===============================================================================

class LoadGenerator:
    """Concurrent load generator with rate limiting and metrics collection."""

    def __init__(self, metrics: MetricsCollector, rate_limiter: RateLimiter,
                 workers: int = 10, duration: float = 0, verbose: bool = True):
        self.metrics = metrics
        self.rate_limiter = rate_limiter
        self.workers = workers
        self.duration = duration
        self.verbose = verbose
        self._stop = threading.Event()
        self._total_target = 0
        self._dispatch_lock = threading.Lock()
        self._dispatched = 0

    def run(self, task_fn: Callable[[], Optional[float]], total_target: int = 0,
            pre_hook: Optional[Callable] = None, post_hook: Optional[Callable] = None):
        """Run tasks concurrently.

        Args:
            task_fn: Function that executes one task. Returns latency_s on
                     success, None on skip. Raises exception on error.
            total_target: Maximum number of tasks to run (0 = unlimited).
            pre_hook: Called once in each worker before starting.
            post_hook: Called once in each worker after finishing.
        """
        self._total_target = total_target
        self._stop.clear()
        self._dispatched = 0
        self.metrics.reset()

        # Progress tracking
        if self.verbose:
            progress_t = threading.Thread(target=self._progress_reporter, daemon=True)
            progress_t.start()

        with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
            futures = []
            for _ in range(self.workers):
                futures.append(pool.submit(
                    self._worker_loop, task_fn, total_target, pre_hook, post_hook
                ))

            # Wait for completion or duration
            deadline = time.time() + self.duration if self.duration > 0 else float("inf")
            try:
                for f in concurrent.futures.as_completed(futures, timeout=self.duration or None):
                    try:
                        f.result()
                    except Exception as e:
                        pass  # errors already recorded in _worker_loop
            except concurrent.futures.TimeoutError:
                self._stop.set()
            except KeyboardInterrupt:
                if self.verbose:
                    print("\n  [!] Interrupted -- collecting intermediate results...")
                self._stop.set()

        # Print final summary
        if self.verbose:
            print()  # newline after progress line
            snap = self.metrics.snapshot()
            print(ReportPrinter.summary(snap))

    def _worker_loop(self, task_fn, total_target, pre_hook, post_hook):
        """Worker thread loop."""
        if pre_hook:
            pre_hook()

        while not self._stop.is_set():
            # Check global progress with thread-safe dispatch counter
            if total_target > 0:
                with self._dispatch_lock:
                    if self._dispatched >= total_target:
                        break
                    self._dispatched += 1

            # Rate limiting
            wait = self.rate_limiter.acquire()
            if wait > 0:
                time.sleep(wait)
                if self._stop.is_set():
                    break

            # Duration check
            if self.duration > 0 and time.time() - self.metrics._start_time > self.duration:
                self._stop.set()
                break

            # Execute task
            t0 = time.time()
            try:
                result = task_fn()
                # 返回 _SKIP_METRICS 表示 task_fn 自行管理指标记录
                if result is _SKIP_METRICS:
                    pass
                else:
                    latency = time.time() - t0
                    self.metrics.record_success(latency)
            except APIError as e:
                latency = time.time() - t0
                error_type = f"api_{e.code}"
                self.metrics.record_error(latency, error_type)
            except Exception as e:
                latency = time.time() - t0
                error_type = type(e).__name__
                self.metrics.record_error(latency, error_type)

        if post_hook:
            post_hook()

    def _progress_reporter(self):
        """Periodically print progress to stderr."""
        last_snap_time = time.time()
        while not self._stop.is_set():
            time.sleep(1.0)
            snap = self.metrics.snapshot()
            line = ReportPrinter.progress_line(snap, self._total_target)
            # Use \r to overwrite line
            sys.stderr.write(f"\r{line}\033[K")
            sys.stderr.flush()


# ===============================================================================
# Scenario: Register
# ===============================================================================

def _random_email(prefix: str = "bench") -> str:
    return f"{prefix}_{uuid.uuid4().hex[:8]}_{int(time.time()*1000)}@aim.dev"


def _load_user_fixtures(path: str = "") -> List[dict]:
    """从 user.json 加载预生成的用户数据。"""
    if not path:
        path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "user.json")
    if not os.path.exists(path):
        return []
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def _load_msg_fixtures(path: str = "") -> List[str]:
    """从 msg.txt 加载预生成的消息内容（每行一条）。"""
    if not path:
        path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "msg.txt")
    if not os.path.exists(path):
        return []
    with open(path, "r", encoding="utf-8") as f:
        return [line.rstrip("\n") for line in f if line.strip()]


class RegisterScenario:
    """Bulk user registration benchmark.

    支持两种模式：
    - fixture 模式：从 user.json 读取预生成用户数据，确保稳定可复现
    - 随机模式：运行时生成随机用户（默认行为，向后兼容）
    """

    def __init__(self, users: int, rps: float = 0, ramp_up: float = 0,
                 use_fixtures: bool = True, user_fixture_path: str = ""):
        self.users = users
        self.metrics = MetricsCollector()
        self.rate_limiter = RateLimiter(rps, ramp_up)
        self.use_fixtures = use_fixtures
        self.user_fixture_path = user_fixture_path

    def run(self):
        emails: List[str] = []
        email_lock = threading.Lock()

        # 尝试加载 fixture 数据
        fixtures = []
        if self.use_fixtures:
            fixtures = _load_user_fixtures(self.user_fixture_path)
            if fixtures:
                print(f"  Loaded {len(fixtures)} user fixtures from user.json")
            else:
                print(f"  user.json not found, falling back to random generation")

        fixture_idx = 0
        idx_lock = threading.Lock()

        def register_one():
            nonlocal emails, fixture_idx
            with idx_lock:
                if fixture_idx < len(fixtures):
                    user_data = fixtures[fixture_idx]
                    fixture_idx += 1
                else:
                    user_data = None

            if user_data:
                email = user_data["email"]
                password = user_data["password"]
                username = user_data["username"]
            else:
                email = _random_email("benchreg")
                password = "bench123456"
                username = f"User_{email[:8]}"

            client = RESTClient()
            client.register(email, password, username)
            with email_lock:
                emails.append(email)
            # Also login to save token for cleanup
            client.login(email, password)

        print(f"\n  Scenario: Register -- {self.users} users"
              + (f", RPS={self.rate_limiter._target_rps}" if self.rate_limiter._target_rps > 0 else "")
              + (f", fixtures={'on' if fixtures else 'off'}" if self.use_fixtures else ""))

        gen = LoadGenerator(self.metrics, self.rate_limiter,
                            workers=min(self.users, 50), duration=0)
        gen.run(register_one, total_target=self.users)

        # Cleanup state files
        self._cleanup_state()

        print(f"  Registered {len(emails)} / {self.users} users")
        return self.metrics.snapshot()

    @staticmethod
    def _cleanup_state():
        base = os.path.dirname(__file__)
        for name in os.listdir(base):
            if name.startswith(".aim_state_") and name.endswith(".json"):
                try:
                    os.remove(os.path.join(base, name))
                except OSError:
                    pass


# ===============================================================================
# Scenario: Login
# ===============================================================================

class LoginScenario:
    """Bulk login benchmark. Requires pre-registered users."""

    def __init__(self, users: int, rps: float = 0, ramp_up: float = 0,
                 pre_register: bool = True):
        self.users = users
        self.pre_register = pre_register
        self.metrics = MetricsCollector()
        self.rate_limiter = RateLimiter(rps, ramp_up)
        self._emails: List[str] = []
        self._password = "bench123456"

    def run(self):
        if self.pre_register:
            print(f"\n  Pre-registering {self.users} users...")
            reg_scenario = RegisterScenario(self.users)
            reg_scenario.run()
            # Collect registered emails from state files
            self._emails = self._gather_emails()
        else:
            self._emails = self._gather_emails()
            if not self._emails:
                print("  x No pre-registered users found. Run 'benchmark.py register' first.")
                return self.metrics.snapshot()

        actual_users = min(len(self._emails), self.users)
        email_idx = 0
        idx_lock = threading.Lock()

        def login_one():
            nonlocal email_idx
            with idx_lock:
                if email_idx >= len(self._emails):
                    return
                email = self._emails[email_idx]
                email_idx += 1
            client = RESTClient()
            client.login(email, self._password)

        print(f"\n  Scenario: Login -- {actual_users} users"
              + (f", RPS={self.rate_limiter._target_rps}" if self.rate_limiter._target_rps > 0 else ""))

        gen = LoadGenerator(self.metrics, self.rate_limiter,
                            workers=min(actual_users, 50), duration=0)
        gen.run(login_one, total_target=actual_users)

        self._cleanup_state()
        return self.metrics.snapshot()

    def _gather_emails(self) -> List[str]:
        """Scan state files and extract user info."""
        base = os.path.dirname(__file__)
        emails = []
        for name in os.listdir(base):
            if name.startswith(".aim_state_benchreg_") and name.endswith(".json"):
                # Don't try to extract email from state files -- just return state file
                # names as markers. Actually, we need to re-register since we can't
                # extract emails from state files.
                pass
        return emails

    @staticmethod
    def _cleanup_state():
        base = os.path.dirname(__file__)
        for name in os.listdir(base):
            if name.startswith(".aim_state_") and name.endswith(".json"):
                try:
                    os.remove(os.path.join(base, name))
                except OSError:
                    pass


# ===============================================================================
# Scenario: Friend Chain
# ===============================================================================

class FriendChainScenario:
    """Full friend chain: register -> login -> add friend -> accept."""

    def __init__(self, users: int, rps: float = 0, ramp_up: float = 0):
        self.users = users
        self.metrics = MetricsCollector()
        self.rate_limiter = RateLimiter(rps, ramp_up)
        self._password = "bench123456"

    def run(self):
        pairs = self.users // 2  # N users = N/2 friend pairs
        if pairs < 1:
            print("  x Need at least 2 users for friend chain")
            return self.metrics.snapshot()

        print(f"\n  Scenario: Friend Chain -- {pairs} pairs ({self.users} users)"
              + (f", RPS={self.rate_limiter._target_rps}" if self.rate_limiter._target_rps > 0 else ""))

        # Step 1: Register all users
        print("  [1/4] Registering users...")
        user_creds: List[tuple] = []  # (email, user_id, token)
        cred_lock = threading.Lock()

        def reg_and_login():
            email = _random_email("benchfc")
            client = RESTClient()
            client.register(email, self._password, f"FC_{email[:6]}")
            client.login(email, self._password)
            with cred_lock:
                user_creds.append((email, client.token.user_id, client.token.access_token))

        gen = LoadGenerator(MetricsCollector(), RateLimiter(),
                            workers=min(self.users, 20), verbose=False)
        gen.run(reg_and_login, total_target=self.users)

        if len(user_creds) < 2:
            print("  x Not enough users registered")
            return self.metrics.snapshot()

        # Wait for Kafka sync
        print("  ... Waiting for Kafka user sync (5s)...")
        time.sleep(5)

        # Step 2: Add friends (user[i] -> user[i+1]) in pairs
        self.metrics.reset()
        print("  [2/4] Sending friend requests...")
        pair_count = len(user_creds) // 2
        pair_idx = 0
        idx_lock = threading.Lock()

        def send_friend_request():
            nonlocal pair_idx
            with idx_lock:
                if pair_idx >= pair_count:
                    return
                i = pair_idx
                pair_idx += 1

            a_email, a_id, a_token = user_creds[i * 2]
            b_email, b_id, b_token = user_creds[i * 2 + 1]

            # A sends friend request to B
            client_a = RESTClient(token=TokenManager.load())
            client_a.token.access_token = a_token
            client_a.token.user_id = a_id
            client_a.add_friend(b_id)

        gen2 = LoadGenerator(self.metrics, self.rate_limiter,
                             workers=min(pair_count, 20), duration=0)
        gen2.run(send_friend_request, total_target=pair_count)
        friend_snap = self.metrics.snapshot()

        # Step 3: Accept friends (user[i+1] accepts from user[i])
        self.metrics.reset()
        print("  [3/4] Accepting friend requests...")
        pair_idx = 0

        def accept_friend_request():
            nonlocal pair_idx
            with idx_lock:
                if pair_idx >= pair_count:
                    return
                i = pair_idx
                pair_idx += 1

            a_email, a_id, a_token = user_creds[i * 2]
            b_email, b_id, b_token = user_creds[i * 2 + 1]

            # B accepts friend request from A
            client_b = RESTClient(token=TokenManager.load())
            client_b.token.access_token = b_token
            client_b.token.user_id = b_id
            client_b.accept_friend(a_id)

        gen3 = LoadGenerator(self.metrics, self.rate_limiter,
                             workers=min(pair_count, 20), duration=0)
        gen3.run(accept_friend_request, total_target=pair_count)
        accept_snap = self.metrics.snapshot()

        # Cleanup
        self._cleanup_state()

        # Merge metrics
        merged = MetricsCollector()
        merged._total = friend_snap.total + accept_snap.total
        merged._success = friend_snap.success + accept_snap.success
        merged._errors = friend_snap.errors + accept_snap.errors
        merged._latencies = friend_snap.latencies + accept_snap.latencies
        for k, v in friend_snap.error_types.items():
            merged._error_types[k] += v
        for k, v in accept_snap.error_types.items():
            merged._error_types[k] += v

        return merged.snapshot()

    @staticmethod
    def _cleanup_state():
        base = os.path.dirname(__file__)
        for name in os.listdir(base):
            if name.startswith(".aim_state_") and name.endswith(".json"):
                try:
                    os.remove(os.path.join(base, name))
                except OSError:
                    pass


# ===============================================================================
# Scenario: WebSocket Message
# ===============================================================================

class WsMessageScenario:
    """WebSocket message send/receive benchmark.

    延迟度量：端到端延迟 (A 发送 → 服务器 → B 收到 PUSH_MESSAGE)。
    每个 conversation pair 中 A 为发送方，B 为接收方；
    通过 client_msg_id 关联发送与接收，计算 A_send_time → B_recv_time。

    支持从 msg.txt 加载预生成消息内容，确保稳定可复现。
    """

    # 接收方等待单条消息的超时时间（秒）
    RECV_TIMEOUT = 30.0

    def __init__(self, users: int, messages_per_user: int = 100,
                 rps: float = 0, ramp_up: float = 0, duration: float = 0,
                 quiet: bool = False, use_fixtures: bool = True,
                 msg_fixture_path: str = ""):
        self.users = users
        self.messages_per_user = messages_per_user
        self.metrics = MetricsCollector()
        self.rate_limiter = RateLimiter(rps, ramp_up)
        self.duration = duration
        self.quiet = quiet
        self.use_fixtures = use_fixtures
        self.msg_fixture_path = msg_fixture_path
        self._password = "bench123456"

    def run(self):
        pairs = self.users // 2
        if pairs < 1:
            print("  x Need at least 2 users for WS benchmark")
            return self.metrics.snapshot()

        print(f"\n  Scenario: WS Message (e2e) -- {pairs} pairs, {self.messages_per_user} msgs/user"
              + (f", RPS={self.rate_limiter._target_rps}" if self.rate_limiter._target_rps > 0 else ""))

        # Step 1: Register & login
        print("  [1/5] Registering & logging in...")
        user_creds: List[tuple] = []  # (user_id, access_token, refresh_token, expires_at, device_id)
        conv_pairs: List[tuple] = []  # (conv_id, a_id, a_token, a_refresh, a_expires, a_device, b_id, b_token, b_refresh, b_expires, b_device)
        cred_lock = threading.Lock()

        def reg_login_and_conv():
            email = _random_email("benchws")
            client = RESTClient()
            client.register(email, self._password, f"WS_{email[:6]}")
            client.login(email, self._password)
            uid = client.token.user_id
            token = client.token.access_token
            refresh = client.token.refresh_token
            expires = client.token.expires_at
            device = client.token.device_id
            with cred_lock:
                user_creds.append((uid, token, refresh, expires, device))

        gen = LoadGenerator(MetricsCollector(), RateLimiter(),
                            workers=min(self.users, 20), verbose=False)
        gen.run(reg_login_and_conv, total_target=self.users)

        if len(user_creds) < 2:
            print("  x Not enough users registered")
            return self.metrics.snapshot()

        # Wait for Kafka sync
        print("  ... Waiting for Kafka user sync (5s)...")
        time.sleep(5)

        # Step 2: Create conversations (pair users)
        print("  [2/5] Creating conversations...")
        conv_lock = threading.Lock()

        def create_conv():
            if not user_creds:
                return
            with conv_lock:
                idx = len(conv_pairs)
            if idx * 2 + 1 >= len(user_creds):
                return
            a_id, a_token, a_refresh, a_expires, a_device = user_creds[idx * 2]
            b_id, b_token, b_refresh, b_expires, b_device = user_creds[idx * 2 + 1]
            client = RESTClient(token=TokenManager.load())
            client.token.access_token = a_token
            client.token.user_id = a_id
            resp = client.create_conversation([b_id])
            conv_id = resp["conversation_id"]
            with conv_lock:
                conv_pairs.append((conv_id, a_id, a_token, a_refresh, a_expires, a_device,
                                   b_id, b_token, b_refresh, b_expires, b_device))

        gen2 = LoadGenerator(MetricsCollector(), RateLimiter(),
                             workers=min(pairs, 20), verbose=False)
        gen2.run(create_conv, total_target=pairs)

        if not conv_pairs:
            print("  x No conversations created")
            return self.metrics.snapshot()

        # Step 3: Connect receiver WS clients (B side)
        print("  [3/5] Connecting receiver WebSocket clients...")

        # pending_sends: client_msg_id → threading.Event
        # send_times:    client_msg_id → send_timestamp
        pending_sends: Dict[str, threading.Event] = {}
        send_times: Dict[str, float] = {}
        pending_lock = threading.Lock()
        # Track messages that timed out or were orphaned
        timed_out_count = {"value": 0}

        receiver_ws_clients: List[tuple] = []  # (conv_id, receiver_user_id, WSClient)

        for i, (conv_id, a_id, a_token, a_refresh, a_expires, a_device,
                 b_id, b_token, b_refresh, b_expires, b_device) in enumerate(conv_pairs):
            token_mgr = TokenManager.load()
            token_mgr.access_token = b_token
            token_mgr.refresh_token = b_refresh
            token_mgr.expires_at = b_expires
            token_mgr.device_id = b_device
            token_mgr.user_id = b_id
            rest = RESTClient(token=token_mgr)
            ws = WSClient(token=token_mgr, rest_client=rest)

            # 注册 on_frame 回调，接收方监听 PUSH_MESSAGE
            def make_on_frame(receiver_uid: int):
                def on_frame(frame, payload):
                    if frame.type == ws_pb2.FRAME_TYPE_PUSH_MESSAGE and payload is not None:
                        msg_id = getattr(payload, "client_msg_id", "")
                        if msg_id:
                            recv_time = time.time()
                            with pending_lock:
                                t0 = send_times.pop(msg_id, None)
                                evt = pending_sends.pop(msg_id, None)
                            if t0 is not None:
                                latency = recv_time - t0
                                self.metrics.record_success(latency)
                                if evt:
                                    evt.set()
                return on_frame

            ws.on_frame = make_on_frame(b_id)
            ws.connect()
            if ws.is_connected():
                receiver_ws_clients.append((conv_id, b_id, ws))
            else:
                _vprint(f"    x Receiver WS connection #{i} failed")

        active_receivers = len(receiver_ws_clients)
        print(f"    {active_receivers}/{pairs} receiver WS connections established")

        # Step 4: Connect sender WS clients (A side)
        print("  [4/5] Connecting sender WebSocket clients...")
        sender_ws_clients: List[tuple] = []  # (conv_id, WSClient)

        # 构建 conv_id → receiver 映射，方便查找哪些 conv 有活跃接收方
        active_conv_ids = {c for c, _, _ in receiver_ws_clients}

        for conv_id, a_id, a_token, a_refresh, a_expires, a_device, \
                b_id, b_token, b_refresh, b_expires, b_device in conv_pairs:
            if conv_id not in active_conv_ids:
                _vprint(f"    x Skipping sender for conv {conv_id} (no receiver)")
                continue
            token_mgr = TokenManager.load()
            token_mgr.access_token = a_token
            token_mgr.refresh_token = a_refresh
            token_mgr.expires_at = a_expires
            token_mgr.device_id = a_device
            token_mgr.user_id = a_id
            rest = RESTClient(token=token_mgr)
            ws = WSClient(token=token_mgr, rest_client=rest)
            ws.connect()
            if ws.is_connected():
                sender_ws_clients.append((conv_id, ws))
            else:
                _vprint(f"    x Sender WS connection for conv {conv_id} failed")

        active_senders = len(sender_ws_clients)
        print(f"    {active_senders}/{pairs} sender WS connections established")

        if not sender_ws_clients:
            print("  x No sender WS connections established")
            for _, _, ws in receiver_ws_clients:
                ws.disconnect()
            return self.metrics.snapshot()

        # 加载消息 fixture
        msg_fixtures = []
        if self.use_fixtures:
            msg_fixtures = _load_msg_fixtures(self.msg_fixture_path)
            if msg_fixtures:
                print(f"  Loaded {len(msg_fixtures)} message fixtures from msg.txt")

        # Step 5: Send messages & measure e2e latency
        total_expected = active_senders * self.messages_per_user
        print(f"  [5/5] Sending messages ({total_expected} total, e2e latency)..."
              + (f", fixtures={'on' if msg_fixtures else 'off'}" if self.use_fixtures else ""))
        self.metrics.reset()

        msg_idx = 0
        idx_lock = threading.Lock()
        # 重连锁：每个 sender 独立，避免多个 worker 同时重连同一个 WS
        reconnect_locks = [threading.Lock() for _ in sender_ws_clients]

        def send_one():
            nonlocal msg_idx
            with idx_lock:
                if msg_idx >= total_expected:
                    return _SKIP_METRICS
                msg_idx += 1
            # Round-robin across senders
            sender_idx = (msg_idx - 1) % len(sender_ws_clients)
            conv_id, ws = sender_ws_clients[sender_idx]
            # 优先使用 fixture 数据
            if msg_fixtures:
                content = msg_fixtures[(msg_idx - 1) % len(msg_fixtures)]
            else:
                content = f"bench_msg_{msg_idx}"

            # 尝试发送，断线时重连重试
            max_send_retries = 2  # 最多重试 2 次（含首次共 3 次机会）
            for attempt in range(max_send_retries + 1):
                try:
                    # 注册 pending 追踪，再发送
                    evt = threading.Event()
                    t0 = time.time()
                    client_msg_id = ws.send_message(conv_id, content, "text")
                    with pending_lock:
                        send_times[client_msg_id] = t0
                        pending_sends[client_msg_id] = evt

                    # 等待接收方确认（带超时）
                    if not evt.wait(timeout=self.RECV_TIMEOUT):
                        # 超时：清理并记录错误
                        with pending_lock:
                            send_times.pop(client_msg_id, None)
                            pending_sends.pop(client_msg_id, None)
                        timed_out_count["value"] += 1
                        latency = time.time() - t0
                        self.metrics.record_error(latency, "recv_timeout")

                    # 返回 _SKIP_METRICS: 本函数自行管理 record_success/record_error
                    return _SKIP_METRICS

                except RuntimeError as e:
                    if "Not connected" not in str(e) or attempt >= max_send_retries:
                        # 非断线 RuntimeError 或重试耗尽，记录错误
                        latency = time.time() - t0
                        self.metrics.record_error(latency, "ws_disconnected")
                        return _SKIP_METRICS
                    # 断线：尝试重连
                    with reconnect_locks[sender_idx]:
                        if not ws.is_connected():
                            ws.reconnect(max_retries=1)
                    # 重连后继续循环重试

        gen3 = LoadGenerator(self.metrics, self.rate_limiter,
                             workers=min(active_senders, 20),
                             duration=self.duration)
        with MuteContext(self.quiet):
            gen3.run(send_one, total_target=total_expected)

        # If muted, print summary after unmute
        if self.quiet:
            snap = self.metrics.snapshot()
            print(ReportPrinter.summary(snap))

        # 清理残留 pending（压测 duration 模式可能中途中断）
        with pending_lock:
            remaining = len(pending_sends)
            if remaining > 0:
                for mid in list(pending_sends.keys()):
                    t0 = send_times.pop(mid, time.time())
                    latency = time.time() - t0
                    self.metrics.record_error(latency, "orphaned")
                pending_sends.clear()

        if timed_out_count["value"] > 0 or remaining > 0:
            print(f"  ⚠ {timed_out_count['value']} timed out, {remaining} orphaned messages")

        # Disconnect all
        for _, ws in sender_ws_clients:
            ws.disconnect()
        for _, _, ws in receiver_ws_clients:
            ws.disconnect()

        # Cleanup
        self._cleanup_state()
        return self.metrics.snapshot()

    @staticmethod
    def _cleanup_state():
        base = os.path.dirname(__file__)
        for name in os.listdir(base):
            if name.startswith(".aim_state_") and name.endswith(".json"):
                try:
                    os.remove(os.path.join(base, name))
                except OSError:
                    pass


# ===============================================================================
# Scenario: Mixed (REST + WS)
# ===============================================================================

class MixedScenario:
    """Mixed workload: REST queries + WS messages simultaneously."""

    def __init__(self, users: int, duration: float = 30,
                 rps: float = 0, ramp_up: float = 0,
                 ws_ratio: float = 0.7, quiet: bool = False):
        self.users = users
        self.duration = duration
        self.rps = rps
        self.ramp_up = ramp_up
        self.ws_ratio = ws_ratio
        self.quiet = quiet
        self.metrics = MetricsCollector()
        self._password = "bench123456"

    def run(self):
        pairs = self.users // 2
        if pairs < 1:
            pairs = 1
            self.users = 2

        print(f"\n  Scenario: Mixed -- {self.users} users, {self.duration}s duration"
              + f", WS ratio={self.ws_ratio:.0%}")

        # Setup: register, login, create conversations
        print("  [Setup] Registering users & creating conversations...")
        user_creds: List[tuple] = []
        conv_pairs: List[tuple] = []
        cred_lock = threading.Lock()

        def setup_user():
            email = _random_email("benchmx")
            client = RESTClient()
            client.register(email, self._password, f"MX_{email[:6]}")
            client.login(email, self._password)
            uid = client.token.user_id
            token = client.token.access_token
            refresh = client.token.refresh_token
            expires = client.token.expires_at
            device = client.token.device_id
            with cred_lock:
                user_creds.append((uid, token, refresh, expires, device))

        gen = LoadGenerator(MetricsCollector(), RateLimiter(),
                            workers=min(self.users, 20), verbose=False)
        gen.run(setup_user, total_target=self.users)

        time.sleep(5)  # Kafka sync

        # Create conversations
        conv_lock = threading.Lock()

        def create_conv():
            with conv_lock:
                idx = len(conv_pairs)
            if idx * 2 + 1 >= len(user_creds):
                return
            a_id, a_token, a_refresh, a_expires, a_device = user_creds[idx * 2]
            b_id, b_token, b_refresh, b_expires, b_device = user_creds[idx * 2 + 1]
            client = RESTClient(token=TokenManager.load())
            client.token.access_token = a_token
            client.token.user_id = a_id
            resp = client.create_conversation([b_id])
            with conv_lock:
                conv_pairs.append((resp["conversation_id"], a_id, a_token, a_refresh, a_expires, a_device,
                                   b_id, b_token, b_refresh, b_expires, b_device))

        gen2 = LoadGenerator(MetricsCollector(), RateLimiter(),
                             workers=min(pairs, 20), verbose=False)
        gen2.run(create_conv, total_target=pairs)

        if not conv_pairs:
            print("  x No conversations created")
            return self.metrics.snapshot()

        # Connect WS clients
        print("  [Setup] Connecting WebSocket clients...")
        ws_clients: List[tuple] = []  # (conv_id, ws)
        for conv_id, a_id, a_token, a_refresh, a_expires, a_device, \
                b_id, b_token, b_refresh, b_expires, b_device in conv_pairs:
            token_mgr = TokenManager.load()
            token_mgr.access_token = a_token
            token_mgr.refresh_token = a_refresh
            token_mgr.expires_at = a_expires
            token_mgr.device_id = a_device
            token_mgr.user_id = a_id
            rest = RESTClient(token=token_mgr)
            ws = WSClient(token=token_mgr, rest_client=rest)
            ws.connect()
            if ws.is_connected():
                ws_clients.append((conv_id, ws))

        print(f"    {len(ws_clients)} WS connections established")

        # Run mixed workload
        print(f"  [Run] Mixed workload for {self.duration}s...")
        self.metrics.reset()

        ws_counter = 0
        rest_counter = 0
        counter_lock = threading.Lock()
        rate_limiter = RateLimiter(self.rps, self.ramp_up)

        def mixed_task():
            nonlocal ws_counter, rest_counter
            wait = rate_limiter.acquire()
            if wait > 0:
                time.sleep(wait)

            import random
            if random.random() < self.ws_ratio and ws_clients:
                # WS send（带断线重连）
                with counter_lock:
                    ws_counter += 1
                    idx = ws_counter % len(ws_clients)
                conv_id, ws = ws_clients[idx]
                content = f"mx_{ws_counter}"
                try:
                    ws.send_message(conv_id, content, "text")
                except RuntimeError:
                    # 断线：尝试重连一次，重连失败则忽略此条（_worker_loop 会记录 RuntimeError）
                    try:
                        ws.reconnect(max_retries=1)
                        ws.send_message(conv_id, content, "text")
                    except RuntimeError:
                        raise  # 重连也失败，让 _worker_loop 记录错误
            else:
                # REST query -- pick a random user and search/list
                with counter_lock:
                    rest_counter += 1
                    idx = rest_counter % len(conv_pairs)
                conv_id, a_id, a_token, b_id, b_token = conv_pairs[idx]
                client = RESTClient(token=TokenManager.load())
                client.token.access_token = a_token
                client.token.user_id = a_id
                # Alternate between different REST calls
                op = rest_counter % 3
                if op == 0:
                    client.list_friends()
                elif op == 1:
                    client.list_conversations()
                else:
                    client.get_history(conv_id, limit=5)

        gen3 = LoadGenerator(self.metrics, rate_limiter,
                             workers=min(len(ws_clients) + 5, 30),
                             duration=self.duration)
        with MuteContext(self.quiet):
            gen3.run(mixed_task, total_target=0)

        # If muted, print summary after unmute
        if self.quiet:
            snap = self.metrics.snapshot()
            print(ReportPrinter.summary(snap))

        # Disconnect
        for conv_id, ws in ws_clients:
            ws.disconnect()

        self._cleanup_state()
        return self.metrics.snapshot()

    @staticmethod
    def _cleanup_state():
        base = os.path.dirname(__file__)
        for name in os.listdir(base):
            if name.startswith(".aim_state_") and name.endswith(".json"):
                try:
                    os.remove(os.path.join(base, name))
                except OSError:
                    pass


# ===============================================================================
# CLI
# ===============================================================================

def _add_common_args(parser: argparse.ArgumentParser):
    parser.add_argument("--users", type=int, default=100,
                        help="Number of concurrent users (default: 100)")
    parser.add_argument("--rps", type=float, default=0,
                        help="Target requests per second (default: 0 = unlimited)")
    parser.add_argument("--duration", type=float, default=0,
                        help="Duration in seconds (default: 0 = run until complete)")
    parser.add_argument("--ramp-up", type=float, default=0,
                        help="Ramp-up time in seconds (default: 0)")
    parser.add_argument("--output", type=str, default="",
                        help="Output JSON report path")
    parser.add_argument("--quiet", action="store_true",
                        help="Suppress verbose WS frame output during benchmarks")
    parser.add_argument("--no-fixtures", action="store_true",
                        help="Disable fixture files (user.json/msg.txt), use random data")
    parser.add_argument("--gateway", type=str, default="",
                        help="Gateway HTTP URL (default: auto-detect, benchmark env: http://127.0.0.1:18888)")


def _save_report(snap: MetricsSnapshot, path: str, title: str = "benchmark"):
    """Save metrics snapshot as JSON file."""
    data = ReportPrinter.to_json(snap, title)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
    print(f"  Report saved to {path}")


class MuteContext:
    """Context manager to suppress noisy output during benchmarks.
    Redirects stdout (and optionally stderr) to devnull during the context.
    """

    def __init__(self, mute: bool = True, mute_stderr: bool = False):
        self._mute = mute
        self._mute_stderr = mute_stderr
        self._saved_stdout = None
        self._saved_stderr = None
        self._devnull = None

    def __enter__(self):
        if self._mute:
            self._devnull = open(os.devnull, 'w')
            self._saved_stdout = sys.stdout
            sys.stdout = self._devnull
            if self._mute_stderr:
                self._saved_stderr = sys.stderr
                sys.stderr = self._devnull
        return self

    def __exit__(self, *args):
        if self._mute:
            if self._saved_stdout is not None:
                sys.stdout = self._saved_stdout
            if self._saved_stderr is not None:
                sys.stderr = self._saved_stderr
            if self._devnull is not None:
                try:
                    self._devnull.close()
                except Exception:
                    pass
        return False


def cmd_register(args):
    _apply_quiet(getattr(args, "quiet", False))
    use_fixtures = not getattr(args, "no_fixtures", False)
    gateway = getattr(args, "gateway", "")
    if gateway:
        import aim_test
        aim_test.GATEWAY_HTTP = gateway
    scenario = RegisterScenario(args.users, args.rps, args.ramp_up, use_fixtures=use_fixtures)
    snap = scenario.run()
    if args.output:
        _save_report(snap, args.output, "register")


def cmd_login(args):
    _apply_quiet(getattr(args, "quiet", False))
    scenario = LoginScenario(args.users, args.rps, args.ramp_up)
    snap = scenario.run()
    if args.output:
        _save_report(snap, args.output, "login")


def cmd_friend_chain(args):
    _apply_quiet(getattr(args, "quiet", False))
    scenario = FriendChainScenario(args.users, args.rps, args.ramp_up)
    snap = scenario.run()
    if args.output:
        _save_report(snap, args.output, "friend-chain")


def cmd_ws_message(args):
    msgs = getattr(args, "messages_per_user", 100)
    quiet = getattr(args, "quiet", False)
    _apply_quiet(quiet)
    use_fixtures = not getattr(args, "no_fixtures", False)
    gateway = getattr(args, "gateway", "")
    if gateway:
        import aim_test
        aim_test.GATEWAY_HTTP = gateway
        aim_test.GATEWAY_WS = gateway.replace("http://", "ws://") + "/ws"
    scenario = WsMessageScenario(args.users, msgs, args.rps, args.ramp_up, args.duration,
                                  quiet=quiet, use_fixtures=use_fixtures)
    snap = scenario.run()
    if args.output:
        _save_report(snap, args.output, "ws-message")


def cmd_mixed(args):
    ws_ratio = getattr(args, "ws_ratio", 0.7)
    quiet = getattr(args, "quiet", False)
    _apply_quiet(quiet)
    gateway = getattr(args, "gateway", "")
    if gateway:
        import aim_test
        aim_test.GATEWAY_HTTP = gateway
        aim_test.GATEWAY_WS = gateway.replace("http://", "ws://") + "/ws"
    scenario = MixedScenario(args.users, args.duration, args.rps, args.ramp_up, ws_ratio,
                              quiet=quiet)
    snap = scenario.run()
    if args.output:
        _save_report(snap, args.output, "mixed")


def main():
    parser = argparse.ArgumentParser(
        description="AIM Benchmark Tool -- Load & Stress Testing",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python benchmark.py register --users 100
  python benchmark.py register --users 500 --rps 50 --ramp-up 10
  python benchmark.py register --users 1000 --gateway http://127.0.0.1:18888
  python benchmark.py login --users 100
  python benchmark.py friend-chain --users 50
  python benchmark.py ws-message --users 20 --messages-per-user 100
  python benchmark.py ws-message --users 50 --messages-per-user 500 --rps 1000
  python benchmark.py ws-message --users 50 --messages-per-user 500 --no-fixtures
  python benchmark.py mixed --users 50 --duration 30
  python benchmark.py mixed --users 100 --duration 60 --rps 200 --output report.json

Benchmark Environment:
  docker compose up -d    # Start isolated benchmark env (port +10000)
  docker compose down -v  # Stop and clean up
        """
    )
    sub = parser.add_subparsers(dest="command", help="Benchmark scenarios")

    # register
    p = sub.add_parser("register", help="Bulk user registration benchmark")
    _add_common_args(p)

    # login
    p = sub.add_parser("login", help="Bulk user login benchmark")
    _add_common_args(p)

    # friend-chain
    p = sub.add_parser("friend-chain", help="Friend chain benchmark (register -> login -> add -> accept)")
    _add_common_args(p)

    # ws-message
    p = sub.add_parser("ws-message", help="WebSocket message benchmark")
    _add_common_args(p)
    p.add_argument("--messages-per-user", type=int, default=100,
                   help="Messages per user (default: 100)")

    # mixed
    p = sub.add_parser("mixed", help="Mixed REST + WS benchmark")
    _add_common_args(p)
    p.add_argument("--ws-ratio", type=float, default=0.7,
                   help="Ratio of WS requests (default: 0.7)")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return

    commands = {
        "register": cmd_register,
        "login": cmd_login,
        "friend-chain": cmd_friend_chain,
        "ws-message": cmd_ws_message,
        "mixed": cmd_mixed,
    }

    handler = commands.get(args.command)
    if handler:
        handler(args)


if __name__ == "__main__":
    main()
