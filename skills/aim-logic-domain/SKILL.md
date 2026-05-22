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

- 2026-05-22: 新增 `config.DevConf` 及 yaml `Dev:` 块，提供 `TemporaryConversationMessageLimit`（默认 10，设 0/负数为不限制）；`DatabasePermissionChecker` 新增 `NewDatabasePermissionCheckerWithLimit` 构造函数。仅供本地开发 / 压测使用，生产保持默认。

- 2026-05-19: logic RPC 本地 docker 监听/注册端口从 8080 调整为 8082，避免与 core RPC 端口冲突。
- 2026-05-19: 为 logic Kafka consumers 补充 span.RecordError 观测，并修复 WS ACK 冲突映射；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: ConversationService RPC 暴露 GetConversationMembers，用于 core 投递链路查询 direct/group 会话成员。
- 2026-05-21: direct 会话去重（Find-or-Create）：`CreateConversation` 在 direct 类型下先通过 `GetDirectConversationByMembers` 查找已有活跃会话，找到则直接返回，避免创建重复的直接会话。
- 2026-05-21: ConversationService RPC 新增 `GetUserConversations`，返回指定用户参与的所有会话列表（通过 `GetConversationsByUserID` 查询）；该接口用于 gateway `GET /api/conversations` 端点。
