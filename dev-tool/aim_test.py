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

try:
    from prompt_toolkit import PromptSession
    from prompt_toolkit.patch_stdout import patch_stdout
except ImportError:  # pragma: no cover - optional interactive dependency
    PromptSession = None
    patch_stdout = None

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
_PRINT_LOCK = threading.RLock()


def set_verbose(v: bool):
    """Enable/disable WSClient debug prints process-wide."""
    global VERBOSE
    VERBOSE = bool(v)


def _safe_print(*args, **kwargs):
    """Thread-safe print used by WS callbacks and CLI output."""
    with _PRINT_LOCK:
        print(*args, **kwargs)


def _ws_debug_print(*args, **kwargs):
    """Print WS debug output without corrupting interactive prompt."""
    if VERBOSE:
        _safe_print(*args, **kwargs)
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
    "PUSH_READ_RECEIPT": ws_pb2.FRAME_TYPE_PUSH_READ_RECEIPT,
}

FRAME_TYPE_NAMES = {v: k for k, v in FRAME_TYPES.items()}

REPLAYABLE_PUSH_TYPES = {
    ws_pb2.FRAME_TYPE_PUSH_MESSAGE,
    ws_pb2.FRAME_TYPE_PUSH_NOTIFICATION,
    ws_pb2.FRAME_TYPE_PUSH_FRIEND_APPLICATION,
    ws_pb2.FRAME_TYPE_PUSH_READ_RECEIPT,
}


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

    def _safe_json(self, r) -> dict:
        text = r.text.strip()
        if not text:
            return {}
        return r.json()

    def _get(self, path: str, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.get(self._path(path), headers=h, **kwargs)
        data = self._safe_json(r)
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.get(self._path(path), headers=h, **kwargs)
        data = r.json()
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})

    def _post(self, path: str, body=None, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.post(self._path(path), json=body, headers=h, **kwargs)
        data = self._safe_json(r)
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})
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

    def get_friends_presence(self) -> list:
        return self._get("/api/presence/friends").get("friends", [])

    def accept_friend(self, friend_id: int) -> dict:
        """NEW: accept pending friend request."""
        return self._post(f"/api/friends/accept/{friend_id}")["friendship"]

    def reject_friend(self, friend_id: int) -> dict:
        """NEW: reject pending friend request."""
        return self._post(f"/api/friends/reject/{friend_id}")["friendship"]

    # ── Friend Tags ──

    def list_friend_tags(self) -> list:
        return self._get("/api/friends/tags")["tags"]

    def create_friend_tag(self, name: str) -> dict:
        return self._post("/api/friends/tags", {"name": name})["tag"]

    def rename_friend_tag(self, tag_id: int, name: str) -> dict:
        return self._put(f"/api/friends/tags/{tag_id}", {"name": name})["tag"]

    def delete_friend_tag(self, tag_id: int) -> dict:
        return self._delete(f"/api/friends/tags/{tag_id}")

    def set_friend_tags(self, friend_id: int, tag_ids: list) -> dict:
        return self._put(f"/api/friends/{friend_id}/tags", {"tag_ids": tag_ids})["friendship"]

    def remove_friend_tag(self, friend_id: int, tag_id: int) -> dict:
        return self._delete(f"/api/friends/{friend_id}/tags/{tag_id}")["friendship"]

    # ── Conversations ──

    def create_conversation(self, member_ids: list, name: str = "") -> dict:
        conversation_type = "direct" if len(member_ids) == 1 else "group"
        body = {
            "conversation_type": conversation_type,
            "member_ids": member_ids,
        }
        if conversation_type == "group":
            if not name:
                raise ValueError("group conversation requires name")
            body["name"] = name
        elif name:
            # direct conversation does not require name, but keep explicit user input for compatibility.
            body["name"] = name
        return self._post("/api/conversations", body)

    def create_group(self, member_ids: list, name: str = "", avatar: str = "") -> dict:
        if not name:
            raise ValueError("group conversation requires name")
        body = {"member_ids": member_ids, "name": name}
        if avatar:
            body["avatar"] = avatar
        return self._post("/api/conversations/group", body)

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

    def _delete(self, path: str, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.delete(self._path(path), headers=h, **kwargs)
        data = self._safe_json(r)
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.delete(self._path(path), headers=h, **kwargs)
        data = r.json()
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})

    def _put(self, path: str, body=None, **kwargs) -> dict:
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.put(self._path(path), json=body, headers=h, **kwargs)
        data = self._safe_json(r)
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})
        h = {**self.token.auth_header(), **kwargs.pop("headers", {})}
        r = self.session.put(self._path(path), json=body, headers=h, **kwargs)
        data = r.json()
        if data.get("code", 0) != 0:
            raise APIError(data["code"], data.get("msg", "unknown error"))
        return data.get("body", {})

    def get_conversation_members(self, conversation_id: int) -> dict:
        return self._get(f"/api/conversations/{conversation_id}/members")

    def add_group_members(self, conversation_id: int, member_ids: list) -> dict:
        return self._post(f"/api/conversations/{conversation_id}/members", {"member_ids": member_ids})

    def remove_group_member(self, conversation_id: int, user_id: int) -> dict:
        return self._delete(f"/api/conversations/{conversation_id}/members/{user_id}")

    def leave_group(self, conversation_id: int) -> dict:
        return self._post(f"/api/conversations/{conversation_id}/leave")

    def dismiss_group(self, conversation_id: int) -> dict:
        return self._delete(f"/api/conversations/{conversation_id}")

    def update_group_info(self, conversation_id: int, name: str = None, avatar: str = None) -> dict:
        body = {}
        if name is not None:
            body["name"] = name
        if avatar is not None:
            body["avatar"] = avatar
        return self._put(f"/api/conversations/{conversation_id}", body)

    # ── Search ──

    def unified_search(self, query: str, scope: str = "", conversation_id: int = 0,
                       cursor_created_at: int = 0, cursor_id: int = 0, limit: int = 20) -> dict:
        params = [f"q={query}"]
        if scope:
            params.append(f"scope={scope}")
        if conversation_id:
            params.append(f"conversation_id={conversation_id}")
        if cursor_created_at:
            params.append(f"cursor_created_at={cursor_created_at}")
        if cursor_id:
            params.append(f"cursor_id={cursor_id}")
        if limit:
            params.append(f"limit={limit}")
        qs = "?" + "&".join(params)
        return self._get(f"/api/search{qs}")

    # ── Attachments ──

    def init_attachment_upload(self, conversation_id: int, kind: str, original_name: str,
                                 mime: str, size: int, sha256: str = "") -> dict:
        body = {
            "conversation_id": conversation_id,
            "kind": kind,
            "original_name": original_name,
            "mime": mime,
            "size": size,
        }
        if sha256:
            body["sha256"] = sha256
        return self._post("/api/attachments/init", body)

    def get_attachment(self, file_id: str) -> dict:
        return self._get(f"/api/attachments/{file_id}")

    def complete_attachment_upload(self, file_id: str, sha256: str = "") -> dict:
        body = {}
        if sha256:
            body["sha256"] = sha256
        return self._post(f"/api/attachments/{file_id}/complete", body)

    def download_attachment(self, file_id: str) -> dict:
        return self._get(f"/api/attachments/{file_id}/download")

    # ── Group Admin ──

    def grant_group_admin(self, conversation_id: int, user_id: int) -> dict:
        return self._post(f"/api/conversations/{conversation_id}/members/{user_id}/admin")

    def revoke_group_admin(self, conversation_id: int, user_id: int) -> dict:
        return self._delete(f"/api/conversations/{conversation_id}/members/{user_id}/admin")

    def transfer_group_owner(self, conversation_id: int, user_id: int) -> dict:
        return self._post(f"/api/conversations/{conversation_id}/owner",
                           {"user_id": user_id})

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
                 reconnect_max_retries: int = None,
                 rest_client: 'RESTClient' = None):
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
        self._rest_client = rest_client
        self._seq_lock = threading.RLock()
        # 客户端已连续处理的最大白名单 pending 推送 seq；自动心跳会作为 HeartbeatPayload.last_seq 上报。
        self._last_pending_seq = 0

    def _next_seq(self) -> int:
        with self._seq_lock:
            self.seq += 1
            return self.seq

    def last_pending_seq(self) -> int:
        """Return the largest processed replayable server push seq."""
        with self._seq_lock:
            return self._last_pending_seq

    def _mark_replayable_frame_processed(self, frame: ws_pb2.WsFrame):
        """Advance local replay cursor for server-side L1 pending cleanup."""
        if frame.type not in REPLAYABLE_PUSH_TYPES or frame.seq <= 0:
            return
        with self._seq_lock:
            if frame.seq > self._last_pending_seq:
                self._last_pending_seq = frame.seq

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
                    _ws_debug_print(f"  ✗ WS connection failed: {self._error}")
                self.disconnect()
                return
            if not t.is_alive():
                if VERBOSE:
                    _ws_debug_print(f"  ✗ WS thread died unexpectedly")
                self.disconnect()
                return
            time.sleep(0.1)
        if not self._connected:
            if VERBOSE:
                if self._error:
                    _ws_debug_print(f"  ✗ WS connection failed: {self._error}")
                else:
                    _ws_debug_print(f"  ✗ WS connection timed out after {int(connect_timeout)}s")
            self.disconnect()
        else:
            # 连接成功，启动心跳
            self._reconnect_count = 0
            self._start_heartbeat()

    def reconnect(self, max_retries: int = None) -> bool:
        """Attempt to reconnect with exponential backoff.

        If the access token is expired and a RESTClient is available,
        automatically refresh the token before reconnecting.

        Returns True if reconnection succeeds, False otherwise.
        """
        self._try_refresh_token()
        retries = max_retries if max_retries is not None else self._reconnect_max
        for attempt in range(retries):
            backoff = self.RECONNECT_BACKOFF_BASE * (2 ** attempt)
            if VERBOSE:
                _ws_debug_print(f"  ↻ WS reconnect attempt {attempt + 1}/{retries} in {backoff:.1f}s...")
            time.sleep(backoff)
            self.connect()
            if self._connected:
                if VERBOSE:
                    _ws_debug_print(f"  ✓ WS reconnected on attempt {attempt + 1}")
                return True
        if VERBOSE:
            _ws_debug_print(f"  ✗ WS reconnect failed after {retries} attempts")
        return False

    def _try_refresh_token(self):
        """Refresh access token if expired and a RESTClient is available."""
        if not self.token.is_expired():
            return
        if self._rest_client is None:
            if VERBOSE:
                _ws_debug_print("  ⚠ Token expired but no RESTClient for refresh")
            return
        try:
            self._rest_client.refresh()
            if VERBOSE:
                _ws_debug_print(f"  ✓ Token refreshed (user_id={self.token.user_id})")
        except Exception as e:
            if VERBOSE:
                _ws_debug_print(f"  ✗ Token refresh failed: {e}")

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
            _ws_debug_print(f"  ✓ WS connected to {self.url}")

    def _on_message(self, ws, data: bytes):
        try:
            frame = ws_pb2.WsFrame()
            frame.ParseFromString(data)
            ftype = FRAME_TYPE_NAMES.get(frame.type, f"UNKNOWN({frame.type})")
            payload = self._decode_payload(frame)
            self._mark_replayable_frame_processed(frame)
            if VERBOSE:
                _ws_debug_print(f"\n  ← [{ftype}] seq={frame.seq}")
                if payload:
                    _ws_debug_print(f"    {json_format.MessageToDict(payload, preserving_proto_field_name=True)}")
            if self.on_frame:
                self.on_frame(frame, payload)
        except Exception as e:
            if VERBOSE:
                _ws_debug_print(f"  ✗ Decode error: {e}")

    def _on_error(self, ws, error):
        self._error = str(error)
        if VERBOSE:
            _ws_debug_print(f"  ✗ WS error: {error}")

    def _on_close(self, ws, status, msg):
        self._connected = False
        self._stop_heartbeat()
        if VERBOSE:
            _ws_debug_print(f"  ✓ WS closed (status={status})")
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
            ws_pb2.FRAME_TYPE_PUSH_READ_RECEIPT: ws_pb2.PushReadReceiptPayload,
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
            _ws_debug_print(f"  → [{ftype}] seq={frame.seq}{payload_info}")

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

    def send_message(self, conversation_id: int, content: str, message_type: str = "text",
                     client_msg_id: str = "", mentions: Optional[list[str]] = None) -> str:
        """Send a chat message and return the client_msg_id for correlation."""
        payload = ws_pb2.SendMessagePayload()
        payload.conversation_id = conversation_id
        payload.message_type = message_type
        payload.content = content
        client_msg_id = client_msg_id or str(uuid.uuid4())
        payload.client_msg_id = client_msg_id
        if mentions:
            payload.mentions.extend(str(m) for m in mentions)
        self._send_frame(ws_pb2.FRAME_TYPE_SEND_MESSAGE, payload)
        return client_msg_id

    def send_heartbeat(self, last_seq: Optional[int] = None):
        payload = ws_pb2.HeartbeatPayload()
        if last_seq is None:
            last_seq = self.last_pending_seq()
        payload.last_seq = max(int(last_seq), 0)
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

    def send_ack(self, ack_seq: int):
        payload = ws_pb2.ClientAckPayload()
        payload.ack_seq = max(int(ack_seq), 0)
        with self._seq_lock:
            if payload.ack_seq > self._last_pending_seq:
                self._last_pending_seq = payload.ack_seq
        self._send_frame(ws_pb2.FRAME_TYPE_ACK, payload)


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


