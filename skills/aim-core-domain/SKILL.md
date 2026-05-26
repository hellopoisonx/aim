---
name: aim-core-domain
description: aim 的核心域。对应 `core` 模块。
---
# aim-core-domain

## 如何使用

- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`
- 涉及 Transfer 热路径与幂等规则 -> `references/transfer-pipeline.md`

## 参考资料

- `references/detail.md`
- `references/api.md`
- `references/transfer-pipeline.md`

## 最近变更

- 2026-05-25: Core 附件引用校验改为调用 `AttachmentService.ValidateReference` gRPC，保留 `core.attachment.validate_reference` client span；配置改为 `AttachmentRpc` Nacos 服务发现。
- 2026-05-25: Transfer 热路径新增附件消息校验：`image`/`video`/`audio` 的 `content` 必须符合 `aim.attachment.v1` JSON schema，并通过 attachment 服务校验上传完成、发送者、会话归属和类型匹配；配置新增 `AttachmentRpc`。
- 2026-05-24: `core.yaml` 补齐 `ReadReceiptConsumerConf`，确保 `aim.read_receipt.events` 由 core 消费并通过 `PushReadReceipt` 跨节点转发；typing/read_receipt consumer 会跳过事件中的源 `gateway_node_id`，避免 Gateway 本节点即时推送后 Kafka 回流重复；`docker-compose.yaml` 的 kafka-init 同步创建 presence/typing/read_receipt topics，避免依赖 Kafka 自动建 topic。
- 2026-05-23: 新增 `ReadReceiptConsumer`：消费 `aim.read_receipt.events`，查会话成员后通过 `GatewayClient.PushReadReceipt` 把已读游标推到除发送者外的成员所在网关节点。`GatewayPusher` 接口与 `GatewayRouter` 同步新增 `PushReadReceipt`。配置结构 `ReadReceiptConsumerConf kq.KqConf`。
- 2026-05-22: 新增 `ConversationEventConsumer`：消费 `aim.conversation.events` topic，将群变更系统消息（AddGroupMembers, RemoveGroupMembers, LeaveGroup, DismissGroup, UpdateGroupInfo）通过 `GatewayClient.PushMessage`（`is_system=true`）推送给每个 target_user_id。配置结构 `ConversationEventConsumerConf kq.KqConf`。详见 `references/detail.md` §ConversationEventConsumer。
- 2026-05-22: 修复 `PresenceConsumer` 在线状态推送无法到达目标客户端 Bug：`PushPresenceReq` 新增 `TargetUserId` 字段，consumer 将 `friendID` 写入 `TargetUserId`，`GatewayServer.PushPresence` 改用 `TargetUserId` 寻址，避免错误使用状态变更者本人的 userId 查连接。详见 `references/detail.md` §PresenceConsumer 修复。
- 2026-05-22: 新增 `PresenceConsumer`（消费 `aim.presence.events`，查好友 → 查目录 → 调 `gateway.PushPresence`）和 `TypingConsumer`（消费 `aim.typing.events`，查成员 → 查目录 → 调 `gateway.PushTyping`）；`PresenceStore` 改为 Set 语义；`GatewayRouter` 按 `node_id` 路由。
- 2026-05-19: 为 core Kafka delivery consumer 补充 span.RecordError 观测；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: 修复消息投递链路：core delivery consumer 通过 logic ConversationService.GetConversationMembers 查询会话成员，并对成员去重 fanout 到 gateway，避免只回推 sender。
