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

- 2026-05-22: 修复 `PresenceConsumer` 在线状态推送无法到达目标客户端 Bug：`PushPresenceReq` 新增 `TargetUserId` 字段，consumer 将 `friendID` 写入 `TargetUserId`，`GatewayServer.PushPresence` 改用 `TargetUserId` 寻址，避免错误使用状态变更者本人的 userId 查连接。详见 `references/detail.md` §PresenceConsumer 修复。
- 2026-05-22: 新增 `PresenceConsumer`（消费 `aim.presence.events`，查好友 → 查目录 → 调 `gateway.PushPresence`）和 `TypingConsumer`（消费 `aim.typing.events`，查成员 → 查目录 → 调 `gateway.PushTyping`）；`PresenceStore` 改为 Set 语义；`GatewayRouter` 按 `node_id` 路由。
- 2026-05-19: 为 core Kafka delivery consumer 补充 span.RecordError 观测；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: 修复消息投递链路：core delivery consumer 通过 logic ConversationService.GetConversationMembers 查询会话成员，并对成员去重 fanout 到 gateway，避免只回推 sender。
