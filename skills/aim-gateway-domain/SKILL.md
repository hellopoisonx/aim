---
name: aim-gateway-domain
description: aim 的网关域。对应 `gateway` 模块。
---
# aim-gateway-domain

## 如何使用

- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`
- 涉及 WebSocket 内部实现 -> `references/ws-internals.md`

## 外部接口边界

- 只有 gateway 可以面向客户端/公网暴露 REST API 和 WebSocket 入口；客户端（含 Desktop、第三方 Bot）只能访问 `app/gateway/api` 下的 `/api/*` 与 `/ws`。
- auth/core/logic/attachment/data_parsing 等非 gateway 模块不得新增对外 REST/WS 端口、`.api` 服务或 WebSocket handler；如需客户端能力，先在 `app/gateway/api/gateway.api` 声明，由 gateway 调内部 gRPC/Kafka。
- 允许的例外仅限服务间内部接口（如 GatewayService gRPC、attachment.rpc gRPC），Docker Compose 中不得将这些内部端口 publish 到宿主机；使用内部网络 `expose` 或仅容器名/Nacos 访问。

## 参考资料

- `references/detail.md`
- `references/api.md`
- `references/ws-internals.md`

## 最近变更

- 2026-05-25: 明确外部接口边界：只有 gateway 可对客户端/公网暴露 REST API 与 WebSocket；非 gateway 模块的 HTTP/REST 只能作为服务间内部接口，Docker Compose 不得 publish 内部 REST/WS 端口。
- 2026-05-25: attachment 服务由内部 HTTP 改为 `AttachmentService` gRPC；gateway `/api/attachments` 和 core 附件引用校验统一通过 `AttachmentRpc`/Nacos 调用，不再使用 `/v1/attachments/*` 内部 HTTP。
- 2026-05-25: Gateway 新增 `/api/attachments` 端点：`init`、`complete`、`download`、`get` 均受 Auth 中间件保护，通过 `AttachmentRpc` 调用 `aim-attachment`，客户端仍只面向 gateway 获取 SeaweedFS 直传/下载授权。
- 2026-05-24: Gateway WS 输入状态和已读回执新增本节点即时 fan-out：收到 `FRAME_TYPE_TYPING` / `FRAME_TYPE_READ_RECEIPT` 后仍发布 Kafka 给 core 跨节点转发，同时查询会话成员并直接推送到本节点在线连接，降低单节点 Desktop 的输入状态缺失和已读刷新延迟；typing/read_receipt Kafka 事件携带 `gateway_node_id` 供 core 跳过源节点，避免本节点重复推送；配置显式补齐 `Kafka.ReadReceiptTopic`。
- 2026-05-24: Gateway WS 连接新增 per-connection 写锁，`PushPresence`/`PushTyping`/`PushMessage`/`SERVER_ACK`/`TOKEN_EXPIRED` 等服务端写帧统一串行化，避免同一 WebSocket 上并发写导致推送帧丢失或连接异常。
- 2026-05-24: WebSocket upgrade 后的 per-connection context 使用 `shared/tracing.DetachSpanContext` 清除 HTTP upgrade span 作为当前 span，避免长连接期间 per-frame/RPC/Kafka 短 span 引用尚未结束的 upgrade span，触发 Jaeger `invalid parent span IDs=...; skipping clock skew adjustment` 警告。
- 2026-05-24: REST/WS 协议统一：所有返回用户 ID 的 REST 类型（`FriendshipItem`、`ConversationItem`、`MemberDetailItem`、`PresenceItem`、`SenderInfo`、`ReadStateItem`、`MessageReadDetailItem`、`UserInfo`、`UserListItem`）新增 `display_name` 快照字段；`MessageItem` 新增 `is_system` 布尔字段使 REST 历史接口与 WS `PushMessagePayload` 对齐。WS `SenderInfo` proto（ws.proto / gateway.proto）新增 `display_name` 字段号 3。Logic `GetConversationMembersDetail` SQL JOIN 新增 `ui.nickname AS display_name`；`GetConversationHistory` 现在填充 `is_system`（`sender_id==0 && message_type=="system"`）、`ReadStateItem` 的 email/avatar/display_name（来自成员列表）、`MessageReadDetailItem.display_name`。Gateway REST 各端点从 logic RPC 或 user service 获取 name 快照填充新字段。
- 2026-05-23: 已读回执后端接入。WS handler 新增 `handleReadReceipt` 分支（解码 `ReadReceiptPayload` → 调 `logic.UpdateReadReceipt` upsert → 发布 `aim.read_receipt.events` Kafka 事件 → SERVER_ACK）。`GatewayServer` 新增 `PushReadReceipt` RPC，按 `target_user_id` 把 `FRAME_TYPE_PUSH_READ_RECEIPT` 推到目标用户的所有连接。`GET /api/conversations/history/:id` 响应新增 `read_states` 数组（`user_id / last_read_message_id / updated_at`）。新增 `ReadReceiptPublisher` 与配置项 `Kafka.ReadReceiptTopic`（默认 `aim.read_receipt.events`）。
- 2026-05-22: 新增 `POST /api/conversations/group` 专用创建群聊端点。请求体只需 `member_ids`（必填）、`name`/`avatar`（可选），无需指定 `conversation_type`。底层复用 `ConversationService.CreateConversation` RPC（固定 `conversation_type="group"`），响应类型与 `POST /api/conversations` 一致。详见 `references/api.md`。
- 2026-05-22: 新增群管理 REST 端点：`GET /api/conversations/:id/members`（成员详情）、`POST /api/conversations/:id/members`（添加成员）、`DELETE /api/conversations/:id/members/:uid`（移除成员）、`POST /api/conversations/:id/leave`（退出群聊）、`DELETE /api/conversations/:id`（解散群聊）、`PUT /api/conversations/:id`（更新群信息）。`ConversationItem` 和 `CreateConversationResponse` 新增 name/avatar/creator_id 字段。`PushMessage` 传递 `is_system` 字段标识群变更系统消息。详见 `references/api.md` §群管理 REST 端点。
- 2026-05-22: 修复 `PushPresence` 推送寻址 Bug：`PushPresenceReq` 改用 `TargetUserId` 查找目标用户连接，兼容 `TargetUserId == 0` 时回退到 `UserId`。新增 `TestGatewayServerPushPresenceFallbackToUserId` 测试覆盖回退兼容路径。参见 `references/ws-internals.md` §PushPresence。
- 2026-05-22: 打通 presence/typing 推送链路：新增 `PushTyping` gRPC、`GET /api/presence/friends` 快照接口；Manager 维护 Redis Set 聚合多设备状态；用 kq.Pusher 真发 `aim.presence.events` 和 `aim.typing.events`；PresenceTTL 默认 45s，客户端心跳 20s。
- 2026-05-21: 新增 `GET /api/conversations` 端点，返回当前用户参与的所有会话列表（包含会话基本信息和成员列表）；该端点受 `Auth` 中间件保护，调用 `LogicConversationClient.GetUserConversations` 获取数据。
- 2026-05-19: gateway RPC 容器监听改为 `0.0.0.0:9090`；Nacos resolver 在服务列表为空时不上报空地址列表，避免启动期空服务列表失败。
- 2026-05-19: 补齐 gateway 生产 WS 路由注册与 WS ACK 409 映射；接入 RPC 统一 unary 错误拦截器。
