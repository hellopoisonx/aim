# 接口定义

## 消息写入（已迁出 core）

`core.proto` 不再定义 `TransferService`：WS / Bot OpenAPI 发消息由 gateway 直接调用 logic 的 `MessageService.IngestMessage`。core 只保留消费侧：

- 消费 `aim-message-transfer`（FIFO，Key/MessageGroup = `conversation_id`）
- 发布 `aim-gateway-push`（FIFO，同上），payload 见 `app/shared/events/push.go` 的 `PushEvent`

`app/core/rpc/pb` 当前只有文件描述符，没有服务；core 仍启动 zrpc server 以保持端口/etcd 注册稳定。

## PushEvent 契约

| 字段 | 说明 |
| --- | --- |
| `message_id` / `conversation_id` / `conversation_type` / `message_type` / `content` / `sent_at` | 消息本体 |
| `sender_id` / `sender_info` / `source_device_id` | 发送者快照与源设备（gateway 跳过源设备） |
| `target_user_id` | 单聊写扩散：本条事件唯一接收者，`member_ids` 为空 |
| `member_ids` | 群聊读扩散：会话成员快照，`target_user_id` 为 0 |
| `traceparent` / `tracestate` | 链路追踪透传 |

两个寻址字段互斥，gateway 对同时为空或同时非空的事件记日志后丢弃。

## GatewayService（调用方）

aim-core 的 Delivery Consumer 和 Presence Service 调用 `shared/proto/gateway/gateway.proto` 定义的 GatewayService：

Proto 定义：`shared/proto/gateway/gateway.proto`，生成的 pb 代码在 `shared/proto/gateway/pb/`。

| RPC | 用途 |
| --- | --- |
| `PushMessage(PushMessageReq) returns (PushMessageResp)` | 推送聊天消息到目标用户 |
| `PushPresence(PushPresenceReq) returns (PushPresenceResp)` | 推送用户在线状态变更 |
| `KickUser(KickUserReq) returns (KickUserResp)` | 踢出用户连接 |
| `DrainNotify(DrainNotifyReq) returns (DrainNotifyResp)` | 通知网关节点即将关闭 |

## PermissionService（调用方）

消息写入迁到 logic 后，core 不再调用 `PermissionService`；权限校验在 logic 进程内完成（`PermissionChecker` + Casbin）。

| RPC | 用途 |
| --- | --- |
| `CheckMessagePermission(CheckMessagePermissionReq) returns (CheckMessagePermissionResp)` | 校验发送者是否可以向会话发送消息，包括会话成员、好友/黑名单、群成员禁言等 logic 域规则 |
