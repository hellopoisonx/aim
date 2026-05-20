---
name: aim-logic-domain
description: aim 的逻辑域。对应 `logic` 模块。
---
# aim-logic-domain

## 如何使用

- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`

## 参考资料

- `references/detail.md`
- `references/api.md`

## 最近变更

- 2026-05-19: logic RPC 本地 docker 监听/注册端口从 8080 调整为 8082，避免与 core RPC 端口冲突。
- 2026-05-19: 为 logic Kafka consumers 补充 span.RecordError 观测，并修复 WS ACK 冲突映射；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: ConversationService RPC 暴露 GetConversationMembers，用于 core 投递链路查询 direct/group 会话成员。
