# AIM Bot OpenAPI 开发者指南（V0）

本指南面向第三方开发者，介绍如何接入 AIM 群聊 Bot：通过 HTTP REST 接口
发送消息、接收 Webhook 回调。

> 本文档对应 V0 接口面，规范在 [`docs/api/gateway-openapi.yaml`](api/gateway-openapi.yaml)。
> 服务端实现见仓库内的 `aim-bot-domain` Skill。

## 1. 概览

```
┌──────────────┐  Bot Token   ┌─────────────────┐  gRPC   ┌─────────┐
│ 第三方 Bot   │ ───────────▶ │ AIM Gateway     │ ──────▶ │ AIM Core│
│  (HTTP 服务) │              │ /api/bot/v1/*   │         └─────────┘
│              │ ◀─────────── │ Webhook 回调    │
└──────────────┘   HMAC 签名  └─────────────────┘
```

- 一个 Bot 在 AIM 中等价于一个特殊的「用户」(`user_type=bot`)，拥有自己的
  `user_id`、昵称、头像，可作为成员加入若干群聊。
- 群中其他人发的消息通过 Webhook 推给 Bot 的 HTTP 服务；
  Bot 发的消息通过 REST 调用回 AIM。

## 2. 鉴权

所有 `/api/bot/v1/*` 路由都需要带 token：

```
Authorization: Bot aim_bot_<64位hex>
```

注意：

- 方案名 `Bot` 与 `Bearer` 区分；后者用于人类用户的 JWT。
- Token 在配置时由 AIM 运维生成，**仅展示一次**，AIM 服务端只保存
  哈希。丢失只能由运维撤销重发。
- Token 携带 scope（如 `messages:send`）。当前 V0 调用 `/messages` 端点
  时校验 `messages:send`。

错误码（biz code → HTTP）：

| code | HTTP | 含义 |
|------|------|------|
| 40110 | 401 | token 缺失/格式错误/未知 |
| 40111 | 401 | token 已撤销或过期 |
| 40112 | 401 | bot 已被禁用 |
| 40310 | 403 | token 缺少所需 scope |
| 40010 | 400 | 请求体校验失败（如 webhook url 非法） |
| 42900 | 429 | 触发限流 |

响应封装统一为 `{ "code": 0, "msg": "ok", "body": {...} }`，错误时 `code != 0`。

## 3. 接口

服务地址（本地）：`http://127.0.0.1:8888`。

### 3.1 GET `/api/bot/v1/me`

返回当前 token 关联的 Bot 资料。

```bash
curl -H "Authorization: Bot $TOKEN" \
     http://127.0.0.1:8888/api/bot/v1/me
```

```json
{
  "code": 0,
  "msg": "ok",
  "body": {
    "bot": {
      "bot_user_id": 9000000001,
      "nickname": "broadcast-bot",
      "avatar": "https://implement.me",
      "status": 1,
      "scopes": ["messages:send"]
    }
  }
}
```

### 3.2 GET `/api/bot/v1/conversations`

列出 Bot 当前所在的全部会话（含群和直接对话）。

### 3.3 POST `/api/bot/v1/messages`

向群发送消息。

