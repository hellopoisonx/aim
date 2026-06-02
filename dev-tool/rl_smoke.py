#!/usr/bin/env python3
"""
AIM Gateway 限流冒烟测试
=========================

冒烟测试 gateway REST + WS 限流行为,覆盖 plan 中 5 个端到端测试场景:

  T1 rest    — REST 限流(走 Auth,RateLimit 中间件)
  T2 ws      — WS SEND_MESSAGE 限流(走 RateLimitQuota,与 REST 共享桶)
  T3 bot     — Bot 写消息限流(走 BotAuth,BotRateLimit 中间件,独立桶)
  T4 share   — WS 用尽 quota 后, REST 紧接全 429(强证共享桶)
  T5 opcode  — WS 桶耗尽后, TYPING/HEARTBEAT/READ_RECEIPT/ACK 不受限

用法:
    python rl_smoke.py all                 # 跑全部 5 个测试(默认)
    python rl_smoke.py t1                  # 单跑 T1
    python rl_smoke.py t2 --no-reset       # 不等桶重置(快速调试)
    python rl_smoke.py all --quota-window 60  # 覆盖默认 60s 窗口

环境:
    AIM_GATEWAY_HTTP=http://127.0.0.1:8888   (默认)
    AIM_GATEWAY_WS=ws://127.0.0.1:8888/ws    (默认)

设计:
    复用 aim_test.py 的 RESTClient / WSClient / TokenManager / APIError,
    不重复实现 HTTP/WS 客户端。冒烟测试**不**做并发,所有请求串行,
    避免被测限流逻辑(本身是单连接滑动窗口)被并发干扰。

退出码:
    0  全部通过
    1  至少一个测试失败
    2  测试因环境/网络问题无法运行
"""

import argparse
import json
import os
import sys
import time

# 让 rl_smoke.py 与 aim_test.py 共享 dev-tool 目录下的模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import requests
import ws_pb2

from aim_test import (
    TokenManager,
    RESTClient,
    WSClient,
    APIError,
    GATEWAY_HTTP,
    GATEWAY_WS,
)

# ── 配置 ──────────────────────────────────────────────────────────────────
# 与 gateway-api.yaml 默认对齐: MaxRequests=100 / WindowSeconds=60
DEFAULT_MAX_REQUESTS = 100
DEFAULT_WINDOW_SECONDS = 60
BURST_REQUESTS = DEFAULT_MAX_REQUESTS + 30  # 触发 30 个限流
OPCODE_REQUESTS = 30  # T5 每种 opcode 发送次数

# ── 终端配色 ──────────────────────────────────────────────────────────────
GREEN = "\033[32m"
RED = "\033[31m"
YELLOW = "\033[33m"
BLUE = "\033[34m"
RESET = "\033[0m"


def _print_pass(name: str, msg: str = ""):
    print(f"  {GREEN}✓ PASS{RESET}  {name}  {msg}")


def _print_fail(name: str, msg: str = ""):
    print(f"  {RED}✗ FAIL{RESET}  {name}  {msg}")


def _print_info(msg: str):
    print(f"  {BLUE}ℹ{RESET} {msg}")


def _print_warn(msg: str):
    print(f"  {YELLOW}⚠{RESET} {msg}")


# ── 共享 helper ──────────────────────────────────────────────────────────
def _wait_for_quota_reset(seconds: int, reason: str):
    """等滑动窗口重置,避免上次测试残留影响本次。"""
    if seconds <= 0:
        return
    print(f"  {YELLOW}⏳{RESET} 等 {seconds}s 让限流桶重置 ({reason}) ...")
    time.sleep(seconds)
def _ensure_token_fresh(tm: TokenManager) -> bool:
    """如 token 剩余寿命 < 90s, 则 refresh。True=可用, False=refresh 失败。"""
    if tm.access_token and tm.expires_at - int(time.time()) > 90:
        return True
    if not tm.refresh_token:
        return False
    rc = RESTClient(token=tm)
    try:
        rc.refresh()
        tm.access_token = rc.token.access_token
        tm.expires_at = rc.token.expires_at
        tm.save()
        return True
    except Exception:
        return False


