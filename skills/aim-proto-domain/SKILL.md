---
name: aim-proto-domain
description: aim 的 Protobuf 协议域。定义跨端/跨服务线缆协议。对应 `shared/proto` 目录。
---
# aim-proto-domain

## 如何使用

- 涉及协议规则与帧编号约定 -> `references/protocol-rules.md`
- 涉及 proto 代码生成 -> `references/protocol-rules.md`（生成命令在文末）

## 参考资料

- `references/protocol-rules.md`

## 最近变更


- 2026-06-02: 限流从 core 上提到 gateway。core 删除 `TransferQuotaConf` 与 `TransferLogic.checkQuota`;gateway 新增 `app/gateway/api/internal/middleware.RateLimitMiddleware` 与 `BotRateLimitMiddleware`,共享 `app/shared/quota.QuotaStore`,命名空间 `aim:gateway:quota` (人类 / key = `<deviceID>:<userID>`) 与 `aim:gateway:quota:bot` (Bot / 单维 `tokenID`)。WebSocket `handleSendMessage` 在调 `core.Transfer` 前手动限流,命中时通过 `ServerAckPayload` 返回 `ACK_STATUS_REJECTED + CodeRateLimit`。详见 `references/protocol-rules.md` §速率限流。

- 2026-05-24: 协议层 name 快照统一。`logic.proto` 新增字段：`ConversationResponse.display_name`(9)、`MemberDetailItem.display_name`(6)、`SenderInfo.display_name`(3)、`MessageReadDetailItem.display_name`(8)、`ReadStateItem.email`(4)/`avatar`(5)/`display_name`(6)、`MessageItem.is_system`(11)。`ws.proto` `SenderInfo` 新增 `display_name`(3)。`gateway.proto` `SenderInfo` 新增 `display_name`(3)。`GetConversationMembersDetail` SQL 新增 `ui.nickname AS display_name`。
- 2026-05-23: @ 提及返回链路：`logic.MessageItem.mentions`（9）、`ws.PushMessagePayload.mentions`（11）；历史与 WS 推送均返回十进制用户 ID 字符串列表。
- 2026-05-23: 后端 proto 实现差距修复。`logic.proto` `GetConversationMembersResp` 新增 `conversation_type`（字段号 3），core delivery/conversation_event consumer 不再硬编码 `"direct"`；`gateway.proto` 新增 `PushNotification(PushNotificationReq) returns (PushNotificationResp)`，对接 ws `FRAME_TYPE_PUSH_NOTIFICATION` 生产链路；`gateway.proto` `PushMessageReq.conversation_type` 注释统一为 `direct/group`。core Transfer 现把 mentions 解析为 int64 后传入 `CheckMessagePermissionReq`，并新增 Redis 滑动窗口限流（`TransferQuotaConf`）。
- 2026-05-23: 已读回执后端链路落地。`ws.proto` 新增 `FRAME_TYPE_PUSH_READ_RECEIPT (109)` 与 `PushReadReceiptPayload`；`gateway.proto` 新增 `PushReadReceipt(PushReadReceiptReq) returns (PushReadReceiptResp)`；`logic.proto` `ConversationService` 新增 `UpdateReadReceipt` / `ListConversationReadStates` 两个 RPC，`GetConversationHistoryResp` 追加 `repeated ReadStateItem read_states`。详见 `references/protocol-rules.md` §已读回执链路。
- 2026-05-22: `ws.proto` PushMessagePayload 新增 `is_system`（字段号 9），标识群变更系统消息；`gateway.proto` PushMessageReq 新增 `is_system`（字段号 11），core ConversationEventConsumer 向 gateway 推送群变更消息时设为 true。参见 `references/protocol-rules.md` §群变更系统消息。
- 2026-05-22: `gateway.proto` 新增 `PushTyping` RPC（`PushTypingReq`/`PushTypingResp`），用于 core `TypingConsumer` 向目标用户网关推送输入状态；字段号 1-4 对应 target_user_id/from_user_id/conversation_id/timestamp。
- 2026-05-22: `PushPresenceReq` 新增 `target_user_id`（字段号 4），解决在线状态/输入中推送无法到达客户端的问题。`PushPresence` 兼容 `target_user_id == 0` 时回退到 `user_id` 寻址。参见 `references/protocol-rules.md` §向后兼容规则。
- 2026-05-20: 从 shared/proto/AGENTS.md 迁移