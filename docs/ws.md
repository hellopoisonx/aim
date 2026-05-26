# AIM Gateway WebSocket 帧协议

本文总结当前 gateway 暴露给客户端的 WebSocket 协议。REST API 见 [`docs/api/gateway-openapi.yaml`](api/gateway-openapi.yaml)，协议源文件见 [`shared/proto/ws/ws.proto`](../shared/proto/ws/ws.proto)。

## 1. 连接入口与鉴权

- URL：`GET /ws`
- 协议：WebSocket binary message，只传 Protobuf 二进制帧，不支持 JSON/text frame。
- 鉴权：握手请求必须携带 HTTP Header：

  ```http
  Authorization: Bearer <access_token>
  ```

- gateway 只接受 Header token，不接受 `?token=...`，避免 JWT 泄露到日志、浏览器历史或代理记录。
- 握手前鉴权失败时，不升级 WebSocket，直接返回 HTTP 401 JSON：

  ```json
  { "code": 40100, "msg": "unauthorized" }
  ```

- 连接身份来自 access token claims：`user_id` 与 `device_id`。同一 gateway 节点内同一个 `(user_id, device_id)` 只能存在一个活跃连接；重复注册会被拒绝并关闭新连接。
- 当前配置默认值（见 `app/gateway/api/internal/config/config.go` 与 `app/gateway/api/etc/gateway-api.yaml`）：
  - `WebSocket.MaxMsgSize`: `1024` bytes（服务端 `SetReadLimit` 使用该值）
  - 写帧超时：`5s`
  - 建议客户端心跳间隔：`20s~30s`；当前配置 `HeartbeatInterv=30`
  - Redis presence TTL：`45s`
  - 服务端 stale connection 扫描阈值：`60s`
  - token 过期重连宽限：`30s`

## 2. 通用帧格式

所有 WebSocket message 都是 `WsFrame` 的 Protobuf binary 编码。`payload` 字段再承载对应帧类型的 Payload Protobuf binary。

```protobuf
message WsFrame {
  FrameType type = 1;     // 帧类型
  int64 seq = 2;          // 序列号
  bytes payload = 3;      // 按 type 反序列化
  int64 timestamp = 4;    // Unix milliseconds
}
```

### 2.1 序列号规则

- 客户端发送帧时维护单调递增 `seq`。
- `FRAME_TYPE_SERVER_ACK` 的 `ServerAckPayload.ack_seq` 必须匹配被确认的客户端帧 `seq`。
- 客户端可通过 `FRAME_TYPE_ACK` 的 `ClientAckPayload.ack_seq` 确认收到服务端推送帧。
- 当前实现细节：
  - WebSocket handler 主动写出的 `SERVER_ACK` / `TOKEN_EXPIRED` 使用服务端本地单调 `seq`。
  - 通过内部 `GatewayService` gRPC 写出的推送帧当前 `seq=0`；客户端应能容忍 `seq=0`，可只对 `seq>0` 的服务端帧发送 `FRAME_TYPE_ACK`。
  - gateway 当前仅记录 `LastAckedSeq`，尚未实现基于 ACK 的离线补发。

## 3. 帧类型总览

### 3.1 客户端 → Gateway

| enum | 编号 | Payload | 是否返回 `SERVER_ACK` | 说明 |
|---|---:|---|---|---|
| `FRAME_TYPE_SEND_MESSAGE` | 1 | `SendMessagePayload` | 是 | 发送聊天消息，经 gateway 调 core `Transfer`。 |
| `FRAME_TYPE_HEARTBEAT` | 2 | `HeartbeatPayload` | 是 | 心跳保活，续约 presence TTL。 |
| `FRAME_TYPE_TYPING` | 3 | `TypingPayload` | 否 | 输入状态，best-effort 发布 Kafka 并本节点即时 fan-out。 |
| `FRAME_TYPE_READ_RECEIPT` | 4 | `ReadReceiptPayload` | 是 | 已读回执，持久化已读游标并 fan-out。 |
| `FRAME_TYPE_ACK` | 5 | `ClientAckPayload` | 否 | 客户端确认收到服务端推送。 |

