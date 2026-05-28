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

- 2026-05-28: `app/logic/rpc/etc/logic.yaml` 已启用 `ConversationEventProducerConf`，logic 在群管理事务提交后会将系统消息事件发布到 `aim.conversation.events`，由 core 的 `ConversationEventConsumer` 推送到 Gateway。
- 2026-05-27: `DatabasePermissionChecker` 对 direct 会话新增 Bot 识别：当发送者或对端 `user_info.user_type='bot'` 时，非好友关系仍保留 block 拦截但跳过临时会话累计消息上限，避免 user ↔ echo-bot 对话达到 10 条后被误判为临时会话限额耗尽。
- 2026-05-25: ArchiveConsumer 修正 `messages.content` JSONB 写入：附件消息等合法 JSON 内容按 JSON object 保存，普通文本消息仍按 JSON string 保存，避免附件 `aim.attachment.v1` 被二次编码。
- 2026-05-25: 修复 direct 会话好友权限校验误用 `conversation_id` 作为好友 `friend_id` 的问题。`DatabasePermissionChecker` 现在先从 `conversation_members` 解析发送者与对端成员，再用对端用户 ID 查询 `GetFriendshipBidirectional`，避免已成为好友后仍被判定为临时会话；同时拒绝非成员或成员数异常的 direct 会话发送。
- 2026-05-24: `ListFriends` 查询改为双向归一化：同时读取 `user_id = 当前用户` 与 `friend_id = 当前用户` 的 accepted 关系，并统一返回 `user_id=当前用户 / friend_id=对端用户`，兼容历史单向 accepted 数据导致 presence/好友快照不对称的问题。
- 2026-05-24: 修复历史消息内容从 `messages.content` JSONB 读取后透传 JSON 字符串字面量的问题。`GetConversationHistory` 现在会对 JSONB 字符串执行 `json.Unmarshal`，避免 Desktop/REST 历史消息展示多出首尾两个 `"`；非字符串 JSON（如系统消息对象）保持 JSON 文本。
- 2026-05-24: 成员详情查询新增 `display_name` 字段。`GetConversationMembersDetail` SQL JOIN 新增 `ui.nickname AS display_name`，`MemberDetail` struct 新增 `DisplayName` 字段，`GetConversationMembersDetail` 映射行更新。`GetConversationHistory` 现在填充 `MessageItem.IsSystem`（`sender_id==0 && message_type=="system"`）、`ReadStateItem` 的 email/avatar/display_name（来自成员列表）、`MessageReadDetailItem.DisplayName`。`SenderInfo` 填充新增 `DisplayName`。
- 2026-05-23: 已读回执服务落地。新增 migration `005_conversation_read_states.sql` 与 sqlc queries `read_state.sql`（`UpsertConversationReadState` 用 `GREATEST` 保证单调递增）；`ConversationService` 新增 `UpdateReadReceipt`（校验会话存在 + 成员身份 + 三字段合法）与 `ListConversationReadStates`；`GetConversationHistory` 同步返回会话成员的已读游标。
- 2026-05-22: 新增群管理功能：`ConversationService` 新增 6 个 RPC（AddGroupMembers, RemoveGroupMembers, LeaveGroup, DismissGroup, UpdateGroupInfo, GetConversationMembersDetail）；数据库迁移 `004_group_management.sql` 为 conversations 表添加 name/avatar/creator_id 字段、conversation_members 表添加 role 字段；logic 通过 `aim.conversation.events` Kafka topic 生产群变更事件通知 core；`CreateConversation` 支持 name 参数且创建者 role 设为 owner。详见 `references/detail.md` §群管理 和 `references/api.md` §群管理 RPC。
- 2026-05-22: 新增 `config.DevConf` 及 yaml `Dev:` 块，提供 `TemporaryConversationMessageLimit`（默认 10，设 0/负数为不限制）；`DatabasePermissionChecker` 新增 `NewDatabasePermissionCheckerWithLimit` 构造函数。仅供本地开发 / 压测使用，生产保持默认。
- 2026-05-19: logic RPC 本地 docker 监听/注册端口从 8080 调整为 8082，避免与 core RPC 端口冲突。
- 2026-05-19: 为 logic Kafka consumers 补充 span.RecordError 观测，并修复 WS ACK 冲突映射；接入 RPC 统一 unary 错误拦截器。
- 2026-05-20: ConversationService RPC 暴露 GetConversationMembers，用于 core 投递链路查询 direct/group 会话成员。
- 2026-05-21: direct 会话去重（Find-or-Create）：`CreateConversation` 在 direct 类型下先通过 `GetDirectConversationByMembers` 查找已有活跃会话，找到则直接返回，避免创建重复的直接会话。
- 2026-05-21: ConversationService RPC 新增 `GetUserConversations`，返回指定用户参与的所有会话列表（通过 `GetConversationsByUserID` 查询）；该接口用于 gateway `GET /api/conversations` 端点。
