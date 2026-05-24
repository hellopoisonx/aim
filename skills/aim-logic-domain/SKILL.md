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

- 2026-05-24: 成员详情查询新增 `display_name` 字段。`GetConversationMembersDetail` SQL JOIN 新增 `ui.nickname AS display_name`，`MemberDetail` struct 新增 `DisplayName` 字段，`GetConversationMembersDetail` 映射行更新。`GetConversationHistory` 现在填充 `MessageItem.IsSystem`（`sender_id==0 && message_type=="system"`）、`ReadStateItem` 的 email/avatar/display_name（来自成员列表）、`MessageReadDetailItem.DisplayName`。`SenderInfo` 填充新增 `DisplayName`。
- 2026-05-23: 已读回执服务落地。新增 migration `005_conversation_read_states.sql` 与 sqlc queries `read_state.sql`（`UpsertConversationReadState` 用 `GREATEST` 保证单调递增）；`ConversationService` 新增 `UpdateReadReceipt`（校验会话存在 + 成员身份 + 三字段合法）与 `ListConversationReadStates`；`GetConversationHistory` 同步返回会话成员的已读游标。
- 2026-05-22: 新增群管理功能：`ConversationService` 新增 6 个 RPC（AddGroupMembers, RemoveGroupMembers, LeaveGroup, DismissGroup, UpdateGroupInfo, GetConversationMembersDetail）；数据库迁移 `004_group_management.sql` 为 conversations 表添加 name/avatar/creator_id 字段、conversation_members 表添加 role 字段；logic 通过 `aim.conversation.events` Kafka topic 生产群变更事件通知 core；`CreateConversation` 支持 name 参数且创建者 role 设为 owner。详见 `references/detail.md` §群管理 和 `references/api.md` §群管理 RPC。
- 2026-05-22: 新增 `config.DevConf` 及 yaml `Dev:` 块，提供 `TemporaryConversationMessageLimit`（默认 10，设 0/负数为不限制）；`DatabasePermissionChecker` 新增 `NewDatabasePermissionCheckerWithLimit` 构造函数。仅供本地开发 / 压测使用，生产保持默认。
- 2026-05-19: logic RPC 本地 docker 监听/注册端口从 8080 调整为 8082，避免与 core RPC 端口冲突。
- 2026-05-19: 为 logic Kafka consumers 补充 span.RecordError 观测，并修复 WS ACK 冲突映射；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: ConversationService RPC 暴露 GetConversationMembers，用于 core 投递链路查询 direct/group 会话成员。
- 2026-05-21: direct 会话去重（Find-or-Create）：`CreateConversation` 在 direct 类型下先通过 `GetDirectConversationByMembers` 查找已有活跃会话，找到则直接返回，避免创建重复的直接会话。
- 2026-05-21: ConversationService RPC 新增 `GetUserConversations`，返回指定用户参与的所有会话列表（通过 `GetConversationsByUserID` 查询）；该接口用于 gateway `GET /api/conversations` 端点。