### 3.2 Gateway → 客户端

| enum | 编号 | Payload | 说明 |
|---|---:|---|---|
| `FRAME_TYPE_PUSH_MESSAGE` | 101 | `PushMessagePayload` | 推送聊天消息。 |
| `FRAME_TYPE_PUSH_PRESENCE` | 102 | `PushPresencePayload` | 推送好友在线状态变更。 |
| `FRAME_TYPE_PUSH_NOTIFICATION` | 103 | `PushNotificationPayload` | 推送系统通知。 |
| `FRAME_TYPE_PUSH_TYPING` | 104 | `PushTypingPayload` | 推送输入状态。 |
| `FRAME_TYPE_RECONNECT` | 105 | `ReconnectPayload` | 节点 drain，要求客户端重连。 |
| `FRAME_TYPE_SERVER_ACK` | 106 | `ServerAckPayload` | 服务端确认客户端帧。 |
| `FRAME_TYPE_TOKEN_EXPIRED` | 107 | `TokenExpiredPayload` | access token 已过期，随后关闭连接。 |
| `FRAME_TYPE_PUSH_FRIEND_APPLICATION` | 108 | `PushFriendApplicationPayload` | 推送好友申请状态/通知。 |
| `FRAME_TYPE_PUSH_READ_RECEIPT` | 109 | `PushReadReceiptPayload` | 推送已读游标更新。 |

## 4. Payload 结构

### 4.1 客户端发送消息：`SendMessagePayload`

```protobuf
message SendMessagePayload {
  int64 conversation_id = 1;
  string message_type = 2;
  string content = 3;
  string client_msg_id = 4;
  repeated string mentions = 5;
}
```

| 字段 | 说明 |
|---|---|
| `conversation_id` | 目标会话 ID。 |
| `message_type` | 业务消息类型，如 `text` / `image` / `file` / `system` 或扩展类型。客户端普通发送通常不要使用 `system`。 |
| `content` | 消息内容。`text` 为纯文本；附件类通常为 JSON 字符串，建议遵循 `aim.attachment.v1` 内容 schema。 |
| `client_msg_id` | 客户端幂等 ID，建议 UUID；重试必须复用同一个值。 |
| `mentions` | 被 @ 用户 ID 的十进制字符串列表。 |

处理结果通过 `FRAME_TYPE_SERVER_ACK` 返回。

### 4.2 心跳：`HeartbeatPayload`

```protobuf
message HeartbeatPayload {
  int64 last_seq = 1;
}
```

- `last_seq` 表示客户端最新收到的服务端序列号。
- 服务端收到心跳后：
  1. 更新连接 `LastSeen`。
  2. 续约 Redis presence / user_gateway TTL。
  3. 返回 `SERVER_ACK`。
- 当前 `last_seq` 尚未用于自动补发。

### 4.3 输入状态：`TypingPayload`

```protobuf
message TypingPayload {
  int64 conversation_id = 1;
}
```

- gateway 收到后发布 `aim.typing.events`（key=`conversation_id`），并对本节点在线成员做即时 fan-out。
- core 的 TypingConsumer 会跨节点查询会话成员并推送 `PushTypingPayload`。
- 当前该帧不返回 `SERVER_ACK`。
- 客户端建议：输入中事件按约 `2.5s` 节流发送；收到 `PUSH_TYPING` 后约 `4s` 未再次收到则清除 UI 状态。

### 4.4 已读回执：`ReadReceiptPayload`

```protobuf
message ReadReceiptPayload {
  int64 conversation_id = 1;
  int64 last_msg_id = 2;
}
```

