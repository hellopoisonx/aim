# 协议规则

## 概览

`shared/proto` 定义 AIM 的跨端/跨服务线缆协议。这里不是业务实现目录，只放 `.proto` 和生成的 Go 代码；生成文件不要手改。

## 结构

```text
shared/proto/
├── ws/ws.proto              # WebSocket 二进制帧协议，gateway 与客户端共用
└── gateway/gateway.proto    # Core 调 GatewayService 的推送/踢下线/drain RPC
```

## 协议规则

### 字段约定

- `PushPresenceReq` 字段号：
  - `1` = `user_id`（状态变更者）
  - `2` = `status`（online/offline）
  - `3` = `updated_at`
  - `4` = `target_user_id`（接收推送的目标用户；未设置时网关回退用 `user_id` 寻址）
- `PushTypingReq` 字段号：
  - `1` = `target_user_id`
  - `2` = `from_user_id`
  - `3` = `conversation_id`
  - `4` = `timestamp`
- `PushReadReceiptReq` 字段号：
  - `1` = `target_user_id`
  - `2` = `conversation_id`
  - `3` = `from_user_id`（推进游标的成员）
  - `4` = `last_read_message_id`
  - `5` = `updated_at`（毫秒）
- `ws.FrameType` 枚举：客户端→网关使用 `FRAME_TYPE_READ_RECEIPT (4)`；网关→客户端使用 `FRAME_TYPE_PUSH_READ_RECEIPT (109)`。`ReadReceiptPayload` 字段为 `conversation_id`、`last_msg_id`；`PushReadReceiptPayload` 字段为 `conversation_id`、`user_id`、`last_read_message_id`、`updated_at`。

### 向后兼容规则

- 服务端 `PushPresence` 收到 `target_user_id == 0` 时，自动回退到 `user_id` 寻址连接，保证旧版 caller（未设置 `target_user_id` 的 core 部署）行为不变。

### 已读回执链路

- 入站：客户端发送 `FRAME_TYPE_READ_RECEIPT`（带 `conversation_id / last_msg_id`），网关 handler 在 `app/gateway/api/internal/handler/ws/ws_handler.go::handleReadReceipt` 调用 `logic.ConversationService.UpdateReadReceipt` upsert 游标，并将 `readReceiptEvent` 推到 Kafka 主题 `aim.read_receipt.events`（key=`conversation_id`）。
- 出站：core 的 `ReadReceiptConsumer` 消费该主题，按 `GetConversationMembers` 拿到会话成员后，向除发送者外的成员所在网关节点调用 `gateway.GatewayService.PushReadReceipt`；网关收到后通过 `FRAME_TYPE_PUSH_READ_RECEIPT` 推给目标用户的所有连接。
- 历史接口：`ConversationService.GetConversationHistory` 在返回消息列表的同时附带 `repeated ReadStateItem read_states`，客户端可据此渲染“谁已读到哪条”。

### @ 提及（mentions）

- 发送：`SendMessagePayload.mentions`（字段号 5）为十进制用户 ID 字符串列表；core `Transfer` 转 `int64` 后做权限校验并写入 `messages.mentions` JSONB。
- 历史：`logic.MessageItem.mentions`（字段号 9）、`gateway.api MessageItem.mentions` 返回同格式字符串列表。
- 推送：`PushMessagePayload.mentions`（字段号 11）、`gateway.PushMessageReq.mentions`（字段号 9）透传；Desktop 用 `mentions` 渲染「提及」行，正文中的 `@昵称` 由客户端在发送时写入。

### 群变更系统消息

- `PushMessagePayload.is_system`（字段号 9）：`true` 表示群变更系统消息（member_joined, member_left, member_removed, group_renamed, group_dismissed, group_avatar_changed）。前端据此区分展示。
- `PushMessageReq.is_system`（字段号 11）：core `ConversationEventConsumer` 向 gateway 推送群变更事件时设为 `true`。
- `sender_id=0`、`message_type="system"` 标识系统消息，与普通用户消息区分。

### conversation_type 规约

- 全栈合法值：`direct`（双人会话）、`group`（群聊），与 `logic.conversations.conversation_type` 列对齐。
- `gateway.proto` 与 `ws.proto` 历史注释中的 `single/group` 已废弃，应只写 `direct/group`。
- 透传链路：`logic.GetConversationMembersResp.conversation_type` -> core delivery / conversation_event consumer -> `gateway.PushMessageReq.conversation_type` -> `ws.PushMessagePayload.conversation_type`。
- core 在 logic 不可达时（fallback 到仅推送给发送者）允许 `conversation_type` 为空字符串；客户端必须能容忍空值。

### 系统通知推送

- gRPC：`gateway.GatewayService.PushNotification(PushNotificationReq) returns (PushNotificationResp)`。
- 字段号：`1` = `target_user_id`（0 表示广播本节点所有连接）、`2` = `notification_type`、`3` = `title`、`4` = `body`、`5` = `related_id`。
- WS：`FRAME_TYPE_PUSH_NOTIFICATION (103)` + `PushNotificationPayload`。
- 调用方：业务（公告 / 维护 / 强制升级）通过 core `GatewayPusher.PushNotification`（或 `GatewayRouter.PushNotificationToNode`）调用网关；网关查找目标用户连接并写帧。

### 速率限流

- core `Transfer` 在权限检查前调用 Redis 滑动窗口限流（`aim:transfer:quota:{sender_id}`）。
- 配置：`TransferQuotaConf{ WindowSeconds, MaxRequests }`。`MaxRequests <= 0` 关闭限流。
- 命中限流时返回 `errorx.CodeRateLimit (42900)` -> gRPC `ResourceExhausted` -> ws `ServerAckPayload` `ACK_STATUS_REJECTED`。
- Redis 故障时 fail-open（仅记录日志），保证消息可用性优先。

## 一般规则

- WebSocket 只使用 Protobuf binary frame；不要新增 JSON/text frame 协议。
- `WsFrame.seq` 用于请求/ACK 关联；客户端发消息必须带稳定 `client_msg_id` 以支持重试和幂等。
- `FrameType` 编号保持区间语义：客户端到网关使用低编号，网关到客户端推送/ACK 使用高编号；新增值只追加，不复用旧编号。
- Proto 字段号一旦发布不得改含义；废弃字段保留编号和注释，不删除后复用。
- `gateway.proto` 的 `GatewayService` 由 gateway 实现，core 调用；不要让 logic 反向依赖 core/gateway 内部包。

## 生成

```bash
# WS/Gateway 共享 proto：在仓库根执行
protoc --go_out=. shared/proto/ws/ws.proto
protoc --go_out=. --go-grpc_out=. shared/proto/gateway/gateway.proto
```

若生成路径出现 `shared/proto/gateway` 与 `shared/proto/gateway/pb` 双份输出，先检查 `go_package`，不要手工改生成文件内容来修。

## 修改检查

```bash
go test ./app/gateway/api/internal/ws/... ./app/core/...
go build ./...
```