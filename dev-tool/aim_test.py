#!/usr/bin/env python3
"""
AIM Development Tool — REST + WebSocket Test Suite
===================================================
Covers all gateway REST endpoints and WebSocket protocol frames.
Auto-refreshes tokens. Protobuf frames are encoded/decoded transparently.
Supports multiple user profiles for simultaneous multi-user testing.

Usage:
  python aim_test.py register --email a@t.com --password 12345678
  python aim_test.py login --email a@t.com --password 12345678
  python aim_test.py login --email b@t.com --password 12345678 --profile bob
  python aim_test.py search --name alice
  python aim_test.py friend-add --id 2
  python aim_test.py friend-accept --id 1
  python aim_test.py friend-reject --id 1
  python aim_test.py friend-list
  python aim_test.py friend-applications
  python aim_test.py conversation-list
  python aim_test.py conversation-create --member-id 2
  python aim_test.py history --conversation-id 1
  python aim_test.py ws-connect
  python aim_test.py ws-connect --profile bob
  python aim_test.py ws-send --conversation-id 1 --content "hello"
  python aim_test.py ws-send --conversation-id 1 --content "hi" --profile bob
  python aim_test.py interactive
  python aim_test.py run-all
"""

import sys
import os
import json
import time
import uuid
import struct
import signal
import argparse
import threading
from dataclasses import dataclass, field
from typing import Optional, Callable
from datetime import datetime

# Add dev-tool to path so we can import compiled protos
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import requests
import websocket
from google.protobuf import json_format

# Import compiled proto modules
import ws_pb2
import gateway_pb2

# ── Config ────────────────────────────────────────────────────────────────────

# Use 127.0.0.1 instead of localhost to avoid IPv6 resolution delay on Windows.
# localhost resolves to ::1 first (gateway only binds 0.0.0.0), causing
# multi-second fallback latency on every REST request and WS connection timeout.
GATEWAY_HTTP = os.environ.get("AIM_GATEWAY_HTTP", "http://127.0.0.1:8888")
GATEWAY_WS = os.environ.get("AIM_GATEWAY_WS", "ws://127.0.0.1:8888/ws")

# VERBOSE 控制 WSClient 默认的逓帧调试打印（连接/帧发送/帧接收/错误）。
# benchmark.py 在 --quiet 模式下将其设为 False。
# aim_test.py CLI 仅供交互调试，默认保持 True。
VERBOSE = True


def set_verbose(v: bool):
    """Enable/disable WSClient debug prints process-wide."""
    global VERBOSE
    VERBOSE = bool(v)
def _state_file(profile: str = "") -> str:
    """Return per-profile state file path.
    Empty profile → .aim_state.json (backward compat).
    Non-empty profile → .aim_state_{profile}.json.
    """
    base = os.path.dirname(__file__)
    if profile:
        return os.path.join(base, f".aim_state_{profile}.json")
    return os.path.join(base, ".aim_state.json")


def _all_profiles() -> list[str]:
    """Scan for existing profile state files and return profile names."""
    base = os.path.dirname(__file__)
    profiles: list[str] = []
    # Always include "default" if the base state file exists
    if os.path.exists(_state_file("")):
        profiles.append("default")
    for name in os.listdir(base):
        if name.startswith(".aim_state_") and name.endswith(".json"):
            # .aim_state_alice.json → alice
            profile = name[len(".aim_state_"):-len(".json")]
            if profile not in profiles:
                profiles.append(profile)
    return sorted(profiles, key=lambda p: (p != "default", p))

# ── Frame Types (mirrors ws.proto) ─────────────────────────────────────────────

FRAME_TYPES = {
    "SEND_MESSAGE": ws_pb2.FRAME_TYPE_SEND_MESSAGE,
    "HEARTBEAT": ws_pb2.FRAME_TYPE_HEARTBEAT,
    "TYPING": ws_pb2.FRAME_TYPE_TYPING,
    "READ_RECEIPT": ws_pb2.FRAME_TYPE_READ_RECEIPT,
    "ACK": ws_pb2.FRAME_TYPE_ACK,
    "PUSH_MESSAGE": ws_pb2.FRAME_TYPE_PUSH_MESSAGE,
    "PUSH_PRESENCE": ws_pb2.FRAME_TYPE_PUSH_PRESENCE,
    "PUSH_NOTIFICATION": ws_pb2.FRAME_TYPE_PUSH_NOTIFICATION,
    "PUSH_TYPING": ws_pb2.FRAME_TYPE_PUSH_TYPING,
    "RECONNECT": ws_pb2.FRAME_TYPE_RECONNECT,
    "SERVER_ACK": ws_pb2.FRAME_TYPE_SERVER_ACK,
    "TOKEN_EXPIRED": ws_pb2.FRAME_TYPE_TOKEN_EXPIRED,
    "PUSH_FRIEND_APPLICATION": ws_pb2.FRAME_TYPE_PUSH_FRIEND_APPLICATION,
}

FRAME_TYPE_NAMES = {v: k for k, v in FRAME_TYPES.items()}


# ── Token Manager ──────────────────────────────────────────────────────────────

