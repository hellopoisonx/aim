# AIM WebSocket 帧协议

> 协议定义：[shared/proto/ws/ws.proto](../../shared/proto/ws/ws.proto)
> 网关实现：[app/gateway/api/internal/ws/](../../app/gateway/api/internal/ws/)

---

## 概述

AIM 网关与客户端之间通过 WebSocket 传输二进制 Protobuf 帧。所有帧均使用统一的 `WsFrame` 信封封装，通过 `type` 字段区分具体的负载类型。

### 传输格式

```protobuf
message WsFrame {
  FrameType type = 1;      // 帧类型
  int64     seq  = 2;      // 帧序列号
  bytes     payload = 3;   // 具体负载（Protobuf 序列化）
  int64     timestamp = 4; // 时间戳（毫秒）
}
```

### 序列号规则

- **客户端 seq**：单向递增，由客户端维护
- **服务端 seq**：单向递增，由网关维护
- **ACK 匹配**：`ServerAck.ack_seq` 匹配客户端 seq；`ClientAck.ack_seq` 匹配服务端 seq

### 连接流程

1. 客户端通过 `GET /ws` 携带 `Authorization: Bearer <access_token>` 发起 HTTP Upgrade
2. 网关验证 JWT，升级为 WebSocket 连接（二进制模式）
3. 双方通过 `WsFrame` 二进制帧通信
4. 客户端应定期发送 `HEARTBEAT` 保活（默认 20s 间隔，与 Redis 45s TTL 配合使用）
5. Token 过期时网关推送 `TOKEN_EXPIRED` 帧并关闭连接

---

## 帧类型枚举

### 客户端 → 网关（1–99）