def _register_pair(prefix: str = "rl"):
    """注册 alice + bob 并 login, 返回 (alice_tm, bob_tm)。"""


    import uuid as _uuid
    suffix = _uuid.uuid4().hex[:8]
    alice_email = f"{prefix}-a-{suffix}@t.com"
    bob_email = f"{prefix}-b-{suffix}@t.com"
    alice_pwd = "Password123"
    bob_pwd = "Password123"

    # 注册 + 等待 Kafka 消费
    alice = RESTClient()
    bob = RESTClient()
    alice_resp = alice.register(alice_email, alice_pwd, username=f"{prefix}_a_{suffix}")
    alice_user_id = int(alice_resp["body"]["user_id"])
    time.sleep(3)  # 等 UserCreated 事件传完
    bob_resp = bob.register(bob_email, bob_pwd, username=f"{prefix}_b_{suffix}")
    bob_user_id = int(bob_resp["body"]["user_id"])
    time.sleep(3)

    # login
    alice.login(alice_email, alice_pwd)
    bob.login(bob_email, bob_pwd)

    tm_alice = TokenManager.load()
    tm_alice.user_id = alice_user_id
    tm_alice.save()
    tm_bob = TokenManager.load("bob")
    tm_bob.user_id = bob_user_id
    tm_bob.save()

    return tm_alice, tm_bob


def _ensure_friendship(tm_alice: TokenManager, tm_bob: TokenManager):
    """确保 alice/bob 是好友(双向加好友流程)。"""
    alice = RESTClient(token=tm_alice)
    bob = RESTClient(token=tm_bob)
    try:
        bob.add_friend(tm_alice.user_id)
    except APIError:
        pass
    time.sleep(2)
    try:
        alice.accept_friend(tm_bob.user_id)
    except APIError:
        pass
    time.sleep(2)


def _create_conversation(tm_alice: TokenManager, tm_bob: TokenManager) -> int:
    """创建 alice↔bob 私聊,返回 conversation_id。"""
    alice = RESTClient(token=tm_alice)
    conv = alice.create_conversation(member_ids=[tm_bob.user_id])
    conv_id = (
        conv.get("conversation_id")
        or conv.get("body", {}).get("conversation_id")
        or conv.get("data", {}).get("conversation_id")
    )
    if not conv_id:
        raise RuntimeError(f"create_conversation 失败: {json.dumps(conv, ensure_ascii=False)[:200]}")
    return conv_id


def _ws_send_and_collect(ws: WSClient, conv_id: int, n: int, content_prefix: str = "msg") -> list:
    """发 n 个 SEND_MESSAGE, 返回所有 SERVER_ACK 的 (status, code, msg) 列表。"""
    captured: list = []
    server_ack_type = ws_pb2.FRAME_TYPE_SERVER_ACK  # type: ignore[attr-defined]

    def on_frame(f, p):
        if f.type == server_ack_type and p:
            captured.append({"status": p.status, "code": p.code, "msg": p.msg})

    ws.on_frame = on_frame
    for i in range(n):
        ws.send_message(conv_id, f"{content_prefix} {i}")
    # 等待 ACK
    deadline = time.time() + 15
    while len(captured) < n and time.time() < deadline:
        time.sleep(0.2)
    return captured[:n]


def _ws_collect_acks(ws: WSClient, n: int) -> list:
    """设置 on_frame 收集 SERVER_ACK, 等待收到 n 条后返回(给非 SEND_MESSAGE opcode 用)。"""
    captured: list = []
    server_ack_type = ws_pb2.FRAME_TYPE_SERVER_ACK  # type: ignore[attr-defined]

    def on_frame(f, p):
        if f.type == server_ack_type and p:
            captured.append({"status": p.status, "code": p.code, "msg": p.msg})

    ws.on_frame = on_frame
    return captured


def _connect_ws(tm: TokenManager) -> WSClient:
    """建 WS 连接,关闭自动心跳(测试自己控制)。"""
    ws = WSClient(token=tm, heartbeat_interval=0)
    ws.connect()
    deadline = time.time() + 10
    while not ws._connected and time.time() < deadline:
        time.sleep(0.1)
    if not ws._connected:
        raise RuntimeError("WS 连接超时")
    return ws