```bash
curl -X POST \
  -H "Authorization: Bot $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "conversation_id": 1,
        "message_type": "text",
        "content": "hello from bot",
        "client_msg_id": "550e8400-e29b-41d4-a716-446655440000"
      }' \
  http://127.0.0.1:8888/api/bot/v1/messages
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `conversation_id` | 是 | 目标会话 ID，Bot 必须是该会话成员 |
| `message_type` | 是 | 业务消息类型，例如 `text`、`image`，最多 32 字符 |
| `content` | 是 | 业务消息载荷，由你和你的客户端协商；服务端只透传 |
| `client_msg_id` | 是 | **幂等键**：相同 `(bot_user_id, "bot-api", client_msg_id)` 重复调用返回同一 `message_id` |
| `mentions` | 否 | 被 @ 的用户 ID，最多 20 个 |

成功响应：

```json
{
  "body": {
    "message_id": 123456789,
    "client_msg_id": "550e8400-...",
    "accepted_at": 1700000000000
  }
}
```

服务端会按 `conversation_id` 路由到 Kafka，群内其他成员通过 WebSocket
立即收到 `PUSH_MESSAGE` 帧。

### 3.4 GET `/api/bot/v1/webhook`

返回当前 webhook 配置（**不含** plaintext secret）。

```json
{
  "body": {
    "configured": true,
    "webhook": {
      "url": "https://yourbot.example.com/aim/webhook",
      "events": ["message.created"],
      "enabled": true,
      "updated_at": 1700000000000
    }
  }
}
```

### 3.5 PUT `/api/bot/v1/webhook`

新建或更新 webhook 配置。

```json
{
  "url": "https://yourbot.example.com/aim/webhook",
  "events": ["message.created"],
  "enabled": true,
  "rotate_secret": true
}
```

字段：

| 字段 | 说明 |
|------|------|
| `url` | 必填，HTTPS 地址 |
| `events` | 可选，订阅的事件名数组；省略默认 `["message.created"]` |
| `enabled` | 可选，默认 `true` |
| `secret` | 提供你已有的签名 secret；与 `rotate_secret` 二选一 |
| `rotate_secret` | `true` 时服务端生成新 secret 并**仅在响应里返回一次** |

返回：

```json
{
  "body": {
    "webhook": {
      "url": "...",
      "events": ["message.created"],
      "enabled": true,
      "updated_at": 1700000000000
    },
    "plaintext_secret": "9d4a..."   // 仅在 rotate_secret=true 时出现
  }
}
```

注意：服务端只保存 secret 的 SHA-256 哈希；丢失明文只能再次 rotate。

### 3.6 DELETE `/api/bot/v1/webhook`

删除 webhook 配置（之后将不再有回调）。

## 4. 接收 Webhook 回调

### 4.1 请求格式

服务端在群消息触发后发起 HTTP POST，body 是 JSON：

```json
{
  "event_id": "9d4a3f5c1c1a4f5fbb7e6d3c4f8a1b2d",
  "type": "message.created",
  "created_at": 1700000000123,
  "conversation_id": 1,
  "message": {
    "message_id": 123456789,
    "sender_id": 1001,
    "sender_type": "human",
    "message_type": "text",
    "content": "hi bot",
    "client_msg_id": "abc-123",
    "timestamp": 1700000000000
  }
}
```

请求头：

| Header | 值 |
|--------|----|
| `Content-Type` | `application/json` |
| `User-Agent` | `AIM-Bot-Webhook/1.0` |
| `X-AIM-Event-Id` | 32 位 hex，可用作幂等键 |
| `X-AIM-Event-Type` | `message.created`（V0 唯一类型） |
| `X-AIM-Signature` | `sha256=<hex>`，详见下文 |

服务端期望 2xx 响应；非 2xx 会按指数退避重试最多 5 次（间隔 1s/2s/4s/8s/16s）。

### 4.2 校验签名

签名 = HMAC-SHA256(secret_hash, raw_body)，hex 编码。注意 V0 用的是
**stored secret_hash**，因为这是服务端唯一保存的值；如果你需要直接用
plaintext secret 验签，未来版本会通过 X-AIM-Signature 的算法前缀升级
（仍然向后兼容）。

Python 示例（V0）：

```python
import hmac, hashlib

def verify(stored_secret_hash: str, raw_body: bytes, signature_header: str) -> bool:
    if not signature_header.startswith("sha256="):
        return False
    expected = signature_header[len("sha256="):]
    digest = hmac.new(stored_secret_hash.encode(), raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(digest, expected)
```

### 4.3 防回调循环

- AIM 在分发前会过滤 `sender_id == bot_user_id` 的事件；Bot 自己发的消息
  **不会**触发回调，无需在 Bot 端再去重。
- 即便如此，建议你在 Bot 服务里仍以 `event_id` 作幂等键。

## 5. 限流

V0 阶段限流复用 core 的滑动窗口配额，按 `(sender_id, device_id)` 计数；
Bot 的 `device_id` 固定为 `bot-api`，因此与人类用户的限额隔离。

超额时 `/api/bot/v1/messages` 返回：

```json
{ "code": 42900, "msg": "rate limit" }
```

请实现指数退避，不要在收到 429 后立即重试。

## 6. 端到端联调

仓库内置 `dev-tool/aim_test.py` 提供命令行调试入口（已加入 Bot 子命令）：

```bash
export AIM_BOT_TOKEN=aim_bot_xxxxxxxxxxxxxxxxxxxxxxxx

python aim_test.py bot-me
python aim_test.py bot-conv-list
python aim_test.py bot-send --conversation-id 1 --content "hello from bot"
python aim_test.py bot-webhook-set --url https://your.tunnel.example/aim --rotate-secret
python aim_test.py bot-webhook-get
```

如需让一个本地 HTTP 服务接收回调，配合 `ngrok http 9000` 这类隧道工具
将 `--url` 暴露给 docker-compose 中的 `aim-logic`。

## 7. 参考实现注意点

- 始终带上 `client_msg_id` —— 你的客户端崩溃 / 网络重试都不会重复发消息。
- Webhook 处理建议异步：把 `event_id` 入队后立即返回 200；否则容易触发
  服务端的指数退避重试。
- 为每个 Bot 单独维护一个 secret，避免共用导致泄露面扩大。