@dataclass
class TokenManager:
    """Manages auth tokens with auto-refresh support."""
    access_token: Optional[str] = None
    refresh_token: Optional[str] = None
    expires_at: int = 0
    user_id: int = 0
    device_id: str = ""
    profile: str = ""

    @classmethod
    def load(cls, profile: str = "") -> "TokenManager":
        path = _state_file(profile)
        if os.path.exists(path):
            with open(path) as f:
                data = json.load(f)
                tm = cls(**data)
                tm.profile = profile
                return tm
        return cls(profile=profile)

    def save(self):
        path = _state_file(self.profile)
        with open(path, "w") as f:
            json.dump({
                "access_token": self.access_token,
                "refresh_token": self.refresh_token,
                "expires_at": self.expires_at,
                "user_id": self.user_id,
                "device_id": self.device_id,
            }, f, indent=2)

    def clear(self):
        path = _state_file(self.profile)
        self.access_token = None
        self.refresh_token = None
        self.expires_at = 0
        self.user_id = 0
        if os.path.exists(path):
            os.remove(path)

    def is_expired(self) -> bool:
        if not self.expires_at or not self.access_token:
            return True
        return int(time.time()) >= self.expires_at

    def auth_header(self) -> dict:
        return {"Authorization": f"Bearer {self.access_token}"} if self.access_token else {}


# ── REST Client ────────────────────────────────────────────────────────────────

class RESTClient:
    """Covers all AIM gateway REST endpoints."""

    def __init__(self, base_url: str = None, token: TokenManager = None, profile: str = ""):
        self.base_url = (base_url or GATEWAY_HTTP).rstrip("/")
        self.token = token or TokenManager.load(profile)
        self.profile = self.token.profile
        self.session = requests.Session()
        self.session.headers["Content-Type"] = "application/json"

    def _path(self, path: str) -> str:
        return f"{self.base_url}{path}"

    def _get(self, path: str, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.get(self._path(path), headers=h, **kwargs)
        data = r.json()
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})

    def _post(self, path: str, body=None, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.post(self._path(path), json=body, headers=h, **kwargs)
        data = r.json()
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})

    # ── Auth ──

    def register(self, email: str, password: str, username: str = "", avatar: str = "") -> dict:
        body = {
            "email": email,
            "password": password,
            "device_id": self.token.device_id or str(uuid.uuid4()),
        }
        if username:
            body["username"] = username
        if avatar:
            body["avatar"] = avatar
        return self._post("/api/auth/register", body)

    def login(self, email: str, password: str) -> dict:
        body = {
            "email": email,
            "password": password,
            "device_id": self.token.device_id or str(uuid.uuid4()),
        }
        resp = self._post("/api/auth/login", body)
        self.token.access_token = resp["access_token"]
        self.token.refresh_token = resp["refresh_token"]
        self.token.expires_at = resp["expires_at"]
        self.token.user_id = resp["user_id"]
        self.token.device_id = body["device_id"]
        self.token.save()
        return resp

    def refresh(self) -> dict:
        resp = self._post("/api/auth/refresh", {"refresh_token": self.token.refresh_token})
        self.token.access_token = resp["access_token"]
        self.token.refresh_token = resp["refresh_token"]
        self.token.expires_at = resp["expires_at"]
        self.token.save()
        return resp

    def logout(self) -> dict:
        resp = self._post("/api/auth/logout")
        self.token.clear()
        return resp

    # ── Users ──

    def search_users(self, name: str) -> list:
        return self._get(f"/api/users/by-name/{name}")["users"]

    def get_user(self, user_id: int) -> dict:
        return self._get(f"/api/users/by-id/{user_id}")["user"]

    def add_friend(self, friend_id: int) -> dict:
        return self._post(f"/api/users/friends/{friend_id}")["friendship"]

    # ── Friends ──

    def list_friend_applications(self) -> list:
        return self._get("/api/friends/applications")["applications"]

    def list_friends(self) -> list:
        return self._get("/api/friends/me")["friends"]

    def accept_friend(self, friend_id: int) -> dict:
        """NEW: accept pending friend request."""
        return self._post(f"/api/friends/accept/{friend_id}")["friendship"]

    def reject_friend(self, friend_id: int) -> dict:
        """NEW: reject pending friend request."""
        return self._post(f"/api/friends/reject/{friend_id}")["friendship"]

    # ── Conversations ──

    def create_conversation(self, member_ids: list) -> dict:
        body = {
            "conversation_type": "direct" if len(member_ids) == 1 else "group",
            "member_ids": member_ids,
        }
        return self._post("/api/conversations", body)

    def get_history(self, conversation_id: int, cursor_created_at: int = 0,
                    cursor_id: int = 0, limit: int = 50) -> dict:
        params = []
        if cursor_created_at:
            params.append(f"cursor_created_at={cursor_created_at}")
        if cursor_id:
            params.append(f"cursor_id={cursor_id}")
        if limit:
            params.append(f"limit={limit}")
        query = "?" + "&".join(params) if params else ""
        return self._get(f"/api/conversations/history/{conversation_id}{query}")

    def list_conversations(self) -> list:
        return self._get("/api/conversations")["conversations"]


# ── WebSocket Client ────────────────────────────────────────────────────────────