def cmd_presence_friends(args):
    client = RESTClient()
    try:
        friends = client.get_friends_presence()
        print(f"✓ {len(friends)} friend presence record(s):")
        print_json(friends)
    except APIError as e:
        print(f"✗ Get friends presence failed: {e}")


def cmd_friend_tag_list(args):
    client = RESTClient()
    try:
        tags = client.list_friend_tags()
        print(f"✓ {len(tags)} tag(s):")
        print_json(tags)
    except APIError as e:
        print(f"✗ List tags failed: {e}")


def cmd_friend_tag_create(args):
    client = RESTClient()
    try:
        tag = client.create_friend_tag(args.name)
        print(f"✓ Tag created:")
        print_json(tag)
    except APIError as e:
        print(f"✗ Create tag failed: {e}")


def cmd_friend_tag_rename(args):
    client = RESTClient()
    try:
        tag = client.rename_friend_tag(args.id, args.name)
        print(f"✓ Tag renamed:")
        print_json(tag)
    except APIError as e:
        print(f"✗ Rename tag failed: {e}")


def cmd_friend_tag_delete(args):
    client = RESTClient()
    try:
        result = client.delete_friend_tag(args.id)
        print(f"✓ Deleted: {result.get('deleted', False)}")
    except APIError as e:
        print(f"✗ Delete tag failed: {e}")


