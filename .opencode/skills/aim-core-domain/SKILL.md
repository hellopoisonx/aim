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

- 2026-05-19: 为 core Kafka delivery consumer 补充 span.RecordError 观测；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: 修复消息投递链路：core delivery consumer 通过 logic ConversationService.GetConversationMembers 查询会话成员，并对成员去重 fanout 到 gateway，避免只回推 sender。