class WSClient:
    """WebSocket client with automatic protobuf frame encoding/decoding.

    Features:
    - Auto heartbeat: periodically sends heartbeat frames to keep the connection alive.
    - Auto reconnect: on unexpected close, retries connection with exponential backoff.
    """

    # 心跳间隔（秒），0 表示禁用自动心跳
    HEARTBEAT_INTERVAL = 30.0
    # 自动重连最大次数，0 表示禁用自动重连
    RECONNECT_MAX_RETRIES = 3
    # 重连初始退避（秒）
    RECONNECT_BACKOFF_BASE = 1.0

    def __init__(self, url: str = None, token: TokenManager = None,
                 heartbeat_interval: float = None,
                 reconnect_max_retries: int = None):
        self.url = url or GATEWAY_WS
        self.token = token or TokenManager.load()
        self.ws: Optional[websocket.WebSocketApp] = None
        self.seq = 0
        self.on_frame: Optional[Callable] = None
        self._connected = False
        self._error: Optional[str] = None
        self._heartbeat_interval = heartbeat_interval if heartbeat_interval is not None else self.HEARTBEAT_INTERVAL
        self._reconnect_max = reconnect_max_retries if reconnect_max_retries is not None else self.RECONNECT_MAX_RETRIES
        self._heartbeat_timer: Optional[threading.Timer] = None
        self._intentional_close = False
        self._reconnect_count = 0

    def _next_seq(self) -> int:
        self.seq += 1
        return self.seq

    def connect(self):
        # The gateway expects a GET /ws upgrade request with Authorization header.
        # websocket-client sends HTTP upgrade (GET + Upgrade headers) natively.
        self._intentional_close = False
        headers = {}
        if self.token.access_token:
            headers["Authorization"] = f"Bearer {self.token.access_token}"
        self._error = None
        self.ws = websocket.WebSocketApp(
            self.url,
            header=headers,
            on_open=self._on_open,
            on_message=self._on_message,
            on_error=self._on_error,
            on_close=self._on_close,
        )
        # Run in background thread
        connect_timeout = 15.0
        t = threading.Thread(target=self.ws.run_forever, daemon=True)
        t.start()
        # Wait for connection with timeout
        deadline = time.time() + connect_timeout
        while not self._connected and time.time() < deadline:
            if self._error:
                if VERBOSE:
                    print(f"  ✗ WS connection failed: {self._error}")
                self.disconnect()
                return
            if not t.is_alive():
                if VERBOSE:
                    print(f"  ✗ WS thread died unexpectedly")
                self.disconnect()
                return
            time.sleep(0.1)
        if not self._connected:
            if VERBOSE:
                if self._error:
                    print(f"  ✗ WS connection failed: {self._error}")
                else:
                    print(f"  ✗ WS connection timed out after {int(connect_timeout)}s")
            self.disconnect()
        else:
            # 连接成功，启动心跳
            self._reconnect_count = 0
            self._start_heartbeat()

    def reconnect(self, max_retries: int = None) -> bool:
        """Attempt to reconnect with exponential backoff.

        Returns True if reconnection succeeds, False otherwise.
        """
        retries = max_retries if max_retries is not None else self._reconnect_max
        for attempt in range(retries):
            backoff = self.RECONNECT_BACKOFF_BASE * (2 ** attempt)
            if VERBOSE:
                print(f"  ↻ WS reconnect attempt {attempt + 1}/{retries} in {backoff:.1f}s...")
            time.sleep(backoff)
            self.connect()
            if self._connected:
                if VERBOSE:
                    print(f"  ✓ WS reconnected on attempt {attempt + 1}")
                return True
        if VERBOSE:
            print(f"  ✗ WS reconnect failed after {retries} attempts")
        return False

    def _start_heartbeat(self):
        """Start periodic heartbeat."""
        self._stop_heartbeat()
        if self._heartbeat_interval <= 0:
            return
        def _heartbeat_loop():
            if self._connected and not self._intentional_close:
                try:
                    self.send_heartbeat()
                except Exception:
                    pass  # 连接已断开，心跳失败忽略
            if self._connected and not self._intentional_close:
                self._heartbeat_timer = threading.Timer(self._heartbeat_interval, _heartbeat_loop)
                self._heartbeat_timer.daemon = True
                self._heartbeat_timer.start()
        self._heartbeat_timer = threading.Timer(self._heartbeat_interval, _heartbeat_loop)
        self._heartbeat_timer.daemon = True
        self._heartbeat_timer.start()

    def _stop_heartbeat(self):
        """Stop periodic heartbeat."""
        if self._heartbeat_timer is not None:
            self._heartbeat_timer.cancel()
            self._heartbeat_timer = None

    def disconnect(self):
        self._intentional_close = True
        self._stop_heartbeat()
        if self.ws:
            self.ws.close()

    def is_connected(self) -> bool:
        return self._connected

    def _on_open(self, ws):
        self._connected = True
        if VERBOSE:
            print(f"  ✓ WS connected to {self.url}")

    def _on_message(self, ws, data: bytes):
        try:
            frame = ws_pb2.WsFrame()
            frame.ParseFromString(data)
            ftype = FRAME_TYPE_NAMES.get(frame.type, f"UNKNOWN({frame.type})")
            payload = self._decode_payload(frame)
            if VERBOSE:
                print(f"\n  ← [{ftype}] seq={frame.seq}")
                if payload:
                    print(f"    {json_format.MessageToDict(payload, preserving_proto_field_name=True)}")
            if self.on_frame:
                self.on_frame(frame, payload)
        except Exception as e:
            if VERBOSE:
                print(f"  ✗ Decode error: {e}")

    def _on_error(self, ws, error):
        self._error = str(error)
        if VERBOSE:
            print(f"  ✗ WS error: {error}")

    def _on_close(self, ws, status, msg):
        self._connected = False
        self._stop_heartbeat()
        if VERBOSE:
            print(f"  ✓ WS closed (status={status})")
        # 非主动关闭时尝试自动重连
        if not self._intentional_close and self._reconnect_max > 0:
            self._reconnect_count += 1
            if self._reconnect_count <= self._reconnect_max:
                def _do_reconnect():
                    if self.reconnect(max_retries=1):
                        self._reconnect_count = 0  # 重连成功，重置计数
                t = threading.Thread(target=_do_reconnect, daemon=True)
                t.start()

    def _decode_payload(self, frame: ws_pb2.WsFrame):
        """Decode payload based on frame type."""
        mapping = {
            ws_pb2.FRAME_TYPE_SEND_MESSAGE: ws_pb2.SendMessagePayload,
            ws_pb2.FRAME_TYPE_HEARTBEAT: ws_pb2.HeartbeatPayload,
            ws_pb2.FRAME_TYPE_TYPING: ws_pb2.TypingPayload,
            ws_pb2.FRAME_TYPE_READ_RECEIPT: ws_pb2.ReadReceiptPayload,
            ws_pb2.FRAME_TYPE_ACK: ws_pb2.ClientAckPayload,
            ws_pb2.FRAME_TYPE_PUSH_MESSAGE: ws_pb2.PushMessagePayload,
            ws_pb2.FRAME_TYPE_PUSH_PRESENCE: ws_pb2.PushPresencePayload,
            ws_pb2.FRAME_TYPE_PUSH_NOTIFICATION: ws_pb2.PushNotificationPayload,
            ws_pb2.FRAME_TYPE_PUSH_TYPING: ws_pb2.PushTypingPayload,
            ws_pb2.FRAME_TYPE_RECONNECT: ws_pb2.ReconnectPayload,
            ws_pb2.FRAME_TYPE_SERVER_ACK: ws_pb2.ServerAckPayload,
            ws_pb2.FRAME_TYPE_TOKEN_EXPIRED: ws_pb2.TokenExpiredPayload,
            ws_pb2.FRAME_TYPE_PUSH_FRIEND_APPLICATION: ws_pb2.PushFriendApplicationPayload,
        }
        cls = mapping.get(frame.type)
        if cls is None or not frame.payload:
            return None
        msg = cls()
        msg.ParseFromString(frame.payload)
        return msg

    def _send_frame(self, frame_type: int, payload_msg=None):
        if not self._connected:
            raise RuntimeError("Not connected. Run ws-connect first.")
        frame = ws_pb2.WsFrame()
        frame.type = frame_type
        frame.seq = self._next_seq()
        frame.timestamp = int(time.time() * 1000)
        if payload_msg:
            frame.payload = payload_msg.SerializeToString()
        data = frame.SerializeToString()
        self.ws.send(data, opcode=websocket.ABNF.OPCODE_BINARY)
        if VERBOSE:
            ftype = FRAME_TYPE_NAMES.get(frame_type, f"UNKNOWN({frame_type})")
            payload_info = ""
            if payload_msg:
                d = json_format.MessageToDict(payload_msg, preserving_proto_field_name=True)
                payload_info = f" {d}"
            print(f"  → [{ftype}] seq={frame.seq}{payload_info}")

    # ── Convenience methods ──

    def ensure_connected(self, max_retries: int = 1) -> bool:
        """Check connection and attempt reconnect if disconnected.

        Returns True if connected (either already or after reconnect), False otherwise.
        """
        if self._connected:
            return True
        if self._intentional_close:
            return False
        return self.reconnect(max_retries=max_retries)

    def send_message(self, conversation_id: int, content: str, message_type: str = "text") -> str:
        """Send a chat message and return the client_msg_id for correlation."""
        payload = ws_pb2.SendMessagePayload()
        payload.conversation_id = conversation_id
        payload.message_type = message_type
        payload.content = content
        client_msg_id = str(uuid.uuid4())
        payload.client_msg_id = client_msg_id
        self._send_frame(ws_pb2.FRAME_TYPE_SEND_MESSAGE, payload)
        return client_msg_id

    def send_heartbeat(self):
        payload = ws_pb2.HeartbeatPayload()
        self._send_frame(ws_pb2.FRAME_TYPE_HEARTBEAT, payload)

    def send_typing(self, conversation_id: int):
        payload = ws_pb2.TypingPayload()
        payload.conversation_id = conversation_id
        self._send_frame(ws_pb2.FRAME_TYPE_TYPING, payload)

    def send_read_receipt(self, conversation_id: int, last_msg_id: int):
        payload = ws_pb2.ReadReceiptPayload()
        payload.conversation_id = conversation_id
        payload.last_msg_id = last_msg_id
        self._send_frame(ws_pb2.FRAME_TYPE_READ_RECEIPT, payload)