- `last_msg_id` 表示当前用户在该会话内已读到的最大消息 ID。
- gateway 收到后：
  1. 调 logic `UpdateReadReceipt` upsert 已读游标。
  2. 发布 Kafka `aim.read_receipt.events`（key=`conversation_id`）。
  3. 对本节点其他在线会话成员即时 fan-out `PUSH_READ_RECEIPT`。
  4. 返回 `SERVER_ACK`。
- 成功 ACK 当前使用基础 `ServerAckPayload`，`code=0`，`status=ACK_STATUS_UNSPECIFIED`，客户端应按 `code=0` 视为成功。

### 4.5 客户端确认：`ClientAckPayload`

```protobuf
message ClientAckPayload {
  int64 ack_seq = 1;
}
```

- `ack_seq` 为客户端确认收到的服务端帧 `seq`。
- gateway 当前只记录每个连接的最大 `LastAckedSeq`，不返回 ACK。
- 由于当前内部 gRPC 推送帧可能 `seq=0`，客户端可只对 `seq>0` 的服务端帧发送确认。

### 4.6 推送消息：`PushMessagePayload`

```protobuf
message SenderInfo {
  string name = 1;
  string email = 2;
  string display_name = 3;
}

message PushMessagePayload {
  int64 message_id = 1;
  int64 conversation_id = 2;
  string message_type = 3;
  string content = 4;
  int64 sender_id = 5;
  int64 sent_at = 6;
  string conversation_type = 7;
  string client_msg_id = 8;
  bool is_system = 9;
  SenderInfo sender_info = 10;
  repeated string mentions = 11;
}
```

| 字段 | 说明 |
|---|---|
| `message_id` | 服务端消息 ID，全局唯一，用于幂等与已读回执。 |
| `conversation_id` | 会话 ID。 |
| `message_type` | 消息类型：`text` / `image` / `file` / `system` / 扩展类型。 |
| `content` | 消息内容，附件/系统消息通常为 JSON 字符串。 |
| `sender_id` | 发送者用户 ID；系统消息为 `0`。 |
| `sent_at` | 发送时间，Unix milliseconds。 |
| `conversation_type` | `direct` / `group`；容错情况下可能为空字符串，客户端需兼容。 |
| `client_msg_id` | 发送者客户端消息 ID；发送方可用它与本地 pending 消息匹配。 |
| `is_system` | `true` 表示群变更系统消息。 |
| `sender_info` | 发送者名称、邮箱、显示名快照。 |
| `mentions` | 被 @ 用户 ID 的十进制字符串列表。 |

群变更系统消息通常满足：`sender_id=0`、`message_type="system"`、`is_system=true`，常见事件包括 `member_joined`、`member_left`、`member_removed`、`group_renamed`、`group_dismissed`、`group_avatar_changed`。

### 4.7 在线状态推送：`PushPresencePayload`

```protobuf
message PushPresencePayload {
  int64 user_id = 1;
  string status = 2;
  int64 updated_at = 3;
}
```

- `user_id` 是状态发生变化的用户，不是接收者。
- `status`: `online` / `offline`。
- `updated_at`: Unix milliseconds。
- 初始快照通过 REST `GET /api/presence/friends` 获取；实时变化通过该 WS 帧更新。

### 4.8 系统通知：`PushNotificationPayload`

```protobuf
message PushNotificationPayload {
  string notification_type = 1;
  string title = 2;
  string body = 3;
  int64 related_id = 4;
}
```

- `notification_type`: 业务自定义，如 `announcement` / `maintenance` / `force_update` 等。
- `body`: 字符串；需要结构化内容时可放 JSON 字符串。
- `related_id`: 业务关联 ID，可为 `0`。

### 4.9 输入状态推送：`PushTypingPayload`

```protobuf
message PushTypingPayload {
  int64 user_id = 1;
  int64 conversation_id = 2;
}
```

- `user_id` 是正在输入的用户。
- 客户端应按会话隔离输入状态，并避免向发送者自己展示。

### 4.10 好友申请推送：`PushFriendApplicationPayload`