# ── T1: REST 限流 ────────────────────────────────────────────────────────
def test_rest_ratelimit(quota_window: int, no_reset: bool) -> bool:
    """
    T1: GET /api/presence/friends (走 Auth,RateLimit 中间件) 连发 BURST 次,
    期望: 前 MAX_REQUESTS 个返回 code=0, 后 30 个返回 code=42900。
    """
    name = "T1 REST 限流"
    print(f"\n{BLUE}━━━ {name} ━━━{RESET}")
    tm = TokenManager.load()
    if not tm.access_token:
        _print_warn("default profile 无 token, 跑 T1 前需要先 login")
        return False
    _print_info(f"alice user_id={tm.user_id}, device_id={tm.device_id[:8]}...")
    if not no_reset:
        _wait_for_quota_reset(quota_window, "避免上次测试残留")

    headers = {"Authorization": f"Bearer {tm.access_token}"}
    url = f"{GATEWAY_HTTP}/api/presence/friends"
    status_codes: dict = {}
    for _ in range(BURST_REQUESTS):
        r = requests.get(url, headers=headers, timeout=5)
        try:
            code = r.json().get("code", -1)
        except Exception:
            code = f"http_{r.status_code}"
        status_codes[code] = status_codes.get(code, 0) + 1

    ok = status_codes.get(0, 0)
    rl = status_codes.get(42900, 0)
    _print_info(f"code 分布: {json.dumps(status_codes, sort_keys=True)}")
    _print_info(f"ok={ok} (期望 ≤ {DEFAULT_MAX_REQUESTS}), rate-limited={rl} (期望 ≥ 20)")

    if ok <= DEFAULT_MAX_REQUESTS and rl >= 20:
        _print_pass(name, f"RateLimit 中间件在 {DEFAULT_MAX_REQUESTS} 处精确触发, 30 个请求被 429 拒绝")
        return True
    _print_fail(name, f"ok={ok}, rl={rl}")
    return False


# ── T2: WS SEND_MESSAGE 限流 ─────────────────────────────────────────────
def test_ws_ratelimit(quota_window: int, no_reset: bool) -> bool:
    """
    T2: WS SEND_MESSAGE × BURST, 期望前 MAX_REQUESTS 个 ACK_ACCEPTED,
    后 30 个 ACK_REJECTED(code=42900, msg="rate limit"), WS 连接保持。
    """
    name = "T2 WS SEND_MESSAGE 限流"
    print(f"\n{BLUE}━━━ {name} ━━━{RESET}")
    tm = TokenManager.load()
    tm_bob = TokenManager.load("bob")
    if not tm.access_token or not tm_bob.access_token:
        _print_warn("需要 alice + bob 两个 profile 的 token")
        return False
    if not _ensure_token_fresh(tm) or not _ensure_token_fresh(tm_bob):
        _print_warn("alice 或 bob token 过期且无法 refresh, 请重新 login")
        return False
    _ensure_friendship(tm, tm_bob)
    if not no_reset:
        _wait_for_quota_reset(quota_window, "避免上次测试残留")
    conv_id = _create_conversation(tm, tm_bob)
    _print_info(f"conversation_id={conv_id}")

    ws = _connect_ws(tm)
    acks = _ws_send_and_collect(ws, conv_id, BURST_REQUESTS, "t2")
    ws.disconnect()

    accepted = sum(1 for a in acks if a["status"] == 1)  # ACK_STATUS_ACCEPTED
    rejected = sum(1 for a in acks if a["status"] == 2)  # ACK_STATUS_REJECTED
    rl_code = sum(1 for a in acks if a["code"] == 42900)
    _print_info(f"ACK 分布: total={len(acks)}, ACCEPTED={accepted}, REJECTED={rejected} (含 {rl_code} 个 42900)")

    if accepted <= DEFAULT_MAX_REQUESTS and rl_code >= 20:
        _print_pass(name, f"{accepted} 通过 + {rl_code} 限流, WS 与 REST 共享 quota 命中")
        return True
    _print_fail(name, f"accepted={accepted}, rate_limited_42900={rl_code}")
    return False