def cmd_friend_tags_set(args):
    client = RESTClient()
    try:
        tag_ids = [int(x.strip()) for x in args.tag_ids.split(",") if x.strip()]
        result = client.set_friend_tags(args.friend_id, tag_ids)
        print(f"✓ Tags set for friend {args.friend_id}:")
        print_json(result)
    except APIError as e:
        print(f"✗ Set tags failed: {e}")


def cmd_friend_tag_remove(args):
    client = RESTClient()
    try:
        result = client.remove_friend_tag(args.friend_id, args.tag_id)
        print(f"✓ Tag {args.tag_id} removed from friend {args.friend_id}:")
        print_json(result)
    except APIError as e:
        print(f"✗ Remove tag failed: {e}")


def cmd_search_unified(args):
    client = RESTClient()
    try:
        result = client.unified_search(
            args.q, args.scope, args.conversation_id,
            args.cursor_created_at, args.cursor_id, args.limit,
        )
        print(f"✓ Search results for '{args.q}':")
        for kind in ["users", "friends", "conversations", "messages"]:
            items = result.get(kind, [])
            if items:
                print(f"  ── {kind.capitalize()} ({len(items)}):")
                print_json(items)
        print(f"  next_cursor_created_at={result.get('next_cursor_created_at')}, next_cursor_id={result.get('next_cursor_id')}, has_more={result.get('has_more')}")
    except APIError as e:
        print(f"✗ Search failed: {e}")


    client = RESTClient()
    try:
        member_ids = [int(m.strip()) for m in args.member_ids.split(",")] if args.member_ids else [args.member_id] if args.member_id else []
        if not member_ids:
            print("✗ At least one --member-id or --member-ids is required")
            return
        name = args.name or ""
        conv = client.create_conversation(member_ids, name)
        print(f"✓ Conversation #{conv['conversation_id']} created ({conv['conversation_type']})")
        if len(member_ids) > 1:
            print(f"  Members: {member_ids}")
        if name:
            print(f"  Name: {name}")
        print_json(conv)
    except (APIError, ValueError) as e:
        print(f"✗ Create conversation failed: {e}")