```protobuf
message PushFriendApplicationPayload {
  int64 user_id = 1;
  int64 friend_id = 2;
  string status = 3;
  int64 created_at = 4;
  int64 updated_at = 5;
}
```

- 当前 gateway proto 中该 payload 表达好友关系/申请状态快照。
- `status` 取值由 logic 侧好友关系状态决定，例如 `pending` / `accepted` / `rejected`。
- 收到后客户端通常应刷新好友申请列表或好友列表。

### 4.11 已读回执推送：`PushReadReceiptPayload`

```protobuf
message PushReadReceiptPayload {
  int64 conversation_id = 1;
  int64 user_id = 2;
  int64 last_read_message_id = 3;
  int64 updated_at = 4;
}
```

- `user_id` 是推进已读游标的成员。
- `last_read_message_id` 是该成员已读到的最大消息 ID。
- `updated_at` 为 Unix milliseconds。
- 客户端也可通过 REST 历史接口响应中的 `read_states` / `read_details` 补齐快照。

### 4.12 Drain 重连：`ReconnectPayload`

```protobuf
message ReconnectPayload {
  int64 reconnect_delay_ms = 1;
  string gateway_node_id = 2;
}
```

- gateway 节点计划下线时先广播该帧。
- 客户端应在 `reconnect_delay_ms` 后主动重连；服务端会在 drain 超时后关闭旧连接。
- `gateway_node_id` 为空时由负载均衡决定新节点。

### 4.13 Token 过期：`TokenExpiredPayload`

```protobuf
message TokenExpiredPayload {
  int64 expired_at = 1;
  string reason = 2;
}
```

- `expired_at`: Unix milliseconds。
- 当前 `reason`: `access_token_expired`。
- 服务端发送该帧后会以 WebSocket close code `1008`（Policy Violation）关闭连接。
- 客户端应调用 REST `POST /api/auth/refresh` 获取新 token，然后携带新 Bearer token 重新连接 `/ws`。
- gateway 对 token 过期断开有约 `30s` reconnect grace；宽限期内重连成功不会立即发布 offline presence。

### 4.14 服务端 ACK：`ServerAckPayload`

```protobuf
enum AckStatus {
  ACK_STATUS_UNSPECIFIED = 0;
  ACK_STATUS_ACCEPTED = 1;
  ACK_STATUS_REJECTED = 2;
  ACK_STATUS_RETRYABLE = 3;
}

message ServerAckPayload {
  int64 ack_seq = 1;
  string client_msg_id = 2;
  int32 code = 3;
  string msg = 4;
  AckStatus status = 5;
  int64 message_id = 6;
}
```

| 字段 | 说明 |
|---|---|
| `ack_seq` | 被确认的客户端帧 `seq`。 |
| `client_msg_id` | 对消息发送 ACK 时回传 `SendMessagePayload.client_msg_id`；心跳/已读成功 ACK 可为空。 |
| `code` | `0` 表示成功；否则为 AIM 业务错误码。 |
| `msg` | 错误描述。 |
| `status` | `ACCEPTED` / `REJECTED` / `RETRYABLE`；心跳和已读成功 ACK 当前可能为 `UNSPECIFIED` 且 `code=0`。 |
| `message_id` | 消息发送被接受时的服务端消息 ID；非消息 ACK 为 `0`。 |

## 5. ACK 映射与重试策略

`FRAME_TYPE_SEND_MESSAGE` 调用 core `Transfer` 后映射为 `SERVER_ACK`：