# ── Error ──────────────────────────────────────────────────────────────────────

class APIError(Exception):
    def __init__(self, code: int, message: str):
        self.code = code
        self.message = message
        super().__init__(f"[{code}] {message}")


# ── CLI ────────────────────────────────────────────────────────────────────────

def print_json(data):
    print(json.dumps(data, indent=2, ensure_ascii=False))


def cmd_register(args):
    client = RESTClient()
    try:
        resp = client.register(args.email, args.password, args.username or "", args.avatar or "")
        print("✓ Registered successfully")
        print_json(resp)
    except APIError as e:
        print(f"✗ Register failed: {e}")


def cmd_login(args):
    profile = getattr(args, "profile", "") or ""
    client = RESTClient(profile=profile)
    try:
        resp = client.login(args.email, args.password)
        print(f"✓ Logged in as user #{resp['user_id']} (profile={'default' if not profile else profile})")
        print_json(resp)
    except APIError as e:
        print(f"✗ Login failed: {e}")


def cmd_refresh(args):
    profile = getattr(args, "profile", "") or ""
    client = RESTClient(profile=profile)
    try:
        resp = client.refresh()
        print("✓ Token refreshed")
        print_json(resp)
    except APIError as e:
        print(f"✗ Refresh failed: {e}")


def cmd_logout(args):
    profile = getattr(args, "profile", "") or ""
    client = RESTClient(profile=profile)
    try:
        resp = client.logout()
        print("✓ Logged out")
    except APIError as e:
        print(f"✗ Logout failed: {e}")


