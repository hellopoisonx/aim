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

| frameType | 值 | 处理逻辑 |
|---|---|---|
| `PUSH_MESSAGE` | 101 | 解析 `conversation_id`/`sender_id`/`content`/`message_id`/`sent_at`/`client_msg_id`。跳过自己的消息（`senderId === currentUserId`）。按 `client_msg_id` 替换乐观消息（去重）。未知 `conversation_id` 时通过 `ensureConversationForPush` 创建会话条目。活跃会话自动发送 `SendReadReceipt`。发送 `SendAck(seq)`。 |
| `PUSH_PRESENCE` | 102 | 更新 `onlineUserIds` Set，更新对应会话的 `isOnline` 状态。 |
| `PUSH_NOTIFICATION` | 103 | 解析 `notification_type`/`title`/`body`，用 `ElMessage.info()` 显示通知。 |
| `PUSH_TYPING` | 104 | 设置 `typingInfo`，4 秒后自动清除。 |
| `RECONNECT` | 105 | 设置 `connectionState = 'connecting'`，延迟 `reconnect_delay_ms` 后调用 `ConnectWS()`。 |
| `SERVER_ACK` | 106 | 按 `client_msg_id` 找到对应消息，更新 `ackStatus`（`delivered`/`failed`）。发送 `SendAck(ackSeq)`。 |
| `TOKEN_EXPIRED` | 107 | 调用 `Refresh()` 尝试刷新 token；成功后调用 `ConnectWS()` 重建 WS；失败则调用 `handleLogout()`。 |
| `PUSH_FRIEND_APPLICATION` | 108 | 根据 `status` 显示不同 `ElMessage`（pending 用 info，accepted 用 success，rejected 用 warning）。 |

### 自动重连（Auto-reconnect）

连接断开后自动重连，避免用户手动刷新：

- **信号源**：`aim:connection_state` 事件中 `ws_connected === false` 时触发
- **主动断线标记**：`intentionalDisconnect` ref，Vue 主动退出登录时设为 `true`，跳过自动重连
- **最大重试次数**：`MAX_RECONNECT_ATTEMPTS = 5`
- **重试间隔**：指数退避（每次 `reconnectAttempt + 1` 秒），避免网络抖动时频繁请求
- **重置时机**：连接成功（`ws_connected === true`）时 `reconnectAttempt` 归零
- **清理**：`onUnmounted` 中 `clearReconnectTimer()` 取消 pending 重试

#### 流程图

```
WS 断开
  ├─ intentionalDisconnect === true → 不重连
  └─ reconnectAttempt < MAX_ATTEMPTS
       └─ setTimeout(ConnectWS, (attempt+1)*1000)
            ├─ 成功 → attempt = 0, startHeartbeat()
            └─ 失败 → attempt++ → 再次调度
```

### 已读回执自动发送

活跃会话（`activeConversationId === conversationId`）收到 `PUSH_MESSAGE` 时，自动调用 `SendReadReceipt(conversationId, msgId)`，将最后一条已读消息 ID 回传服务端。

## 历史消息游标分页

### `Conversation.historyCursor` 字段

```typescript
interface Conversation {
  // ...
  historyCursor?: {
    cursorCreatedAt: number
    cursorId: number
    hasMore: boolean
  }
}
```

- `loadConversationHistory()` 首次加载后将返回的游标信息存入 `conv.historyCursor`
- `MessageArea.vue` 监听滚动到顶部（`scrollTop <= 20`），触发 `load-more` emit
- `App.vue` 接收 `load-more` 事件，用已有 `historyCursor` 的值调用 `GetConversationHistory(convId, cursorCreatedAt, cursorId, limit)`
- 加载更多时不滚动到底部（`isLoadingMore` 标记控制）

### ACK 状态显示

消息气泡增加 ACK 状态图标：

| ackStatus | 图标 | 含义 |
|---|---|---|
| `pending` | ◌ | 发送中，等待服务端确认 |
| `delivered` | ✓ | 已送达 |
| `failed` | ✗ | 发送失败 |

仅自己的消息（`msg.isMine === true`）显示 ACK 状态。

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
- `onUnmounted` 中 `stopHeartbeat()` 确保组件销毁后定时器清除。

## ACK 链

```
客户端帧 (seq=N) → gateway → ServerAck(ack_seq=N, client_msg_id) → 客户端 SendAck(ack_seq)
```

- 客户端为每个发送帧递增 `writeSeq`
- 收到 `ServerAck` 后更新对应消息的 `ackStatus`
- 对服务端帧的 `seq` 发送 `ClientAck` 作为响应