| core / gateway 结果 | `status` | `code` | 客户端建议 |
|---|---|---:|---|
| 发送成功 | `ACK_STATUS_ACCEPTED` | `0` | 标记消息已发送，记录 `message_id`。 |
| 重复 `client_msg_id` 且已有消息 | `ACK_STATUS_ACCEPTED` | `0` | 使用返回的已有 `message_id`，不要重复生成新消息。 |
| 参数错误 | `ACK_STATUS_REJECTED` | `40000` | 展示错误，不自动重试。 |
| token/身份无效 | `ACK_STATUS_REJECTED` | `40100` | 刷新 token 或重新登录后重连。 |
| 无权限、被禁言、非会话成员、拉黑等 | `ACK_STATUS_REJECTED` | `40300` | 展示业务错误，不自动重试。 |
| 会话不存在 | `ACK_STATUS_REJECTED` | `40400` | 刷新会话列表或提示不存在。 |
| 业务冲突 | `ACK_STATUS_REJECTED` | `40900` | 展示业务错误，不自动重试。 |
| 触发限流 | `ACK_STATUS_REJECTED` | `42900` | 按策略退避后再尝试。 |
| core 不可用、deadline、Kafka 瞬时故障、未知基础设施错误 | `ACK_STATUS_RETRYABLE` | `50000` | 保留本地 pending 消息，用相同 `client_msg_id` 重试。 |

`FRAME_TYPE_READ_RECEIPT` 错误 ACK 也使用类似策略：参数/认证/权限/不存在/冲突/限流为 `REJECTED`，内部错误为 `RETRYABLE`。

## 6. 推荐客户端流程

### 6.1 建连

1. 调 REST `POST /api/auth/login` 或 `POST /api/auth/refresh` 获取 access token。
2. 发起 `GET /ws`，Header 带 `Authorization: Bearer <access_token>`。
3. 连接成功后，可调用 REST `GET /api/presence/friends` 获取好友在线状态初始快照。
4. 启动心跳定时器（建议 `20s~30s`）。

### 6.2 发送消息

1. 生成稳定的 `client_msg_id`（建议 UUID）。
2. 编码 `SendMessagePayload`，再编码 `WsFrame{type=SEND_MESSAGE, seq=clientSeq, payload=...}`。
3. 发送 binary WebSocket message。
4. 等待 `SERVER_ACK`：
   - `status=ACCEPTED && code=0`：本地 pending → sent，保存 `message_id`。
   - `status=REJECTED`：本地 pending → failed，提示用户，不自动重试。
   - `status=RETRYABLE`：保留 pending，用相同 `client_msg_id` 退避重试。

### 6.3 接收消息

1. 收到 `PUSH_MESSAGE` 后按 `conversation_id` 入对应会话。
2. 如果 `client_msg_id` 命中本地 pending 消息，可将本地临时消息与服务端消息合并。
3. 如果 `is_system=true` 或 `sender_id=0 && message_type="system"`，按系统消息样式渲染。
4. 用户打开/滚动到消息后，发送 `READ_RECEIPT` 推进已读游标。

### 6.4 Token 续期

1. 收到 `TOKEN_EXPIRED` 或 WebSocket close code `1008` 且 reason 为 token 相关时，停止旧连接读写。
2. 调 REST `POST /api/auth/refresh`。
3. 使用新 access token 重新连接 `/ws`。
4. 重连成功后刷新 presence 快照、会话列表或必要的历史分页。

### 6.5 节点 drain

1. 收到 `RECONNECT` 后，等待 `reconnect_delay_ms` 或立即启动新连接（按客户端策略）。
2. 新连接成功后关闭旧连接。
3. 若旧连接被服务端关闭，按普通断线重连流程处理。

## 7. 编码注意事项

- 必须使用 Protobuf binary：先序列化具体 payload，再填入 `WsFrame.payload`，最后序列化 `WsFrame`。
- 不要发送 JSON 字符串或文本帧。
- 客户端必须容忍新增未知字段；proto3 未知字段应忽略。
- 新增帧类型时只能追加编号，不复用历史编号。
- `mentions` 使用十进制用户 ID 字符串，避免跨语言精度问题。
- `conversation_type` 合法值为 `direct` / `group`；客户端需容忍少数 fallback 推送为空字符串。
- 当前服务端对 typing / notification 等瞬时事件采用 best-effort 投递，客户端不应依赖其强一致到达。