| 枚举值 | 帧类型 | 描述 | 对应 Payload |
|--------|--------|------|--------------|
| 1 | `FRAME_TYPE_SEND_MESSAGE` | 发送聊天消息 | [`SendMessagePayload`](#sendmessagepayload) |
| 2 | `FRAME_TYPE_HEARTBEAT` | 心跳保活 | [`HeartbeatPayload`](#heartbeatpayload) |
| 3 | `FRAME_TYPE_TYPING` | 正在输入 | [`TypingPayload`](#typingpayload) |
| 4 | `FRAME_TYPE_READ_RECEIPT` | 已读回执 | [`ReadReceiptPayload`](#readreceiptpayload) |
| 5 | `FRAME_TYPE_ACK` | 客户端消息确认 | [`ClientAckPayload`](#clientackpayload) |

### 网关 → 客户端（101–199）

| 枚举值 | 帧类型 | 描述 | 对应 Payload |
|--------|--------|------|--------------|
| 101 | `FRAME_TYPE_PUSH_MESSAGE` | 推送聊天消息 | [`PushMessagePayload`](#pushmessagepayload) |
| 102 | `FRAME_TYPE_PUSH_PRESENCE` | 推送在线状态 | [`PushPresencePayload`](#pushpresencepayload) |
| 103 | `FRAME_TYPE_PUSH_NOTIFICATION` | 推送系统通知 | [`PushNotificationPayload`](#pushnotificationpayload) |
| 104 | `FRAME_TYPE_PUSH_TYPING` | 推送输入状态 | [`PushTypingPayload`](#pushtypingpayload) |
| 105 | `FRAME_TYPE_RECONNECT` | 网关要求重连（drain 窗口） | [`ReconnectPayload`](#reconnectpayload) |
| 106 | `FRAME_TYPE_SERVER_ACK` | 服务端确认 | [`ServerAckPayload`](#serverackpayload) |
| 107 | `FRAME_TYPE_TOKEN_EXPIRED` | Token 已过期 | [`TokenExpiredPayload`](#tokenexpiredpayload) |
| 108 | `FRAME_TYPE_PUSH_FRIEND_APPLICATION` | 推送好友申请 | [`PushFriendApplicationPayload`](#pushfriendapplicationpayload) |

---

## AckStatus 枚举

| 枚举值 | 名称 | 描述 |
|--------|------|------|
| 0 | `ACK_STATUS_UNSPECIFIED` | 未指定 |
| 1 | `ACK_STATUS_ACCEPTED` | 消息已被接受 |
| 2 | `ACK_STATUS_REJECTED` | 消息被拒绝（客户端不应重试） |
| 3 | `ACK_STATUS_RETRYABLE` | 可重试（服务端临时故障） |

---

## 消息体定义

### 客户端 → 网关

#### SendMessagePayload

发送聊天消息到指定会话。

```protobuf
message SendMessagePayload {
  int64  conversation_id = 1; // 会话 ID
  string message_type    = 2; // 消息类型（text/image/voice 等）
  string content         = 3; // 消息内容
  string client_msg_id   = 4; // 客户端消息 ID（用于去重）
  repeated string mentions = 5; // 被 @ 的用户 ID 列表
}
```

**方向**：客户端 → 网关

**服务端响应**：网关会返回 [`ServerAckPayload`](#serverackpayload) 帧，其中 `ack_seq` 对应此帧的 `seq`。

**ACK 状态映射**：

| 场景 | AckStatus | code |
|------|-----------|------|
| 发送成功 | `ACCEPTED` | 0 |
| 参数错误 / 鉴权失败 / 无权限 / 会话不存在 / 冲突 / 限流 | `REJECTED` | 对应业务码 |
| 服务端内部错误 | `RETRYABLE` | 50000 |

---

#### HeartbeatPayload

心跳保活，保持连接活跃并更新用户在线状态。

```protobuf
message HeartbeatPayload {
  int64 last_seq = 1; // 客户端已收到的最大服务端 seq
}
```

**方向**：客户端 → 网关

**服务端响应**：网关返回 [`ServerAckPayload`](#serverackpayload) 帧，不携带 message_id。

**副作用**：

- 续约 Redis `aim:presence:{user_id}` 和 `aim:user_gateway:{user_id}` 两个 Set 的 TTL（保持 alive）
- 在线状态事件仅在用户级 0→1（上线）或 1→0（下线）切换时发布到 Kafka（topic: `aim.presence.events`），心跳本身不触发事件

---

#### TypingPayload

通知服务端（及会话中其他用户）当前用户正在输入。

```protobuf
message TypingPayload {
  int64 conversation_id = 1; // 会话 ID
}
```

**方向**：客户端 → 网关

**服务端行为**：发布到 Kafka（topic: `aim.typing.events`），由 core 的 `TypingConsumer` 消费后向会话成员所在网关节点投递 [`PushTypingPayload`](#pushtypingpayload)。

---

#### ReadReceiptPayload

已读回执，标记会话中最后一条已读消息。

```protobuf
message ReadReceiptPayload {
  int64 conversation_id = 1; // 会话 ID
  int64 last_msg_id     = 2; // 最后一条已读消息的 message_id
}
```

**方向**：客户端 → 网关

---

#### ClientAckPayload

客户端确认已收到服务端的推送帧（可靠性保证）。

```protobuf
message ClientAckPayload {
  int64 ack_seq = 1; // 确认的服务端 seq
}
```

**方向**：客户端 → 网关

---

### 网关 → 客户端

#### PushMessagePayload

服务端推送聊天消息给在线客户端。

```protobuf
message PushMessagePayload {
  int64  message_id        = 1; // 消息全局唯一 ID
  int64  conversation_id   = 2; // 会话 ID
  string message_type      = 3; // 消息类型
  string content           = 4; // 消息内容
  int64  sender_id         = 5; // 发送者用户 ID
  int64  sent_at           = 6; // 发送时间戳（毫秒）
  string conversation_type = 7; // 会话类型（direct/group）
  string client_msg_id     = 8; // 客户端消息 ID（若有）
}
```

**方向**：网关 → 客户端

**触发条件**：会话中其他成员发送了消息，core 投递给当前用户的网关节点。

---

#### PushPresencePayload

推送用户在线状态变更。

```protobuf
message PushPresencePayload {
  int64  user_id    = 1; // 用户 ID
  string status     = 2; // 状态（online/offline）
  int64  updated_at = 3; // 状态变更时间戳（毫秒）
}
```

**方向**：网关 → 客户端

**触发条件**：好友上线或下线。

> **在线状态快照**：客户端在 WS 连接建立 / 重连成功后应调用 `GET /api/presence/friends` 获取当前好友的在线状态快照，避免依赖等待逐个 PushPresence 事件填充。

---

#### PushNotificationPayload

推送系统通知。

```protobuf
message PushNotificationPayload {
  string notification_type = 1; // 通知类型
  string title             = 2; // 通知标题
  string body              = 3; // 通知正文
  int64  related_id        = 4; // 关联对象 ID（如会话 ID）
}
```

**方向**：网关 → 客户端

---

#### PushTypingPayload

推送会话中其他用户的输入状态。

```protobuf
message PushTypingPayload {
  int64 user_id         = 1; // 输入中的用户 ID
  int64 conversation_id = 2; // 会话 ID
}
```

**方向**：网关 → 客户端

---

#### ReconnectPayload

网关要求客户端重连（drain 窗口期间发送，之后连接关闭）。

```protobuf
message ReconnectPayload {
  int64  reconnect_delay_ms = 1; // 建议重连延迟（毫秒）
  string gateway_node_id    = 2; // 当前网关节点 ID
}
```

**方向**：网关 → 客户端

**触发条件**：网关节点即将关闭（滚动更新 / 缩容），`DrainNotify` 内部 gRPC 触发。

---

#### ServerAckPayload

服务端确认收到客户端帧。可携带消息发送结果或对心跳的确认。

```protobuf
message ServerAckPayload {
  int64     ack_seq       = 1; // 确认的客户端 seq
  string    client_msg_id = 2; // 客户端消息 ID（若有）
  int32     code          = 3; // 业务状态码（0 表示成功）
  string    msg           = 4; // 状态描述
  AckStatus status        = 5; // 确认状态（见 AckStatus 枚举）
  int64     message_id    = 6; // 服务端分配的消息 ID（发送消息成功时返回）
}
```

**方向**：网关 → 客户端

**触发场景**：

- 响应 `SEND_MESSAGE`：返回发送结果，包含 message_id
- 响应 `HEARTBEAT`：返回确认，不携带 client_msg_id 和 message_id
- 响应 `TYPING` / `READ_RECEIPT` / `ACK`：返回确认

---

#### TokenExpiredPayload

网关通知客户端访问令牌已过期。

```protobuf
message TokenExpiredPayload {
  int64  expired_at = 1; // 过期时间（毫秒）
  string reason     = 2; // 过期原因
}
```

**方向**：网关 → 客户端

**触发条件**：JWT `access_token` 过期。发送此帧后网关会关闭连接。

---

#### PushFriendApplicationPayload

推送好友申请给目标用户。

```protobuf
message PushFriendApplicationPayload {
  int64  user_id    = 1; // 申请人用户 ID
  int64  friend_id  = 2; // 被申请人用户 ID
  string status     = 3; // 申请状态
  int64  created_at = 4; // 申请时间戳
  int64  updated_at = 5; // 更新时间戳
}
```

**方向**：网关 → 客户端

**触发条件**：其他用户通过 REST API `/api/users/friends/:id` 发起了好友申请。

---

## 封包 / 解包逻辑

参考代码：[app/gateway/api/internal/ws/frame.go](../../app/gateway/api/internal/ws/frame.go)

### 发送流程

```text
具体 Payload（如 SendMessagePayload）
       │
       ▼ protobuf.Marshal(payload)
二进制 payload bytes
       │
       ▼ 填入 WsFrame { type, seq, payload, timestamp }
WsFrame 结构体
       │
       ▼ protobuf.Marshal(frame)
二进制 frame bytes
       │
       ▼ conn.Write(websocket.MessageBinary, data)
WebSocket 二进制消息
```

### 接收流程

```text
WebSocket 二进制消息
       │
       ▼ conn.Read()
二进制 frame bytes
       │
       ▼ protobuf.Unmarshal → WsFrame
WsFrame 结构体
       │
       ▼ 根据 frame.type 分支
       ├── FRAME_TYPE_SEND_MESSAGE → protobuf.Unmarshal → SendMessagePayload
       ├── FRAME_TYPE_HEARTBEAT    → protobuf.Unmarshal → HeartbeatPayload
       ├── FRAME_TYPE_TYPING       → protobuf.Unmarshal → TypingPayload
       ├── FRAME_TYPE_READ_RECEIPT → protobuf.Unmarshal → ReadReceiptPayload
       ├── FRAME_TYPE_ACK          → protobuf.Unmarshal → ClientAckPayload
       ├── FRAME_TYPE_PUSH_MESSAGE → protobuf.Unmarshal → PushMessagePayload
       ├── FRAME_TYPE_PUSH_PRESENCE → protobuf.Unmarshal → PushPresencePayload
       ├── FRAME_TYPE_PUSH_NOTIFICATION → protobuf.Unmarshal → PushNotificationPayload
       ├── FRAME_TYPE_PUSH_TYPING  → protobuf.Unmarshal → PushTypingPayload
       ├── FRAME_TYPE_RECONNECT    → protobuf.Unmarshal → ReconnectPayload
       ├── FRAME_TYPE_TOKEN_EXPIRED → protobuf.Unmarshal → TokenExpiredPayload
       ├── FRAME_TYPE_SERVER_ACK   → protobuf.Unmarshal → ServerAckPayload
       └── FRAME_TYPE_PUSH_FRIEND_APPLICATION → protobuf.Unmarshal → PushFriendApplicationPayload
```

---

## 最佳实践

1. **心跳间隔**：建议每 20s 发送一次 `HEARTBEAT`（Vue `startHeartbeat()` 中 `20_000` ms），Redis TTL 45s（`Redis.PresenceTTL`）；心跳仅续约 TTL，不触发状态变更事件；`last_seq` 填入客户端收到的最大服务端 seq
2. **消息重试**：收到 `ServerAck` 中 `status = RETRYABLE` 时可以重试；`REJECTED` 时不应重试，应向用户展示错误
3. **断线重连**：收到 `RECONNECT` 帧时，客户端应在 `reconnect_delay_ms` 后重新发起连接
4. **Token 刷新**：收到 `TOKEN_EXPIRED` 时，客户端应通过 REST API `/api/auth/refresh` 获取新 Token，然后重新建立 WebSocket 连接
5. **序列号**：客户端 seq 从 1 开始单向递增，断线重连后重置
6. **去重**：`client_msg_id` 由客户端生成，服务端据此做幂等去重