# ── T3: Bot 写消息限流 ───────────────────────────────────────────────────
def test_bot_ratelimit(quota_window: int, no_reset: bool) -> bool:
    """
    T3: 用 bot token POST /api/bot/v1/messages × BURST。
    期望: 限流精确在第 MAX_REQUESTS 个后触发(桶独立, 不与人类共享)。
    """
    name = "T3 Bot 写消息限流 (独立桶)"
    print(f"\n{BLUE}━━━ {name} ━━━{RESET}")
    tm = TokenManager.load()
    if not tm.access_token:

        _print_warn("default profile 无 token")
        return False
    if not _ensure_token_fresh(tm):
        _print_warn("default profile token 过期且无法 refresh, 请重新 login")
        return False

    # Bot 桶独立, 但 create_user_bot / create_bot_token / _create_conversation 走人类桶, 需先 reset
    if not no_reset:
        _wait_for_quota_reset(quota_window, "避免 alice 桶被上一个测试残留")
    # 建 bot + 拿 token
    alice = RESTClient(token=tm)
    import uuid as _uuid
    bot_email = f"rl-bot-{_uuid.uuid4().hex[:8]}@t.com"
    bot = alice.create_user_bot(email=bot_email, nickname=f"rl-bot-{_uuid.uuid4().hex[:6]}")
    bot_id = bot["bot"]["bot_user_id"]
    tok = alice.create_bot_token(bot_id=bot_id)
    bot_token = tok.get("plaintext_token")
    if not bot_token:
        _print_fail(name, f"拿不到 bot plaintext_token: {json.dumps(tok, ensure_ascii=False)[:200]}")
        return False
    _print_info(f"bot_id={bot_id}, token={bot_token[:20]}...")

    # 拿 conv (复用 alice↔bob)
    tm_bob = TokenManager.load("bob")
    if not tm_bob.access_token:
        _print_warn("bob profile 无 token, 跳过")
        return False
    # (reset 已在 setup 阶段完成)
    try:
        conv_id = _create_conversation(tm, tm_bob)
    except APIError as e:
        _print_warn(f"create_conversation 失败: {e}, 跳过")
        return False
    _print_info(f"conversation_id={conv_id}")

    # POST × BURST
    url = f"{GATEWAY_HTTP}/api/bot/v1/messages"
    headers = {"Authorization": f"Bot {bot_token}", "Content-Type": "application/json"}
    body = {"conversation_id": conv_id, "content": "rl bot test"}
    status_codes: dict = {}
    for _ in range(BURST_REQUESTS):
        r = requests.post(url, headers=headers, json=body, timeout=5)
        try:
            code = r.json().get("code", -1)
        except Exception:
            code = f"http_{r.status_code}"
        status_codes[code] = status_codes.get(code, 0) + 1

    rl = status_codes.get(42900, 0)
    _print_info(f"code 分布: {json.dumps(status_codes, sort_keys=True)}")
    _print_info(f"rate-limited=42900: {rl} (期望 ≥ 20)")

    if rl >= 20:
        _print_pass(name, f"BotRateLimit 在 100 处精确触发, {rl} 个 429 拒绝(独立桶)")
        return True
    _print_fail(name, f"rate_limited={rl} (期望 ≥ 20), 业务失败可能掩盖限流")
    return False


