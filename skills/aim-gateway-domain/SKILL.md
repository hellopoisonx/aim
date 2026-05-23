---
name: aim-gateway-domain
description: aim 的网关域。对应 `gateway` 模块。
---
# aim-gateway-domain

## 如何使用

- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`
- 涉及 WebSocket 内部实现 -> `references/ws-internals.md`

## 参考资料

- `references/detail.md`
- `references/api.md`
- `references/ws-internals.md`

## 最近变更

- 2026-05-23: 已读回执后端接入。WS handler 新增 `handleReadReceipt` 分支（解码 `ReadReceiptPayload` → 调 `logic.UpdateReadReceipt` upsert → 发布 `aim.read_receipt.events` Kafka 事件 → SERVER_ACK）。`GatewayServer` 新增 `PushReadReceipt` RPC，按 `target_user_id` 把 `FRAME_TYPE_PUSH_READ_RECEIPT` 推到目标用户的所有连接。`GET /api/conversations/history/:id` 响应新增 `read_states` 数组（`user_id / last_read_message_id / updated_at`）。新增 `ReadReceiptPublisher` 与配置项 `Kafka.ReadReceiptTopic`（默认 `aim.read_receipt.events`）。
- 2026-05-22: 新增 `POST /api/conversations/group` 专用创建群聊端点。请求体只需 `member_ids`（必填）、`name`/`avatar`（可选），无需指定 `conversation_type`。底层复用 `ConversationService.CreateConversation` RPC（固定 `conversation_type="group"`），响应类型与 `POST /api/conversations` 一致。详见 `references/api.md`。
- 2026-05-22: 新增群管理 REST 端点：`GET /api/conversations/:id/members`（成员详情）、`POST /api/conversations/:id/members`（添加成员）、`DELETE /api/conversations/:id/members/:uid`（移除成员）、`POST /api/conversations/:id/leave`（退出群聊）、`DELETE /api/conversations/:id`（解散群聊）、`PUT /api/conversations/:id`（更新群信息）。`ConversationItem` 和 `CreateConversationResponse` 新增 name/avatar/creator_id 字段。`PushMessage` 传递 `is_system` 字段标识群变更系统消息。详见 `references/api.md` §群管理 REST 端点。
- 2026-05-22: 修复 `PushPresence` 推送寻址 Bug：`PushPresenceReq` 改用 `TargetUserId` 查找目标用户连接，兼容 `TargetUserId == 0` 时回退到 `UserId`。新增 `TestGatewayServerPushPresenceFallbackToUserId` 测试覆盖回退兼容路径。参见 `references/ws-internals.md` §PushPresence。
- 2026-05-22: 打通 presence/typing 推送链路：新增 `PushTyping` gRPC、`GET /api/presence/friends` 快照接口；Manager 维护 Redis Set 聚合多设备状态；用 kq.Pusher 真发 `aim.presence.events` 和 `aim.typing.events`；PresenceTTL 默认 45s，客户端心跳 20s。
- 2026-05-21: 新增 `GET /api/conversations` 端点，返回当前用户参与的所有会话列表（包含会话基本信息和成员列表）；该端点受 `Auth` 中间件保护，调用 `LogicConversationClient.GetUserConversations` 获取数据。
- 2026-05-19: gateway RPC 容器监听改为 `0.0.0.0:9090`；Nacos resolver 在服务列表为空时不上报空地址列表，避免启动期空服务列表失败。
- 2026-05-19: 补齐 gateway 生产 WS 路由注册与 WS ACK 409 映射；接入 RPC 统一 unary 错误拦截器。