def cmd_create_group(args):
    client = RESTClient()
    try:
        member_ids = [int(m.strip()) for m in args.member_ids.split(",")] if args.member_ids else [args.member_id] if args.member_id else []
        if not member_ids:
            print("✗ At least one --member-id or --member-ids is required")
            return
        name = args.name or ""
        avatar = args.avatar or ""
        conv = client.create_group(member_ids, name, avatar)
        print(f"✓ Group #{conv['conversation_id']} created")
        print(f"  Members: {member_ids}")
        if name:
            print(f"  Name: {name}")
        if avatar:
            print(f"  Avatar: {avatar}")
        print_json(conv)
    except (APIError, ValueError) as e:
        print(f"✗ Create group failed: {e}")


def cmd_create_conversation(args):
    client = RESTClient()
    try:
        if args.member_ids:
            member_ids = [int(m.strip()) for m in args.member_ids.split(",")]
        elif args.member_id:
            member_ids = [args.member_id]
        else:
            print("✗ --member-id or --member-ids required")
            return
        conv = client.create_conversation(member_ids, args.name)
        print(f"✓ Conversation created: #{conv.get('id', 'unknown')}")
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


def cmd_conv_members(args):
    client = RESTClient()
    try:
        resp = client.get_conversation_members(args.conversation_id)
        members = resp.get("members", [])
        print(f"✓ {len(members)} member(s) in conversation #{args.conversation_id}:")
        print_json(members)
    except APIError as e:
        print(f"✗ Get members failed: {e}")


def cmd_add_group_members(args):
    client = RESTClient()
    try:
        member_ids = [int(m.strip()) for m in args.member_ids.split(",")]
        resp = client.add_group_members(args.conversation_id, member_ids)
        print(f"✓ Added {len(member_ids)} member(s) to conversation #{args.conversation_id}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Add group members failed: {e}")


def cmd_remove_group_member(args):
    client = RESTClient()
    try:
        client.remove_group_member(args.conversation_id, args.user_id)
        print(f"✓ Removed user #{args.user_id} from conversation #{args.conversation_id}")
    except APIError as e:
        print(f"✗ Remove group member failed: {e}")


def cmd_leave_group(args):
    client = RESTClient()
    try:
        client.leave_group(args.conversation_id)
        print(f"✓ Left conversation #{args.conversation_id}")
    except APIError as e:
        print(f"✗ Leave group failed: {e}")


def cmd_dismiss_group(args):
    client = RESTClient()
    try:
        client.dismiss_group(args.conversation_id)
        print(f"✓ Dismissed conversation #{args.conversation_id}")
    except APIError as e:
        print(f"✗ Dismiss group failed: {e}")


def cmd_update_group_info(args):
    client = RESTClient()
    try:
        resp = client.update_group_info(args.conversation_id, args.name, args.avatar)
        print(f"✓ Updated conversation #{args.conversation_id}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Update group info failed: {e}")


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
        mentions = [m.strip() for m in (getattr(args, "mentions", "") or "").split(",") if m.strip()]
        ws.send_message(args.conversation_id, args.content, args.message_type or "text", mentions=mentions)
    except Exception as e:
        print(f"✗ Send failed: {e}")


