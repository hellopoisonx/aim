# WebSocket 帧处理

## 概述

桌面客户端与 gateway 之间的 WebSocket 通信使用 protobuf 二进制帧（`shared/proto/ws/ws.proto` → `WsFrame`）。Go 层和 Vue 层各有职责。

## 帧结构

```protobuf
message WsFrame {
  FrameType type      = 1;
  int64     seq       = 2;
  bytes     payload   = 3;
  int64     timestamp = 4; // Unix ms
}
```

## Go 层（wsclient.Client）

### `SendFrame`

`SendFrame` 负责编码并发送帧到 gateway。关键约束：

- **`Timestamp` 必须设置为 `time.Now().UnixMilli()`**，不能为 0
- `Seq` 由 `writeSeq` 原子递增生成
- `Payload` 为 protobuf 序列化的消息体

代码位置：`wsclient/wsclient.go` `func (c *Client) SendFrame(...)`：

```go
frame := &WsFrame{
    Type:      frameType,
    Seq:       seq,
    Timestamp: time.Now().UnixMilli(),
}
```

### `DecodeFramePayload`

从二进制数据解码 `WsFrame`，再根据 `frame.Type` 反序列化 `payload` 为具体 protobuf 类型。类型-载荷映射表见 `DecodePayload()` 的 `switch` 块。

### 事件桥接到 Vue

`OnFrame` 回调调用 `DecodeFramePayload` 后将 `FramePayload` 通过 `runtime.EventsEmit(ctx, "aim:frame_received", payload)` 发出。

## Vue 层（App.vue）

### 帧类型常量

```typescript
const WS_FRAME = {
  PUSH_MESSAGE: 101,
  PUSH_PRESENCE: 102,
  PUSH_NOTIFICATION: 103,
  PUSH_TYPING: 104,
  RECONNECT: 105,
  SERVER_ACK: 106,
  TOKEN_EXPIRED: 107,
  PUSH_FRIEND_APPLICATION: 108,
} as const
```

### `aim:frame_received` 事件分发

`EventsOn('aim:frame_received', ...)` 监听所有帧，按 `frameType` switch 分发：

| frameType | 处理逻辑 |
|---|---|
| **101** `PUSH_MESSAGE` | 解析 `conversation_id`/`sender_id`/`content`/`message_id`/`sent_at`/`client_msg_id`。跳过自己的消息（`senderId === currentUserId`）。按 `client_msg_id` 替换乐观消息（去重）。未知 `conversation_id` 时通过 `ensureConversationForPush` 创建会话条目。发送 `SendAck(seq)`。 |
| **102** `PUSH_PRESENCE` | 更新 `onlineUserIds` Set，更新对应会话的 `isOnline` 状态。 |
| **103** `PUSH_NOTIFICATION** | 解析 `notification_type`/`title`/`body`，用 `ElMessage.info()` 显示通知。 |
| **104** `PUSH_TYPING` | 设置 `typingInfo`，4 秒后自动清除。 |
| **105** `RECONNECT` | 设置 `connectionState = 'connecting'`，延迟 `reconnect_delay_ms` 后调用 `ConnectWS()`。 |
| **106** `SERVER_ACK` | 按 `client_msg_id` 找到对应消息，更新 `ackStatus`（`delivered`/`failed`）。发送 `SendAck(ackSeq)`。 |
| **107** `TOKEN_EXPIRED` | 调用 `Refresh()` 尝试刷新 token；失败则调用 `handleLogout()`。 |
| **108** `PUSH_FRIEND_APPLICATION` | 根据 `status` 显示不同 `ElMessage`（pending 用 info，accepted 用 success，rejected 用 warning）。 |

### `PUSH_NOTIFICATION`（103）处理细节

```typescript
case WS_FRAME.PUSH_NOTIFICATION: {
    const notificationType = payload?.notification_type as string
    const title = payload?.title as string
    const body = payload?.body as string
    const displayText = [title, body].filter(Boolean).join('：')
    if (displayText) ElMessage.info(displayText)
    break
}
```

- 不区分 `notification_type`，统一使用 `title：body` 格式
- `notification_type` 可能取值：`friend_request` / `group_invite` / `system_notice`
- `body` 为 JSONB 字符串，直接显示不解析

## 心跳机制

- 每 30 秒通过 `setInterval` 发送 `SendHeartbeat(lastReadSeq)`。
- `lastReadSeq` 在收到任何帧时更新为 `Math.max(seq, lastReadSeq)`。
- 心跳在 WS 连接断开时自动停止（`stopHeartbeat`），重连成功后重新启动（`startHeartbeat`）。

## ACK 链

```
客户端帧 (seq=N) → gateway → ServerAck(ack_seq=N, client_msg_id) → 客户端 SendAck(ack_seq)
```

- 客户端为每个发送帧递增 `writeSeq`
- 收到 `ServerAck` 后更新对应消息的 `ackStatus`
- 对服务端帧的 `seq` 发送 `ClientAck` 作为响应