def cmd_search(args):
    client = RESTClient()
    try:
        users = client.search_users(args.name)
        print(f"✓ Found {len(users)} user(s):")
        print_json(users)
    except APIError as e:
        print(f"✗ Search failed: {e}")


def cmd_get_user(args):
    client = RESTClient()
    try:
        user = client.get_user(args.id)
        print_json(user)
    except APIError as e:
        print(f"✗ Get user failed: {e}")


def cmd_add_friend(args):
    client = RESTClient()
    try:
        friendship = client.add_friend(args.id)
        print(f"✓ Friend request sent (status={friendship['status']})")
        print_json(friendship)
    except APIError as e:
        print(f"✗ Add friend failed: {e}")


def cmd_friend_applications(args):
    client = RESTClient()
    try:
        apps = client.list_friend_applications()
        print(f"✓ {len(apps)} pending application(s):")
        print_json(apps)
    except APIError as e:
        print(f"✗ List applications failed: {e}")


def cmd_accept_friend(args):
    client = RESTClient()
    try:
        friendship = client.accept_friend(args.id)
        print(f"✓ Friend request accepted (status={friendship['status']})")
        print_json(friendship)
    except APIError as e:
        print(f"✗ Accept friend failed: {e}")


def cmd_reject_friend(args):
    client = RESTClient()
    try:
        friendship = client.reject_friend(args.id)
        print(f"✓ Friend request rejected (status={friendship['status']})")
        print_json(friendship)
    except APIError as e:
        print(f"✗ Reject friend failed: {e}")


def cmd_friend_list(args):
    client = RESTClient()
    try:
        friends = client.list_friends()
        print(f"✓ {len(friends)} friend(s):")
        print_json(friends)
    except APIError as e:
        print(f"✗ List friends failed: {e}")


def cmd_create_conversation(args):
    client = RESTClient()
    try:
        member_ids = [int(m.strip()) for m in args.member_ids.split(",")] if args.member_ids else [args.member_id] if args.member_id else []
        if not member_ids:
            print("✗ At least one --member-id or --member-ids is required")
            return
        conv = client.create_conversation(member_ids)
        print(f"✓ Conversation #{conv['conversation_id']} created ({conv['conversation_type']})")
        if len(member_ids) > 1:
            print(f"  Members: {member_ids}")
        print_json(conv)
    except APIError as e:
        print(f"✗ Create conversation failed: {e}")


def cmd_list_conversations(args):
    client = RESTClient()
    try:
        convs = client.list_conversations()
        print(f"✓ {len(convs)} conversation(s):")
        print_json(convs)
    except APIError as e:
        print(f"✗ List conversations failed: {e}")


def cmd_history(args):
    client = RESTClient()
    try:
        resp = client.get_history(args.conversation_id, args.cursor_created_at or 0,
                                  args.cursor_id or 0, args.limit or 50)
        msgs = resp.get("messages", [])
        print(f"✓ {len(msgs)} message(s):")
        print_json(msgs)
        if resp.get("has_more"):
            print(f"  More messages available (next cursor: {resp.get('next_cursor_id')})")
    except APIError as e:
        print(f"✗ History failed: {e}")


# ── WebSocket commands ──

_ws_clients: dict[str, WSClient] = {}


def _get_ws_client(profile: str = "") -> Optional[WSClient]:
    """Get WS client for a profile. Empty string = default profile key."""
    key = profile if profile else "default"
    return _ws_clients.get(key)


def cmd_ws_connect(args):
    profile = getattr(args, "profile", "") or ""
    key = profile if profile else "default"
    token = TokenManager.load(profile)
    if not token.access_token:
        print("✗ Not logged in. Run 'login' first.")
        return
    if token.is_expired():
        print("✗ Token expired. Run 'refresh' first.")
        return
    # Disconnect existing client for this profile if any
    existing = _ws_clients.get(key)
    if existing:
        existing.disconnect()
    ws = WSClient(token=token)
    ws.connect()
    _ws_clients[key] = ws