def cmd_ws_heartbeat(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    last_seq = getattr(args, "last_seq", None)
    ws.send_heartbeat(last_seq=last_seq)


def cmd_ws_typing(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    ws.send_typing(args.conversation_id)


def cmd_ws_read_receipt(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    ws.send_read_receipt(args.conversation_id, args.last_msg_id)


def cmd_ws_ack(args):
    profile = getattr(args, "profile", "") or ""
    ws = _get_ws_client(profile)
    if not ws or not ws.is_connected():
        print("✗ Not connected. Run 'ws-connect' first.")
        return
    ws.send_ack(args.ack_seq)


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
        return f"ws:   ✓ connected last_seq={ws.last_pending_seq()}"
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
│  friend-list          presence-friends             │
├─ Conversations ──────────────────────────────────┤
│  conv-list                                         │
│  conv-create <member_id> [name]  (name required for group) │
│  group-create <member_id> <name> (or comma-sep)     │
│  conv-members <conv_id>                            │
│  conv-add-members <conv_id> <uid,uid,...>          │
│  conv-remove-member <conv_id> <uid>                │
│  conv-leave <conv_id>                              │
│  conv-dismiss <conv_id>                            │
│  conv-update <conv_id> [--name N] [--avatar A]     │
│  history <conversation_id> [limit]                │
├─ WebSocket ───────────────────────────────────────┤
│  ws-connect [--profile NAME]                      │
│  ws-send <conv_id> <text> [--profile NAME]        │
│  ws-heartbeat [last_seq] [--profile NAME]         │
│  ws-typing <id> [--profile NAME]                  │
│  ws-read-receipt <conv_id> <last_msg_id>          │
│  ws-ack <ack_seq> [--profile NAME]                │
│  ws-recv (wait for incoming frames)               │
├─ Profiles ────────────────────────────────────────┤
│  switch <profile>   change active profile          │
│  status             show all profiles              │
├─ Meta ────────────────────────────────────────────┤
│  help | quit | exit | status                      │
└───────────────────────────────────────────────────┘""")



# ── Attachment commands ──

def cmd_attachment_init(args):
    client = RESTClient()
    try:
        resp = client.init_attachment_upload(
            args.conversation_id, args.kind, args.original_name,
            args.mime, args.size, args.sha256)
        print(f"✓ Attachment upload initialized: file_id={resp.get('file_id', 'unknown')}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Attachment init failed: {e}")

def cmd_attachment_get(args):
    client = RESTClient()
    try:
        resp = client.get_attachment(args.file_id)
        print(f"✓ Attachment #{args.file_id}:")
        print_json(resp)
    except APIError as e:
        print(f"✗ Attachment get failed: {e}")

def cmd_attachment_complete(args):
    client = RESTClient()
    try:
        resp = client.complete_attachment_upload(args.file_id, args.sha256)
        print(f"✓ Attachment #{args.file_id} completed (status={resp.get('status', 'unknown')})")
        print_json(resp)
    except APIError as e:
        print(f"✗ Attachment complete failed: {e}")

def cmd_attachment_download(args):
    client = RESTClient()
    try:
        resp = client.download_attachment(args.file_id)
        print(f"✓ Download URL for attachment #{args.file_id}:")
        print_json(resp)
    except APIError as e:
        print(f"✗ Attachment download failed: {e}")


# ── Group Admin commands ──

def cmd_grant_group_admin(args):
    client = RESTClient()
    try:
        resp = client.grant_group_admin(args.conversation_id, args.user_id)
        print(f"✓ User #{args.user_id} granted admin in conversation #{args.conversation_id}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Grant admin failed: {e}")

def cmd_revoke_group_admin(args):
    client = RESTClient()
    try:
        resp = client.revoke_group_admin(args.conversation_id, args.user_id)
        print(f"✓ User #{args.user_id} revoked admin in conversation #{args.conversation_id}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Revoke admin failed: {e}")

def cmd_transfer_group_owner(args):
    client = RESTClient()
    try:
        resp = client.transfer_group_owner(args.conversation_id, args.user_id)
        print(f"✓ Group #{args.conversation_id} owner transferred to user #{args.user_id}")
        print_json(resp)
    except APIError as e:
        print(f"✗ Transfer owner failed: {e}")


# ── Bot extended commands ──

def cmd_bot_conv_history(args):
    token = _bot_token_for(args)
    params = []
    if args.cursor_created_at:
        params.append(f"cursor_created_at={args.cursor_created_at}")
    if args.cursor_id:
        params.append(f"cursor_id={args.cursor_id}")
    if args.limit:
        params.append(f"limit={args.limit}")
    qs = "?" + "&".join(params) if params else ""
    body = _bot_request("GET", f"/api/bot/v1/conversations/{args.conversation_id}/history{qs}", token)
    msgs = body.get("messages", [])
    print(f"✓ {len(msgs)} message(s) in conversation {args.conversation_id}")
    print_json(body)

def cmd_bot_conv_members(args):
    token = _bot_token_for(args)
    body = _bot_request("GET", f"/api/bot/v1/conversations/{args.conversation_id}/members", token)
    members = body.get("members", [])
    print(f"✓ {len(members)} member(s) in conversation {args.conversation_id}")
    print_json(members)

def cmd_bot_mark_read(args):
    token = _bot_token_for(args)
    body = _bot_request("POST", f"/api/bot/v1/conversations/{args.conversation_id}/read-receipt", token,
                        {"last_read_message_id": args.last_read_message_id})
    print(f"✓ Marked read in conversation {args.conversation_id}")
    print_json(body)

def cmd_bot_list_read_states(args):
    token = _bot_token_for(args)
    body = _bot_request("GET", f"/api/bot/v1/conversations/{args.conversation_id}/read-states", token)
    states = body.get("read_states", [])
    print(f"✓ {len(states)} read state(s) in conversation {args.conversation_id}")
    print_json(states)

def cmd_bot_download_attachment(args):
    token = _bot_token_for(args)
    body = _bot_request("GET", f"/api/bot/v1/attachments/{args.file_id}/download", token)
    print(f"✓ Download URL for bot attachment #{args.file_id}:")
    print_json(body)

def _bot_token_for(args) -> str:
    """Resolve the Bot OpenAPI token from --token flag or AIM_BOT_TOKEN env."""
    tok = getattr(args, "token", None) or os.environ.get("AIM_BOT_TOKEN", "")
    if not tok:
        raise SystemExit("missing bot token: pass --token <token> or set AIM_BOT_TOKEN")
    return tok


def _bot_request(method: str, path: str, token: str, body=None) -> dict:
    """Issue a Bot OpenAPI request and unwrap the {code,msg,body} envelope."""
    url = f"{GATEWAY_HTTP}{path}"
    headers = {"Authorization": f"Bot {token}", "Content-Type": "application/json"}
    r = requests.request(method, url, headers=headers, json=body, timeout=10)
    try:
        data = r.json()
    except ValueError:
        raise SystemExit(f"bot {method} {path} failed: {r.status_code} {r.text!r}")
    if data.get("code", 0) != 0:
        raise SystemExit(f"bot {method} {path} failed: code={data.get('code')} msg={data.get('msg')}")
    return data.get("body", {}) or {}


def cmd_bot_me(args):
    body = _bot_request("GET", "/api/bot/v1/me", _bot_token_for(args))
    print_json(body)


def cmd_bot_conv_list(args):
    body = _bot_request("GET", "/api/bot/v1/conversations", _bot_token_for(args))
    convs = body.get("conversations") or []
    print(f"✓ Bot is in {len(convs)} conversation(s):")
    print_json(convs)


def cmd_bot_send(args):
    payload = {
        "conversation_id": str(args.conversation_id),
        "message_type": args.message_type,
        "content": args.content,
        "client_msg_id": args.client_msg_id or str(uuid.uuid4()),
    }
    if args.mentions:
        payload["mentions"] = [m.strip() for m in args.mentions.split(",") if m.strip()]
    body = _bot_request("POST", "/api/bot/v1/messages", _bot_token_for(args), payload)
    print(f"✓ Sent message_id={body.get('message_id')} client_msg_id={body.get('client_msg_id')}")
    print_json(body)


def cmd_bot_webhook_get(args):
    body = _bot_request("GET", "/api/bot/v1/webhook", _bot_token_for(args))
    print_json(body)


def cmd_bot_webhook_set(args):
    payload = {"url": args.url}
    if args.events:
        payload["events"] = [e.strip() for e in args.events.split(",") if e.strip()]
    if args.enabled is not None:
        payload["enabled"] = args.enabled
    if args.secret:
        payload["secret"] = args.secret
    if args.rotate_secret:
        payload["rotate_secret"] = True
    body = _bot_request("PUT", "/api/bot/v1/webhook", _bot_token_for(args), payload)
    if body.get("plaintext_secret"):
        print("⚠  Plaintext webhook secret (shown once):")
        print(f"   {body['plaintext_secret']}")
    print_json(body.get("webhook"))


def cmd_bot_webhook_delete(args):
    body = _bot_request("DELETE", "/api/bot/v1/webhook", _bot_token_for(args))
    print_json(body)


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

    session = PromptSession() if PromptSession is not None else None

    while True:
        try:
            # Build dynamic prompt with compact status
            auth_hint = f"#{client.token.user_id}" if client.token.access_token and not client.token.is_expired() else "?"
            ws = _get_ws_client(_profile_key())
            ws_hint = "⚡" if (ws and ws.is_connected()) else "·"
            profile_hint = _active_profile
            prompt = f"aim [{profile_hint}] [{auth_hint}] {ws_hint}> "
            if session is not None and patch_stdout is not None:
                with patch_stdout(raw=True):
                    line = session.prompt(prompt).strip()
            else:
                line = input(prompt).strip()

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
            elif cmd == "presence-friends":
                friends = client.get_friends_presence()
                print(f"✓ {len(friends)} friend presence record(s):")
                print_json(friends)
            elif cmd == "friend-tags":
                tags = client.list_friend_tags()
                print(f"✓ {len(tags)} tag(s):")
                print_json(tags)
            elif cmd == "friend-tag-create" and len(parts) >= 2:
                tag = client.create_friend_tag(parts[1])
                print(f"✓ Tag created:")
                print_json(tag)
            elif cmd == "friend-tag-rename" and len(parts) >= 3:
                tag = client.rename_friend_tag(int(parts[1]), parts[2])
                print(f"✓ Tag renamed:")
                print_json(tag)
            elif cmd == "friend-tag-delete" and len(parts) >= 2:
                result = client.delete_friend_tag(int(parts[1]))
                print(f"✓ Deleted: {result.get('deleted', False)}")
            elif cmd == "friend-tags-set" and len(parts) >= 3:
                tag_ids = [int(x.strip()) for x in parts[2].split(",") if x.strip()]
                result = client.set_friend_tags(int(parts[1]), tag_ids)
                print(f"✓ Tags set:")
                print_json(result)
            elif cmd == "friend-tag-remove" and len(parts) >= 3:
                result = client.remove_friend_tag(int(parts[1]), int(parts[2]))
                print(f"✓ Tag removed:")
                print_json(result)
            elif cmd == "search-unified" and len(parts) >= 2:
                result = client.unified_search(parts[1])
                print(f"✓ Search results for '{parts[1]}':")
                print_json(result)

                convs = client.list_conversations()
                print(f"✓ {len(convs)} conversation(s):")
                print_json(convs)
            elif cmd == "conv-create" and len(parts) >= 2:
                member_ids = [int(m.strip()) for m in parts[1].split(",")]
                name = parts[2] if len(parts) > 2 else ""
                conv = client.create_conversation(member_ids, name)
                print(f"✓ Conversation #{conv['conversation_id']} created ({conv['conversation_type']})")
                if len(member_ids) > 1:
                    print(f"  Members: {member_ids}")
                if name:
                    print(f"  Name: {name}")
                print_json(conv)
            elif cmd == "group-create" and len(parts) >= 2:
                member_ids = [int(m.strip()) for m in parts[1].split(",")]
                name = ""
                avatar = ""
                i = 2
                while i < len(parts):
                    if parts[i] == "--name" and i + 1 < len(parts):
                        name = parts[i + 1]
                        i += 2
                    elif parts[i] == "--avatar" and i + 1 < len(parts):
                        avatar = parts[i + 1]
                        i += 2
                    else:
                        name = parts[i]
                        i += 1
                conv = client.create_group(member_ids, name, avatar)
                print(f"✓ Group #{conv['conversation_id']} created")
                print(f"  Members: {member_ids}")
                if name:
                    print(f"  Name: {name}")
                if avatar:
                    print(f"  Avatar: {avatar}")
                print_json(conv)
            elif cmd == "conv-members" and len(parts) >= 2:
                resp = client.get_conversation_members(int(parts[1]))
                members = resp.get("members", [])
                print(f"✓ {len(members)} member(s):")
                print_json(members)
            elif cmd == "conv-add-members" and len(parts) >= 3:
                member_ids = [int(m.strip()) for m in parts[2].split(",")]
                resp = client.add_group_members(int(parts[1]), member_ids)
                print(f"✓ Added {len(member_ids)} member(s)")
                print_json(resp)
            elif cmd == "conv-remove-member" and len(parts) >= 3:
                client.remove_group_member(int(parts[1]), int(parts[2]))
                print(f"✓ Removed user #{parts[2]}")
            elif cmd == "conv-leave" and len(parts) >= 2:
                client.leave_group(int(parts[1]))
                print(f"✓ Left conversation #{parts[1]}")
            elif cmd == "conv-dismiss" and len(parts) >= 2:
                client.dismiss_group(int(parts[1]))
                print(f"✓ Dismissed conversation #{parts[1]}")
            elif cmd == "conv-update" and len(parts) >= 2:
                update_name = None
                update_avatar = None
                i = 2
                while i < len(parts):
                    if parts[i] == "--name" and i + 1 < len(parts):
                        update_name = parts[i + 1]
                        i += 2
                    elif parts[i] == "--avatar" and i + 1 < len(parts):
                        update_avatar = parts[i + 1]
                        i += 2
                    else:
                        i += 1
                resp = client.update_group_info(int(parts[1]), update_name, update_avatar)
                print(f"✓ Updated conversation #{parts[1]}")
                print_json(resp)
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
                last_seq = int(parts[1]) if len(parts) >= 2 else None
                ws.send_heartbeat(last_seq=last_seq)
            elif cmd == "ws-typing" and len(parts) >= 2:
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                ws.send_typing(int(parts[1]))
            elif cmd == "ws-read-receipt" and len(parts) >= 3:
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                ws.send_read_receipt(int(parts[1]), int(parts[2]))
            elif cmd == "ws-ack" and len(parts) >= 2:
                ws = _get_ws_client(_profile_key())
                if not ws or not ws.is_connected():
                    print(f"✗ Not connected (profile={_active_profile}). Run 'ws-connect' first.")
                    continue
                ws.send_ack(int(parts[1]))
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

    # 4.6. Check friend presence endpoint
    print("\n── 4.6. Friend Presence ──")
    alice_presence = client_alice.get_friends_presence()
    print(f"  ✓ Alice sees {len(alice_presence)} friend presence record(s)")

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
    sub.add_parser("presence-friends", help="List friends presence")

    # Friend Tags
    p = sub.add_parser("friend-tags", help="List friend tags")

    p = sub.add_parser("friend-tag-create", help="Create a friend tag")
    p.add_argument("--name", required=True, help="Tag name")

    p = sub.add_parser("friend-tag-rename", help="Rename a friend tag")
    p.add_argument("--id", type=int, required=True, help="Tag ID")
    p.add_argument("--name", required=True, help="New name")

    p = sub.add_parser("friend-tag-delete", help="Delete a friend tag")
    p.add_argument("--id", type=int, required=True, help="Tag ID")

    p = sub.add_parser("friend-tags-set", help="Set tags for a friend")
    p.add_argument("--friend-id", type=int, required=True, help="Friend user ID")
    p.add_argument("--tag-ids", required=True, help="Comma-separated tag IDs")

    p = sub.add_parser("friend-tag-remove", help="Remove a single tag from a friend")
    p.add_argument("--friend-id", type=int, required=True, help="Friend user ID")
    p.add_argument("--tag-id", type=int, required=True, help="Tag ID to remove")

    # Search
    p = sub.add_parser("search-unified", help="Unified search")
    p.add_argument("--q", required=True, help="Search query")
    p.add_argument("--scope", default="", help="Comma-separated scopes (users,friends,conversations,messages)")
    p.add_argument("--conversation-id", type=int, default=0, help="Conversation scope for message search")
    p.add_argument("--cursor-created-at", type=int, default=0, help="Message search pagination cursor")
    p.add_argument("--cursor-id", type=int, default=0, help="Message search pagination cursor")
    p.add_argument("--limit", type=int, default=20, help="Results per page, default 20, max 100")

    p = sub.add_parser("conv-list", help="List conversations")
    p = sub.add_parser("conv-create", help="Create conversation")
    p.add_argument("--member-id", type=int, help="Single member ID")
    p.add_argument("--member-ids", help="Comma-separated member IDs (e.g. 2,3,4)")
    p.add_argument("--name", default="", help="Group name (required when creating a group; direct conversation does not require name)")

    p = sub.add_parser("group-create", help="Create group conversation")
    p.add_argument("--member-id", type=int, help="Single member ID")
    p.add_argument("--member-ids", help="Comma-separated member IDs (e.g. 2,3,4)")
    p.add_argument("--name", required=True, help="Group name (required)")
    p.add_argument("--avatar", default="", help="Group avatar URL (optional)")

    p = sub.add_parser("conv-members", help="Get conversation member details")
    p.add_argument("--conversation-id", type=int, required=True)

    p = sub.add_parser("conv-add-members", help="Add members to a group")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--member-ids", required=True, help="Comma-separated member IDs to add")

    p = sub.add_parser("conv-remove-member", help="Remove a member from a group")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--user-id", type=int, required=True)

    p = sub.add_parser("conv-leave", help="Leave a group conversation")
    p.add_argument("--conversation-id", type=int, required=True)

    p = sub.add_parser("conv-dismiss", help="Dismiss a group conversation")
    p.add_argument("--conversation-id", type=int, required=True)

    p = sub.add_parser("conv-update", help="Update group info (name/avatar)")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--name", default=None, help="New group name")
    p.add_argument("--avatar", default=None, help="New group avatar URL")

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
    p.add_argument("--mentions", default="", help="Comma-separated user IDs to mention")
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-heartbeat", help="Send heartbeat via WebSocket")
    p.add_argument("--profile", default="", help="Profile name")
    p.add_argument("--last-seq", type=int, default=None, help="Override HeartbeatPayload.last_seq; defaults to tracked replay cursor")
    p = sub.add_parser("ws-typing", help="Send typing indicator")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-read-receipt", help="Send read receipt via WebSocket")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--last-msg-id", type=int, required=True)
    p.add_argument("--profile", default="", help="Profile name")
    p = sub.add_parser("ws-ack", help="Send client ACK via WebSocket")
    p.add_argument("--ack-seq", type=int, required=True)
    p.add_argument("--profile", default="", help="Profile name")

    # Bot OpenAPI
    bot_token_help = "Bot token (defaults to AIM_BOT_TOKEN env)"

    p = sub.add_parser("bot-me", help="GET /api/bot/v1/me")
    p.add_argument("--token", default="", help=bot_token_help)

    p = sub.add_parser("bot-conv-list", help="GET /api/bot/v1/conversations")
    p.add_argument("--token", default="", help=bot_token_help)

    p = sub.add_parser("bot-send", help="POST /api/bot/v1/messages")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--content", required=True)
    p.add_argument("--message-type", default="text")
    p.add_argument("--client-msg-id", default="", help="Idempotency key (defaults to a fresh UUID)")
    p.add_argument("--mentions", default="", help="Comma-separated user IDs to mention")

    p = sub.add_parser("bot-webhook-get", help="GET /api/bot/v1/webhook")
    p.add_argument("--token", default="", help=bot_token_help)

    p = sub.add_parser("bot-webhook-set", help="PUT /api/bot/v1/webhook")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--url", required=True)
    p.add_argument("--events", default="", help="Comma-separated event names (default: message.created)")
    enabled_group = p.add_mutually_exclusive_group()
    enabled_group.add_argument("--enable", dest="enabled", action="store_true", default=None)
    enabled_group.add_argument("--disable", dest="enabled", action="store_false", default=None)
    secret_group = p.add_mutually_exclusive_group()
    secret_group.add_argument("--secret", default="", help="Provide a webhook signing secret")
    secret_group.add_argument("--rotate-secret", action="store_true", help="Generate a fresh signing secret server-side")

    p = sub.add_parser("bot-webhook-delete", help="DELETE /api/bot/v1/webhook")
    p.add_argument("--token", default="", help=bot_token_help)

    # Attachments
    p = sub.add_parser("attachment-init", help="POST /api/attachments/init")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--kind", required=True, choices=["image", "video", "audio", "file"])
    p.add_argument("--original-name", required=True)
    p.add_argument("--mime", required=True)
    p.add_argument("--size", type=int, required=True)
    p.add_argument("--sha256", default="")

    p = sub.add_parser("attachment-get", help="GET /api/attachments/:id")
    p.add_argument("--file-id", required=True)

    p = sub.add_parser("attachment-complete", help="POST /api/attachments/:id/complete")
    p.add_argument("--file-id", required=True)
    p.add_argument("--sha256", default="")

    p = sub.add_parser("attachment-download", help="GET /api/attachments/:id/download")
    p.add_argument("--file-id", required=True)

    # Group Admin
    p = sub.add_parser("group-grant-admin", help="POST /api/conversations/:id/members/:uid/admin")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--user-id", type=int, required=True)

    p = sub.add_parser("group-revoke-admin", help="DELETE /api/conversations/:id/members/:uid/admin")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--user-id", type=int, required=True)

    p = sub.add_parser("group-transfer-owner", help="POST /api/conversations/:id/owner")
    p.add_argument("--conversation-id", type=int, required=True)
    p.add_argument("--user-id", type=int, required=True)

    # Bot extended
    p = sub.add_parser("bot-history", help="GET /api/bot/v1/conversations/:id/history")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--conversation-id", required=True)
    p.add_argument("--cursor-created-at", type=int)
    p.add_argument("--cursor-id", default="")
    p.add_argument("--limit", type=int, default=50)

    p = sub.add_parser("bot-members", help="GET /api/bot/v1/conversations/:id/members")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--conversation-id", required=True)

    p = sub.add_parser("bot-mark-read", help="POST /api/bot/v1/conversations/:id/read-receipt")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--conversation-id", required=True)
    p.add_argument("--last-read-message-id", required=True)

    p = sub.add_parser("bot-read-states", help="GET /api/bot/v1/conversations/:id/read-states")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--conversation-id", required=True)

    p = sub.add_parser("bot-download-attachment", help="GET /api/bot/v1/attachments/:id/download")
    p.add_argument("--token", default="", help=bot_token_help)
    p.add_argument("--file-id", required=True)

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
        "presence-friends": cmd_presence_friends,
        "friend-tags": cmd_friend_tag_list,
        "friend-tag-create": cmd_friend_tag_create,
        "friend-tag-rename": cmd_friend_tag_rename,
        "friend-tag-delete": cmd_friend_tag_delete,
        "friend-tags-set": cmd_friend_tags_set,
        "friend-tag-remove": cmd_friend_tag_remove,
        "search-unified": cmd_search_unified,
        "conv-list": cmd_list_conversations,
        "conv-create": cmd_create_conversation,
        "group-create": cmd_create_group,
        "conv-members": cmd_conv_members,
        "conv-add-members": cmd_add_group_members,
        "conv-remove-member": cmd_remove_group_member,
        "conv-leave": cmd_leave_group,
        "conv-dismiss": cmd_dismiss_group,
        "conv-update": cmd_update_group_info,
        "history": cmd_history,
        "ws-connect": cmd_ws_connect,
        "ws-send": cmd_ws_send,
        "ws-heartbeat": cmd_ws_heartbeat,
        "ws-typing": cmd_ws_typing,
        "ws-read-receipt": cmd_ws_read_receipt,
        "ws-ack": cmd_ws_ack,
        "bot-me": cmd_bot_me,
        "bot-conv-list": cmd_bot_conv_list,
        "bot-send": cmd_bot_send,
        "bot-webhook-get": cmd_bot_webhook_get,
        "bot-webhook-set": cmd_bot_webhook_set,
        "bot-webhook-delete": cmd_bot_webhook_delete,
        "bot-history": cmd_bot_conv_history,
        "bot-members": cmd_bot_conv_members,
        "bot-mark-read": cmd_bot_mark_read,
        "bot-read-states": cmd_bot_list_read_states,
        "bot-download-attachment": cmd_bot_download_attachment,
        "attachment-init": cmd_attachment_init,
        "attachment-get": cmd_attachment_get,
        "attachment-complete": cmd_attachment_complete,
        "attachment-download": cmd_attachment_download,
        "group-grant-admin": cmd_grant_group_admin,
        "group-revoke-admin": cmd_revoke_group_admin,
        "group-transfer-owner": cmd_transfer_group_owner,
        "interactive": cmd_interactive,
        "run-all": cmd_run_all,
    }

    handler = commands.get(args.command)
    if handler:
        handler(args)


if __name__ == "__main__":
    main()