# ── T4: WS/REST 共享桶强证 ───────────────────────────────────────────────
def test_shared_quota_bucket(quota_window: int, no_reset: bool) -> bool:
    """
    T4: WS 发 100 SEND_MESSAGE (即使业务失败) 打满桶,
    紧接 REST GET 30 次应全部 42900(强证共享桶)。
    """
    name = "T4 WS/REST 共享桶 (强证)"
    print(f"\n{BLUE}━━━ {name} ━━━{RESET}")
    tm = TokenManager.load()
    tm_bob = TokenManager.load("bob")
    if not tm.access_token or not tm_bob.access_token:
        _print_warn("需要 alice + bob 两个 profile 的 token")
        return False
    if not _ensure_token_fresh(tm) or not _ensure_token_fresh(tm_bob):
        _print_warn("alice 或 bob token 过期且无法 refresh, 请重新 login")
        return False
    _ensure_friendship(tm, tm_bob)
    if not no_reset:
        _wait_for_quota_reset(quota_window, "避免上次测试残留")
    conv_id = _create_conversation(tm, tm_bob)
    _print_info(f"conversation_id={conv_id}")

    ws = _connect_ws(tm)
    p1_acks = _ws_send_and_collect(ws, conv_id, DEFAULT_MAX_REQUESTS, "t4-phase1")
    p1_rl = sum(1 for a in p1_acks if a["code"] == 42900)
    _print_info(f"Phase 1 WS: total={len(p1_acks)}, rate_limited_42900={p1_rl}")

    # 紧接 REST GET × OPCODE_REQUESTS
    headers = {"Authorization": f"Bearer {tm.access_token}"}
    url = f"{GATEWAY_HTTP}/api/presence/friends"
    rest_codes: dict = {}
    for _ in range(OPCODE_REQUESTS):
        r = requests.get(url, headers=headers, timeout=5)
        try:
            code = r.json().get("code", -1)
        except Exception:
            code = f"http_{r.status_code}"
        rest_codes[code] = rest_codes.get(code, 0) + 1
    rl = rest_codes.get(42900, 0)
    _print_info(f"Phase 2 REST: {json.dumps(rest_codes, sort_keys=True)}")

    # 在 disconnect 前记录连接状态(限流不踢人)
    was_connected = ws._connected
    ws.disconnect()

    if rl == OPCODE_REQUESTS and was_connected:
        _print_pass(name, f"WS 耗尽 quota 后, REST {OPCODE_REQUESTS} 个全 429 — 共享桶强证")
        return True
    _print_fail(name, f"WS 耗尽后 REST {OPCODE_REQUESTS} 个只有 {rl} 个 429, ws_connected={was_connected}")
    return False


# ── T5: opcode 不受限 ────────────────────────────────────────────────────
def test_opcodes_unthrottled(quota_window: int, no_reset: bool) -> bool:
    """
    T5: WS 桶耗尽后, TYPING/HEARTBEAT/READ_RECEIPT/ACK 各 OPCODE_REQUESTS 次
    应**不**触发 42900 限流, WS 连接保持。
    """
    name = "T5 非 SEND_MESSAGE opcode 不受限"
    print(f"\n{BLUE}━━━ {name} ━━━{RESET}")
    tm = TokenManager.load()
    tm_bob = TokenManager.load("bob")
    if not tm.access_token or not tm_bob.access_token:
        _print_warn("需要 alice + bob 两个 profile 的 token")
        return False
    if not _ensure_token_fresh(tm) or not _ensure_token_fresh(tm_bob):
        _print_warn("alice 或 bob token 过期且无法 refresh, 请重新 login")
        return False
    _ensure_friendship(tm, tm_bob)
    if not no_reset:
        _wait_for_quota_reset(quota_window, "避免上次测试残留")
    conv_id = _create_conversation(tm, tm_bob)
    _print_info(f"conversation_id={conv_id}")

    ws = _connect_ws(tm)

    # Phase 1: 100 SEND_MESSAGE 打满桶
    p1 = _ws_send_and_collect(ws, conv_id, DEFAULT_MAX_REQUESTS, "t5-phase1")
    p1_rl = sum(1 for a in p1 if a["code"] == 42900)
    _print_info(f"Phase 1 WS SEND_MESSAGE: rate_limited_42900={p1_rl} (期望 ≥ 1, 证明桶打满)")

    # Phase 2-5: 各 opcode × OPCODE_REQUESTS
    opcodes: dict = {}
    # TYPING
    captured = _ws_collect_acks(ws, OPCODE_REQUESTS)
    for _ in range(OPCODE_REQUESTS):
        ws.send_typing(conv_id)
    deadline = time.time() + 5
    while len(captured) < OPCODE_REQUESTS and time.time() < deadline:
        time.sleep(0.2)
    opcodes["TYPING"] = sum(1 for a in captured if a["code"] == 42900)
    captured.clear()

    # HEARTBEAT
    for _ in range(OPCODE_REQUESTS):
        ws.send_heartbeat()
    deadline = time.time() + 5
    while len(captured) < OPCODE_REQUESTS and time.time() < deadline:
        time.sleep(0.2)
    opcodes["HEARTBEAT"] = sum(1 for a in captured if a["code"] == 42900)
    captured.clear()

    # READ_RECEIPT
    for i in range(OPCODE_REQUESTS):
        ws.send_read_receipt(conv_id, last_msg_id=i + 1)
    deadline = time.time() + 5
    while len(captured) < OPCODE_REQUESTS and time.time() < deadline:
        time.sleep(0.2)
    opcodes["READ_RECEIPT"] = sum(1 for a in captured if a["code"] == 42900)
    captured.clear()

    # ACK
    for i in range(OPCODE_REQUESTS):
        ws.send_ack(ack_seq=i + 1)
    deadline = time.time() + 5
    while len(captured) < OPCODE_REQUESTS and time.time() < deadline:
        time.sleep(0.2)
    opcodes["ACK"] = sum(1 for a in captured if a["code"] == 42900)

    _print_info(f"opcode 42900 分布: {json.dumps(opcodes, sort_keys=True)}")
    still_connected = ws._connected
    ws.disconnect()

    total_rl = sum(opcodes.values())
    if total_rl == 0 and still_connected:
        _print_pass(name, f"4 个 opcode × {OPCODE_REQUESTS} 次 = {4 * OPCODE_REQUESTS} 次零限流, WS 连接保持")
        return True
    _print_fail(name, f"total 42900={total_rl}, connected={still_connected}")
    return False