def cmd_ws_send(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        profile_hint = f" (profile={profile})" if profile else ""
        print(f"✗ Not connected{profile_hint}. Run 'ws-connect' first.")
        return
    try:
        ws.send_message(args.conversation_id, args.content, args.message_type or "text")
    except Exception as e:
        print(f"✗ Send failed: {e}")


def cmd_ws_heartbeat(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    ws.send_heartbeat()


def cmd_ws_typing(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    ws.send_typing(args.conversation_id)


def _auth_status_line(token: TokenManager) -> str:
    """Build a compact status line showing current auth state."""
    if not token.access_token:
        return "auth: ✗ not logged in"
    expired = token.is_expired()
    state = "EXPIRED" if expired else "valid"
    marker = "✗" if expired else "✓"
    return f"auth: {marker} user=#{token.user_id} token={state}"


def _ws_status_line(profile: str = "") -> str:
    """Build a compact WebSocket connection status line for a profile."""
    ws = _get_ws_client(profile)
    if ws and ws.is_connected():
        return "ws:   ✓ connected"
    return "ws:   ✗ disconnected"


def _all_profiles_status() -> list[str]:
    """Build status lines for all profiles."""
    lines = []
    for p in _all_profiles():
        token = TokenManager.load("" if p == "default" else p)
        auth = _auth_status_line(token)
        ws = _ws_status_line("" if p == "default" else p)
        label = p
        lines.append(f"[{label}] {auth}  |  {ws}")
    return lines


def _print_help():
    """Print categorized command reference."""
    print("""
┌─ Auth ───────────────────────────────────────────┐
│  register <email> <password> [username]           │
│  login <email> <password> [--profile NAME]        │
│  refresh | logout                                 │
├─ Users & Friends ────────────────────────────────┤
│  search <name>          user <id>                 │
│  friend-add <id>        friend-apps               │
│  friend-accept <id>     friend-reject <id>        │
│  friend-list                                       │
├─ Conversations ──────────────────────────────────┤
│  conv-list                                         │
│  conv-create <member_id>  (or comma-sep for group) │
│  history <conversation_id> [limit]                │
├─ WebSocket ───────────────────────────────────────┤
│  ws-connect [--profile NAME]                      │
│  ws-send <conv_id> <text> [--profile NAME]        │
│  ws-heartbeat [--profile NAME]                    │
│  ws-typing <id> [--profile NAME]                  │
│  ws-recv (wait for incoming frames)               │
├─ Profiles ────────────────────────────────────────┤
│  switch <profile>   change active profile          │
│  status             show all profiles              │
├─ Meta ────────────────────────────────────────────┤
│  help | quit | exit | status                      │
└───────────────────────────────────────────────────┘""")


def cmd_interactive(args):
    """Interactive mode for exploring the API."""
    _active_profile: str = "default"  # current active profile

    def _profile_key() -> str:
        """Return the state-file profile string: "" for default, profile name otherwise."""
        return "" if _active_profile == "default" else _active_profile

    def _reload_client():
        nonlocal client, token
        token = TokenManager.load(_profile_key())
        client = RESTClient(token=token)

    token = TokenManager.load(_profile_key())
    client = RESTClient(token=token)

    width = 52
    gw_label = f"Gateway HTTP: {GATEWAY_HTTP}"
    ws_label = f"Gateway WS:   {GATEWAY_WS}"
    top = "┌" + "─" * (width - 2) + "┐"
    mid = "├" + "─" * (width - 2) + "┤"
    bot = "└" + "─" * (width - 2) + "┘"

    def _banner():
        lines = []
        for stat_line in _all_profiles_status():
            lines.append(f"│ {stat_line:<{width - 4}} │")
        profiles_block = "\n".join(lines) if lines else f"│ {'(no profiles)':<{width - 4}} │"
        print(f"""
{top}
│ {"AIM Dev Tool — Interactive Mode":^{width - 4}} │
{mid}
│ {gw_label:<{width - 4}} │
│ {ws_label:<{width - 4}} │
{mid}
{profiles_block}
{bot}
Active profile: {_active_profile}
Type 'help' for commands, 'quit' to exit.
""")

    _banner()

    while True:
        try:
            # Build dynamic prompt with compact status
            auth_hint = f"#{client.token.user_id}" if client.token.access_token and not client.token.is_expired() else "?"
            ws = _get_ws_client(_profile_key())
            ws_hint = "⚡" if (ws and ws.is_connected()) else "·"
            profile_hint = _active_profile
            line = input(f"aim [{profile_hint}] [{auth_hint}] {ws_hint}> ").strip()

            if not line:
                continue
            parts = line.split()
            cmd = parts[0].lower()

            if cmd == "quit" or cmd == "exit":
                break
            elif cmd == "help":
                _print_help()
            elif cmd == "status":
                _banner()
            elif cmd == "switch" and len(parts) >= 2:
                new_profile = parts[1]
                _active_profile = new_profile
                _reload_client()
                print(f"✓ Switched to profile '{_active_profile}'")
                _banner()
            elif cmd == "register" and len(parts) >= 3:
                resp = client.register(parts[1], parts[2], parts[3] if len(parts) > 3 else "")
                print("✓ Registered successfully")
                print_json(resp)
            elif cmd == "login" and len(parts) >= 3:
                resp = client.login(parts[1], parts[2])
                print(f"✓ Logged in as user #{resp['user_id']} (profile={_active_profile})")
                print_json(resp)
                _banner()
            elif cmd == "refresh":
                resp = client.refresh()
                print("✓ Token refreshed")
                print_json(resp)
                _banner()
            elif cmd == "logout":
                client.logout()
                print("✓ Logged out")
                _banner()
            elif cmd == "search" and len(parts) >= 2:
                users = client.search_users(parts[1])
                print(f"✓ Found {len(users)} user(s):")
                print_json(users)
            elif cmd == "user" and len(parts) >= 2:
                user = client.get_user(int(parts[1]))
                print_json(user)
            elif cmd == "friend-add" and len(parts) >= 2:
                friendship = client.add_friend(int(parts[1]))
                print(f"✓ Friend request sent (status={friendship['status']})")
                print_json(friendship)
            elif cmd == "friend-accept" and len(parts) >= 2:
                friendship = client.accept_friend(int(parts[1]))
                print(f"✓ Friend request accepted (status={friendship['status']})")
                print_json(friendship)
            elif cmd == "friend-reject" and len(parts) >= 2:
                friendship = client.reject_friend(int(parts[1]))
                print(f"✓ Friend request rejected (status={friendship['status']})")
                print_json(friendship)
            elif cmd == "friend-apps":
                apps = client.list_friend_applications()
                print(f"✓ {len(apps)} pending application(s):")
                print_json(apps)
            elif cmd == "friend-list" or cmd == "friends":
                friends = client.list_friends()
                print(f"✓ {len(friends)} friend(s):")
                print_json(friends)
            elif cmd == "conv-list" or cmd == "list-conversations" or cmd == "conversations":
                convs = client.list_conversations()
                print(f"✓ {len(convs)} conversation(s):")
                print_json(convs)
            elif cmd == "conv-create" and len(parts) >= 2:
                member_ids = [int(m.strip()) for m in parts[1].split(",")]
                conv = client.create_conversation(member_ids)
                print(f"✓ Conversation #{conv['conversation_id']} created ({conv['conversation_type']})")
                if len(member_ids) > 1:
                    print(f"  Members: {member_ids}")
                print_json(conv)
            elif cmd == "history" and len(parts) >= 2:
                limit = int(parts[2]) if len(parts) > 2 else 50
                resp = client.get_history(int(parts[1]), limit=limit)
                msgs = resp.get("messages", [])
                print(f"✓ {len(msgs)} message(s):")
                print_json(msgs)
                if resp.get("has_more"):
                    print(f"  More messages available (next cursor: {resp.get('next_cursor_id')})")
            elif cmd == "ws-connect":
                ws_profile = _profile_key()
                existing = _ws_clients.get(_active_profile)
                if existing:
                    existing.disconnect()
                if not client.token.access_token:
                    print("✗ Not logged in. Run 'login' first.")
                    continue
                if client.token.is_expired():
                    print("✗ Token expired. Run 'refresh' first.")
                    continue
                ws_client = WSClient(token=client.token)
                ws_client.connect()
                _ws_clients[_active_profile] = ws_client
                _banner()
            elif cmd == "ws-send" and len(parts) >= 3:
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                try:
                    ws.send_message(int(parts[1]), parts[2], "text")
                except Exception as e:
                    print(f"✗ Send failed: {e}")
            elif cmd == "ws-heartbeat":
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                ws.send_heartbeat()
            elif cmd == "ws-typing" and len(parts) >= 2:
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                ws.send_typing(int(parts[1]))
            elif cmd == "ws-recv":
                print("Waiting for frames... (Ctrl+C to stop)")
                try:
                    while True:
                        time.sleep(0.1)
                except KeyboardInterrupt:
                    print()
            else:
                print(f"  Unknown command: '{cmd}'. Type 'help' for available commands.")
        except APIError as e:
            print(f"  ✗ {e}")
        except KeyboardInterrupt:
            print("\n  Use 'quit' or 'exit' to leave interactive mode.")
        except Exception as e:
            print(f"  ✗ Error: {e}")


# ── Run all tests ──────────────────────────────────────────────────────────────

def cmd_run_all(args):
    """Run a full integration test flow using two profiles for multi-user testing."""
    print("=" * 60)
    print("  AIM Full Integration Test (Multi-Profile)")
    print("=" * 60)

    test_password = "12345678"

    # 1. Register & login alice (profile=alice) and bob (profile=bob)
    print("\n── 1. Register & Login ──")
    alice_email = f"alice_{int(time.time())}@aim.dev"
    bob_email = f"bob_{int(time.time())}@aim.dev"

    client_alice = RESTClient(profile="alice")
    client_alice.register(alice_email, test_password, "Alice")
    print(f"  ✓ Registered Alice ({alice_email})")
    client_alice.login(alice_email, test_password)
    alice_id = client_alice.token.user_id
    print(f"  ✓ Alice logged in as #{alice_id}")

    client_bob = RESTClient(profile="bob")
    client_bob.register(bob_email, test_password, "Bob")
    print(f"  ✓ Registered Bob ({bob_email})")
    client_bob.login(bob_email, test_password)
    bob_id = client_bob.token.user_id
    print(f"  ✓ Bob logged in as #{bob_id}")

    # Wait for Kafka UserCreated events to be consumed and user info
    # to be created in the logic service database (~2-3s propagation delay).
    print("  ⏳ Waiting for Kafka user sync...")
    time.sleep(5)

    # 2. Search users
    print("\n── 2. Search ──")
    users = client_alice.search_users("Alice")
    print(f"  ✓ Found {len(users)} user(s)")

    # 3. Friend request (Alice → Bob)
    print("\n── 3. Friend Request ──")
    friendship = client_alice.add_friend(bob_id)
    print(f"  ✓ Alice sent friend request to Bob (status={friendship['status']})")

    # 4. Accept friend (as Bob)
    print("\n── 4. Accept Friend ──")
    apps = client_bob.list_friend_applications()
    print(f"  ✓ Bob has {len(apps)} pending application(s)")
    friendship = client_bob.accept_friend(alice_id)
    print(f"  ✓ Bob accepted (status={friendship['status']})")

    # 4.5. Check friend lists
    print("\n── 4.5. Friend Lists ──")
    alice_friends = client_alice.list_friends()
    print(f"  ✓ Alice has {len(alice_friends)} friend(s)")
    bob_friends = client_bob.list_friends()
    print(f"  ✓ Bob has {len(bob_friends)} friend(s)")

    # 5. Create conversation
    print("\n── 5. Create Conversation ──")
    conv = client_alice.create_conversation([bob_id])
    conv_id = conv["conversation_id"]
    print(f"  ✓ Conversation #{conv_id} created")

    # 6. Get history (empty)
    print("\n── 6. Get History (empty) ──")
    resp = client_alice.get_history(conv_id, limit=10)
    print(f"  ✓ {len(resp.get('messages', []))} message(s)")

    # 7. WebSocket connect both users simultaneously
    print("\n── 7. WebSocket Connect (both profiles) ──")

    bob_received = threading.Event()
    bob_ws = WSClient(token=client_bob.token)
    bob_ws.on_frame = lambda frame, payload: (
        bob_received.set() if frame.type == ws_pb2.FRAME_TYPE_PUSH_MESSAGE else None
    )
    bob_ws.connect()
    _ws_clients["bob"] = bob_ws

    alice_ws = WSClient(token=client_alice.token)
    alice_ws.connect()
    _ws_clients["alice"] = alice_ws

    # 8. Alice sends message, verify Bob receives it
    print("\n── 8. Alice sends → Bob receives ──")
    alice_ws.send_message(conv_id, "Hello from Alice!")
    # Wait up to 5 seconds for Bob to receive
    received = bob_received.wait(timeout=5.0)
    if received:
        print("  ✓ Bob received the push message from Alice!")
    else:
        print("  ⚠ Bob did not receive push within timeout (may still be delivered)")

    # 9. Check history (should have message now)
    print("\n── 9. Get History (after send) ──")
    resp = client_alice.get_history(conv_id, limit=10)
    msgs = resp.get("messages", [])
    print(f"  ✓ {len(msgs)} message(s) now")

    # 10. Refresh token (Alice)
    print("\n── 10. Refresh Token ──")
    client_alice.refresh()
    print(f"  ✓ Alice token refreshed (expires_at={client_alice.token.expires_at})")

    # 11. Disconnect WS
    print("\n── 11. Disconnect WS ──")
    alice_ws.disconnect()
    bob_ws.disconnect()
    _ws_clients.pop("alice", None)
    _ws_clients.pop("bob", None)
    print("  ✓ Both WS connections closed")

    # 12. Logout
    print("\n── 12. Logout ──")
    client_alice.logout()
    client_bob.logout()
    print("  ✓ Both users logged out")

    # Cleanup profile state files
    for profile in ["alice", "bob"]:
        path = _state_file(profile)
        if os.path.exists(path):
            os.remove(path)

    print("\n" + "=" * 60)
    print("  All tests completed!")
    print("=" * 60)


# ── Main ───────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="AIM Dev Tool — REST + WebSocket Test Suite",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python aim_test.py register --email a@t.com --password 12345678
  python aim_test.py login --email a@t.com --password 12345678
  python aim_test.py login --email b@t.com --password 12345678 --profile bob
  python aim_test.py friend-add --id 2
  python aim_test.py friend-accept --id 1
  python aim_test.py ws-connect
  python aim_test.py ws-connect --profile bob
  python aim_test.py ws-send --conversation-id 1 --content "hello"
  python aim_test.py ws-send --conversation-id 1 --content "hi" --profile bob
  python aim_test.py interactive
  python aim_test.py run-all
        """
    )
    sub = parser.add_subparsers(dest="command", help="Available commands")

    # Auth
    p = sub.add_parser("register", help="Register a new user")
    p.add_argument("--email", required=True)
    p.add_argument("--password", required=True)
    p.add_argument("--username")
    p.add_argument("--avatar")

    p = sub.add_parser("login", help="Login and save token")
    p.add_argument("--email", required=True)
    p.add_argument("--password", required=True)
    p.add_argument("--profile", default="", help="Profile name (default: empty = default profile)")

    p = sub.add_parser("refresh", help="Refresh access token")
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("logout", help="Logout and clear token")
    p.add_argument("--profile", default="", help="Profile name")

    # Users
    p = sub.add_parser("search", help="Search users by name")
    p.add_argument("--name", required=True)

    p = sub.add_parser("get-user", help="Get user by ID")
    p.add_argument("--id", type=int, required=True)

    # Friends
    p = sub.add_parser("friend-add", help="Send friend request")
    p.add_argument("--id", type=int, required=True)

    sub.add_parser("friend-applications", help="List pending friend applications")

    p = sub.add_parser("friend-accept", help="Accept pending friend request")
    p.add_argument("--id", type=int, required=True)

    p = sub.add_parser("friend-reject", help="Reject pending friend request")
    p.add_argument("--id", type=int, required=True)

    sub.add_parser("friend-list", help="List friends")

    # Conversations
    p = sub.add_parser("conv-list", help="List conversations")
    p = sub.add_parser("conv-create", help="Create conversation")
    p.add_argument("--member-id", type=int, help="Single member ID")
    p.add_argument("--member-ids", help="Comma-separated member IDs (e.g. 2,3,4)")

    p = sub.add_parser("history", help="Get conversation history")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--cursor-created-at", type=int)
    p.add_argument("--cursor-id", type=int)
    p.add_argument("--limit", type=int, default=50)

    # WebSocket
    p = sub.add_parser("ws-connect", help="Connect WebSocket")
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-send", help="Send message via WebSocket")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--content", required=True)
    p.add_argument("--message-type", default="text")
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-heartbeat", help="Send heartbeat via WebSocket")
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-typing", help="Send typing indicator")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--profile", default="", help="Profile name")

    # Meta
    sub.add_parser("interactive", help="Interactive mode")
    sub.add_parser("run-all", help="Run full integration test flow")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return

    commands = {
        "register": cmd_register,
        "login": cmd_login,
        "refresh": cmd_refresh,
        "logout": cmd_logout,
        "search": cmd_search,
        "get-user": cmd_get_user,
        "friend-add": cmd_add_friend,
        "friend-applications": cmd_friend_applications,
        "friend-accept": cmd_accept_friend,
        "friend-reject": cmd_reject_friend,
        "friend-list": cmd_friend_list,
        "conv-list": cmd_list_conversations,
        "conv-create": cmd_create_conversation,
        "history": cmd_history,
        "ws-connect": cmd_ws_connect,
        "ws-send": cmd_ws_send,
        "ws-heartbeat": cmd_ws_heartbeat,
        "ws-typing": cmd_ws_typing,
        "interactive": cmd_interactive,
        "run-all": cmd_run_all,
    }

    handler = commands.get(args.command)
    if handler:
        handler(args)


if __name__ == "__main__":
    main()