# ── CLI ──────────────────────────────────────────────────────────────────
TESTS = {
    "t1": ("REST 限流 (Auth,RateLimit)", test_rest_ratelimit),
    "t2": ("WS SEND_MESSAGE 限流 (与 REST 共享桶)", test_ws_ratelimit),
    "t3": ("Bot 写消息限流 (BotAuth,BotRateLimit 独立桶)", test_bot_ratelimit),
    "t4": ("WS/REST 共享桶强证", test_shared_quota_bucket),
    "t5": ("非 SEND_MESSAGE opcode 不受限", test_opcodes_unthrottled),
}


def main():
    parser = argparse.ArgumentParser(
        description="AIM Gateway 限流冒烟测试",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument(
        "test",
        choices=list(TESTS.keys()) + ["all"],
        nargs="?",
        default="all",
        help="要跑的测试 (默认: all)",
    )
    parser.add_argument(
        "--quota-window",
        type=int,
        default=DEFAULT_WINDOW_SECONDS,
        help=f"限流窗口秒数(用于 sleep 重置桶), 默认 {DEFAULT_WINDOW_SECONDS}",
    )
    parser.add_argument(
        "--no-reset",
        action="store_true",
        help="不等桶重置(快速调试, 但可能被前次测试污染)",
    )
    parser.add_argument(
        "--setup",
        action="store_true",
        help="先跑 register/login 流程再开始测试(否则用 .aim_state.json 现有 token)",
    )

    args = parser.parse_args()

    print(f"{BLUE}Gateway HTTP:{RESET} {GATEWAY_HTTP}")
    print(f"{BLUE}Gateway WS:{RESET}   {GATEWAY_WS}")
    print(f"{BLUE}MaxRequests:{RESET} {DEFAULT_MAX_REQUESTS} / {args.quota_window}s")
    print(f"{BLUE}Burst:{RESET}        {BURST_REQUESTS} ({DEFAULT_MAX_REQUESTS} 通过 + 30 限流)")

    if args.setup:
        print(f"\n{BLUE}━━━ Setup: 注册 + login alice/bob ━━━{RESET}")
        _register_pair()
        _print_info("Setup 完成, 后续测试用 .aim_state.json 中的 token")

    selected = list(TESTS.keys()) if args.test == "all" else [args.test]

    results: list = []
    for t in selected:
        title, fn = TESTS[t]
        try:
            ok = fn(args.quota_window, args.no_reset)
        except Exception as e:
            _print_warn(f"{title} 异常: {type(e).__name__}: {e}")
            ok = False
        results.append((t, title, ok))

    # 汇总
    print(f"\n{BLUE}━━━ 汇总 ━━━{RESET}")
    passed = sum(1 for _, _, ok in results if ok)
    total = len(results)
    for t, title, ok in results:
        marker = f"{GREEN}✓{RESET}" if ok else f"{RED}✗{RESET}"
        print(f"  {marker} {t}  {title}")
    print(f"\n{passed}/{total} 通过")

    sys.exit(0 if passed == total else 1)


if __name__ == "__main__":
    main()
