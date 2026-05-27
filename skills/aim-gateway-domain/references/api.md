# 接口定义

## 外部接口边界

- Gateway 是 AIM 唯一面向客户端/公网的 REST API 与 WebSocket 入口；所有新增客户端 REST/WS 协议必须先落到 `app/gateway/api/gateway.api`。
- 其他模块不得新增对外 REST/WS 端点；若需要访问 auth/core/logic/attachment/data_parsing 能力，由 gateway 通过内部 gRPC/Kafka 编排，并负责鉴权、错误清洗与响应包装。

## REST API - `/api`

### 规范

- 统一返回http status code + json
```json5
{
    "code": xxxx, // 业务错误码 (0 -> ok)
    "msg": "xxx", // 错误详情/暴露给前端的消息
    "body": any // 返回给前端的数据
}
```

- 对于grpc内部的错误 对外包装为 `internal error`, 但日志以及链路追踪上下文仍然保留原始错误链

### JWT 鉴权

- Headers.Authorization: Bearer <Token>
- payload: user_id && device_id
- REST 受保护端点（`/api/conversations`、`/api/users`）通过 `Auth` 中间件（`app/gateway/api/internal/middleware/auth_middleware.go`）验签 JWT token，成功后将 `ws.Identity{UserID, DeviceID}` 注入 `ws.WithIdentity` 上下文；失败返回 401 JSON `{code, msg}`。
- WS `/ws` 端点通过 `wsauth.ExtractAndValidate` 验签，独立于 REST 中间件。
- 中间件使用 `Config.Auth.AccessSecret` 密钥，与 auth 服务签发 token 使用相同密钥。

### 代理转发  `auth` - `/api/auth`

- `/register` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**
- `/login` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**
- `/logout` 解析 JWT payload 填入 gRPC 请求参数 **需要鉴权**
- `/refresh` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**

当前实现位置：`app/gateway/api`。

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| POST | `/api/auth/register` | 无需鉴权 | `internal/handler/auth/register_handler.go` |
| POST | `/api/auth/login` | 无需鉴权 | `internal/handler/auth/login_handler.go` |
| POST | `/api/auth/refresh` | 无需鉴权 | `internal/handler/auth/refresh_handler.go` |
| POST | `/api/auth/logout` | `Authorization: Bearer <access_token>` | `internal/handler/auth/logout_handler.go` |

`logout` 使用 `internal/authctx` 将 HTTP `Authorization` header 传入 logic，`internal/logic/auth/logout_logic.go` 本地验证 JWT 后调用 `aim-auth Logout`。

### 代理转发 `logic` - `/api/users`

- `GET /api/users/by-name/:name` 通过 `LogicRpc` 连接 `aim-logic`，调用 `UserService.SearchUserInfoByNickname` 做昵称模糊查询，返回用户列表项 `id/email/avatar`。nickname 不唯一，不要在 REST 层把 by-name 当作单用户详情查询。
- `GET /api/users/by-id/:id` 通过 `LogicRpc` 连接 `aim-logic`，调用 `UserService.GetUserInfo` 查询单个用户详情。
- `POST /api/users/friends/:id` 通过 `LogicRpc` 连接 `aim-logic`，调用 `FriendshipService.AddFriend`，将认证用户 `user_id` 与路径参数 `id` 建立好友关系请求；认证用户来自 `ws.IdentityFromContext(l.ctx)`，路径 `id` 必须为正数。
- `LogicRpc` 配置位于 `app/gateway/api/etc/gateway-api.yaml`，配置结构为 `app/gateway/api/internal/config/config.go` 的 `LogicRpc aimnacos.Config`。
- `app/gateway/api/internal/svc/service_context.go` 通过 Nacos resolver 使用 `nacos:///logic.rpc` 创建 `userservice.UserService`、`friendshipservice.FriendshipService` 客户端。
- 用户端点均受 `Auth` 中间件保护，需要有效的 Bearer JWT token。

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| POST | `/api/users/friends/:id` | `Auth` 中间件（JWT Bearer token） | `internal/handler/users/add_friend_handler.go` |

### 代理转发 `logic` - `/api/friends`

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| GET | `/api/friends/me` | `Auth` 中间件（JWT Bearer token） | `internal/handler/friends/list_friends_handler.go` |
| GET | `/api/friends/applications` | `Auth` 中间件（JWT Bearer token） | `internal/handler/friends/list_friend_applications_handler.go` |
| POST | `/api/friends/accept/:id` | `Auth` 中间件（JWT Bearer token） | `internal/handler/friends/accept_friend_handler.go` |
| POST | `/api/friends/reject/:id` | `Auth` 中间件（JWT Bearer token） | `internal/handler/friends/reject_friend_handler.go` |

`GET /api/friends/me` 从 JWT payload 提取 `user_id`，调用 `FriendshipService.ListFriends` 返回当前用户的所有已接受好友列表。

### 代理转发 `logic` - `/api/conversations`

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| POST | `/api/conversations` | `Auth` 中间件（JWT Bearer token） | `internal/handler/conversations/create_conversation_handler.go` |
| POST | `/api/conversations/group` | `Auth` 中间件（JWT Bearer token） | `internal/handler/conversations/create_group_handler.go` |
| GET | `/api/conversations` | `Auth` 中间件（JWT Bearer token） | `internal/handler/conversations/list_conversations_handler.go` |

- `POST /api/conversations` 调用 `ConversationService.CreateConversation`（通过 `LogicRpc`）创建直聊/群聊会话。
- `POST /api/conversations/group` 调用 `ConversationService.CreateConversation`（通过 `LogicRpc`）创建群聊会话。请求体无需 `conversation_type` 字段（固定为 `"group"`），支持 `name`（群名）和 `avatar`（群头像）可选字段。底层复用同一 RPC 方法，响应类型与 `POST /api/conversations` 一致。
- `GET /api/conversations` 调用 `ConversationService.GetUserConversations`（通过 `LogicRpc`）返回当前用户参与的所有会话，每条记录包含 `conversation_id`、`conversation_type`、`is_active`、`created_at`、`member_ids`。
- 两个端点均受 `Auth` 中间件保护，`user_id` 从 JWT payload 提取。

### 群管理 REST 端点

| Method | Path | Auth | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/conversations/:id/members` | `Auth` 中间件 | 获取成员详情列表（`user_id`, `email`, `avatar`, `role`, `joined_at`） |
| `POST` | `/api/conversations/:id/members` | `Auth` 中间件 | 邀请成员入群；服务端仅允许 `owner` 执行 |
| `DELETE` | `/api/conversations/:id/members/:uid` | `Auth` 中间件 | 移除单个群成员；服务端仅允许 `owner` 执行 |
| `POST` | `/api/conversations/:id/leave` | `Auth` 中间件 | 退出群聊；`owner` 需先转让或解散群聊 |
| `DELETE` | `/api/conversations/:id` | `Auth` 中间件 | 解散群聊；仅 `owner` 可操作 |
| `PUT` | `/api/conversations/:id` | `Auth` 中间件 | 更新群信息（body: `name`, `avatar`，均为 optional；仅 `owner` 可操作） |

以上端点均通过 `LogicRpc` 调用 `ConversationService` 的对应 RPC（`AddGroupMembers`, `RemoveGroupMembers`, `LeaveGroup`, `DismissGroup`, `UpdateGroupInfo`, `GetConversationMembersDetail`）。`user_id` 和 `operator_id` 从 JWT payload 提取。服务端在 logic 层统一执行角色校验：`owner / admin / member` 的权限边界以 `app/logic/rpc/internal/service/conversation_service.go` 为准，gateway 只负责传递身份与参数。

**角色约束说明**：当前群管理接口按“群主强约束”实现，`admin` 角色虽然已入库并可透出到成员详情，但暂未开放额外管理权限；后续若开放管理员能力，需要同步更新 logic 侧权限矩阵与 gateway 文档。

**类型更新**：`ConversationItem`、`CreateConversationRequest`、`CreateConversationResponse` 新增 `name`、`avatar`、`creator_id` 字段。`CreateConversationRequest` 新增 `Name` 字段用于创建群聊时指定群名。

**PushMessage is_system**：`GatewayServer.PushMessage` 将 gRPC `PushMessageReq.is_system` 传递至 WebSocket `PushMessagePayload.is_system`，前端据此区分群变更系统消息和普通消息。

重新生成 REST 脚手架：

```bash
goctl api validate -api app/gateway/api/gateway.api
goctl api go -api app/gateway/api/gateway.api -dir app/gateway/api --style go_zero
```

重新生成后需要检查 `logout_handler.go` 是否仍把 `Authorization` header 写入 `authctx`；goctl 可能覆盖 handler。

### 好友在线状态快照 - `/api/presence`

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| GET | `/api/presence/friends` | `Auth` 中间件（JWT Bearer token） | `internal/handler/presence/get_friends_presence_handler.go` |

- 实现位置：`app/gateway/api/internal/logic/presence/get_friends_presence_logic.go`
- 从 `ws.IdentityFromContext` 提取当前用户 ID，调用 `LogicFriendshipClient.ListFriends` 获取好友列表
- 通过 Redis pipeline 批量 SCARD `aim:presence:{friend_id}` 得到各好友设备数，`>0` 为 `online` 否则 `offline`
- 返回 `[{user_id, status}]` 列表，用于客户端 WS 连接/重连后填充初始在线状态
- 不在 Redis 中记录 `updated_at`，`PresenceItem.updated_at` 字段保留为 0；实时时间戳由 Kafka 事件侧维护

### 附件代理接口 - `/api/attachments`

| Method | Path | Auth | Handler |
| --- | --- | --- | --- |
| POST | `/api/attachments/init` | `Auth` 中间件（JWT Bearer token） | `internal/handler/attachments/handler.go` |
| GET | `/api/attachments/:id` | `Auth` 中间件（JWT Bearer token） | `internal/handler/attachments/handler.go` |
| POST | `/api/attachments/:id/complete` | `Auth` 中间件（JWT Bearer token） | `internal/handler/attachments/handler.go` |
| GET | `/api/attachments/:id/download` | `Auth` 中间件（JWT Bearer token） | `internal/handler/attachments/handler.go` |

- Gateway 只负责把附件相关 REST 请求转换为 `AttachmentService` gRPC 调用，用户身份从 JWT 注入 `owner_id/user_id`。
- 客户端仍只与 gateway 通信；SeaweedFS 直传 URL 由 attachment 服务签发。
- 附件内容通过 `aim.attachment.v1` JSON schema 透传，普通消息仍保持原有 `message_type + content` 兼容路径。
- `kind` 支持 `image` / `video` / `audio` / `file`；`file` 用于普通文件，上传完成后不进入 data_parsing。

### `ws` 升级 - `/ws`

将连接升级为 全双工 WebSocket

## gRPC

```protobuf
syntax = "proto3";

package gateway;

option go_package = "github.com/hellopoisonx/aim/shared/proto/gateway;pb";

// GatewayService — aim-core 的 Delivery Consumer 通过此服务向网关推送消息。
// Gateway 对内暴露 gRPC 接口，供 aim-core 的 Delivery Consumer 调用。
service GatewayService {
  // PushMessage 推送聊天消息到目标用户的 WebSocket 连接。
  // 调用方：aim-core/Delivery Consumer 从 Kafka 消费消息后，查找目标用户所在网关节点，调用此方法投递。
  rpc PushMessage(PushMessageReq) returns (PushMessageResp);

  // PushPresence 推送用户在线状态变更通知给目标用户。
  // 调用方：aim-core/Presence Consumer 消费 presence 事件后，通过此方法推送到目标用户所在网关。
  rpc PushPresence(PushPresenceReq) returns (PushPresenceResp);

  // PushTyping 推送输入状态给目标用户。
  // 调用方：aim-core/Typing Consumer 消费 typing 事件后，通过此方法推送到目标用户所在网关。
  rpc PushTyping(PushTypingReq) returns (PushTypingResp);

  // KickUser 踢下线指定用户设备。
  // 调用方：aim-auth 多端登录策略触发 / 管理员后台操作。
  rpc KickUser(KickUserReq) returns (KickUserResp);

  // DrainNotify 通知网关节点进行优雅迁移（会话 drain）。
  // 调用方：Nacos 服务发现 / 运维管理工具，在网关节点下线前通知其推送 reconnect 帧给客户端。
  rpc DrainNotify(DrainNotifyReq) returns (DrainNotifyResp);

  // PushFriendApplication 推送好友申请通知给目标用户。
  // 调用方：aim-logic/FriendshipService 在好友申请创建后调用此方法通知被申请方。
  rpc PushFriendApplication(PushFriendApplicationReq) returns (PushFriendApplicationResp);
}

// ============================================================
// 推送聊天消息
// ============================================================

message PushMessageReq {
  int64  user_id           = 1; // 接收消息的目标用户 ID
  int64  conversation_id   = 2; // 会话 ID
  int64  message_id        = 3; // 服务端消息 ID（全局唯一，用于幂等和已读回执）
  string message_type      = 4; // 消息类型：text / image / file / system
  string content          = 5; // 消息内容（text 为纯文本，其余为 JSONB）
  int64  sender_id         = 6; // 发送者用户 ID
  int64  sent_at           = 7; // 发送时间戳 Unix ms
  string conversation_type = 8; // 会话类型：private / group
}

message PushMessageResp {
  bool success = 1; // 投递是否成功（用户不在线时 success=false，但不算错误）
}

// ============================================================
// 推送在线状态
// ============================================================

message PushPresenceReq {
  int64          user_id          = 1; // 状态发生变更的用户 ID
  string         status           = 2; // online / offline / typing
  int64          updated_at       = 3; // 变更时间戳 Unix ms
  repeated int64 notify_user_ids = 4; // 需要通知的好友用户 ID 列表
}

message PushPresenceResp {
  bool success = 1;
}

// ============================================================
// 推送输入状态
// ============================================================

message PushTypingReq {
  int64 target_user_id  = 1; // 目标用户 ID
  int64 from_user_id    = 2; // 正在输入的用户 ID
  int64 conversation_id = 3; // 会话 ID
  int64 timestamp       = 4; // 事件时间戳 Unix ms
}

message PushTypingResp {
  bool success = 1;
}

// ============================================================
// 踢下线
// ============================================================

message KickUserReq {
  int64  user_id   = 1; // 要踢下线的用户 ID
  string device_id = 2; // 指定设备（空字符串 = 所有设备）
  string reason    = 3; // 原因：duplicate_login / admin_kick / security
}

message KickUserResp {
  bool success = 1;
}

// ============================================================
// 会话 Drain 通知
// ============================================================

message DrainNotifyReq {
  string gateway_node_id  = 1; // 发起 drain 的网关节点标识
  int64  drain_timeout_sec = 2; // drain 窗口时间（秒）
}

message DrainNotifyResp {
  bool success = 1;
}

// ============================================================
// 推送好友申请通知
// ============================================================

message PushFriendApplicationReq {
  int64  user_id       = 1; // 接收通知的目标用户 ID
  int64  application_id = 2; // 好友申请 ID
  int64  from_user_id   = 3; // 发送请求的用户 ID
  string from_nickname  = 4; // 发送请求的用户昵称
  int64  created_at      = 5; // 申请时间戳 Unix ms
}

message PushFriendApplicationResp {
  bool success = 1;
}
```

## CoreRpc 配置

配置路径：`app/gateway/rpc/etc/gateway-rpc.yaml`，配置结构定义：`app/gateway/rpc/internal/config/config.go`。

```go
type CoreRpcConf struct {
    zrpc.RpcClientConf // 包含 Target / App / Timeout 等字段
}
```

目标地址通过 Nacos resolver 发现（scheme `nacos:///core.rpc`），需在配置中指定 Nacos 注册中心地址（与 AuthRpc 配置方式一致）。

调用示例：`app/gateway/rpc/internal/svc/service_context.go` 注入 `CoreRpc` 到 ServiceContext，供 `Transfer` logic 使用。

## `ws` 通信
```protobuf
syntax = "proto3";

package ws;

option go_package = "github.com/hellopoisonx/aim/shared/proto/ws;pb";

// ============================================================
// WebSocket 帧类型枚举
// ============================================================

enum FrameType {
  FRAME_TYPE_UNSPECIFIED    = 0;

  // ---- 客户端 → 网关 ----
  FRAME_TYPE_SEND_MESSAGE   = 1;  // 发送聊天消息
  FRAME_TYPE_HEARTBEAT      = 2;  // 心跳保活
  FRAME_TYPE_TYPING         = 3;  // 正在输入
  FRAME_TYPE_READ_RECEIPT  = 4;  // 已读回执
  FRAME_TYPE_ACK            = 5;  // 客户端消息确认

  // ---- 网关 → 客户端 ----
  FRAME_TYPE_PUSH_MESSAGE      = 101; // 推送聊天消息
  FRAME_TYPE_PUSH_PRESENCE     = 102; // 推送在线状态
  FRAME_TYPE_PUSH_NOTIFICATION = 103; // 推送系统通知
  FRAME_TYPE_PUSH_TYPING       = 104; // 推送输入状态
  FRAME_TYPE_RECONNECT        = 105; // 网关要求重连（drain 窗口）
  FRAME_TYPE_SERVER_ACK       = 106; // 服务端确认
  FRAME_TYPE_TOKEN_EXPIRED    = 107; // Token 过期通知
  FRAME_TYPE_PUSH_FRIEND_APPLICATION = 108; // 推送好友申请通知

// ============================================================
// WsFrame — 所有 WebSocket 通信统一使用此帧格式封装。
//
// 协议流程：
//   1. 客户端序列化具体 Payload（如 SendMessagePayload）为 bytes
//   2. 填入 WsFrame.payload，设置 type 和 seq
//   3. 序列化 WsFrame 为二进制，通过 WebSocket 发送
//   4. 接收方反序列化 WsFrame，根据 type 决定 payload 的具体类型
//
// 序列号规则：
//   - 客户端 seq：单向递增，由客户端维护
//   - 服务端 seq：单向递增，由网关维护
//   - ACK 匹配：ServerAck.ack_seq 匹配客户端 seq；ClientAck.ack_seq 匹配服务端 seq
// ============================================================

message WsFrame {
  FrameType type      = 1; // 帧类型
  int64     seq       = 2; // 序列号（双向各自递增，用于 ACK 匹配）
  bytes    payload   = 3; // 具体消息体（按 type 反序列化为对应 Payload protobuf）
  int64    timestamp = 4; // 时间戳 Unix ms
}

// ============================================================
// 客户端 → 网关 消息体
// ============================================================

// SendMessagePayload — 客户端发送聊天消息
// 网关收到后：验证 token → 调用 aim-core/Transfer 转发 → 返回 ServerAck
message SendMessagePayload {
  int64          conversation_id = 1; // 会话 ID
  string         message_type    = 2; // 消息类型：text / image / file
  string         content         = 3; // 消息内容（text 为纯文本，image/file 为 JSON）
  string         client_msg_id   = 4; // 客户端消息 ID（幂等去重，UUID 格式）
  repeated string mentions       = 5; // @提及的用户 ID 列表
}

// HeartbeatPayload — 心跳保活
// 客户端每 30s 发送一次，网关据此维护用户在线状态
message HeartbeatPayload {
  int64 last_seq = 1; // 客户端最新收到的服务端消息序列号（用于断线重连后的消息补发）
}

// TypingPayload — 正在输入通知
message TypingPayload {
  int64 conversation_id = 1; // 正在输入的会话 ID
}

// ReadReceiptPayload — 已读回执
message ReadReceiptPayload {
  int64 conversation_id = 1; // 已读会话 ID
  int64 last_msg_id     = 2; // 最后已读消息 ID
}

// ClientAckPayload — 客户端确认收到服务端推送
message ClientAckPayload {
  int64 ack_seq = 1; // 确认收到的服务端序列号
}

// ============================================================
// 网关 → 客户端 消息体
// ============================================================

// PushMessagePayload — 推送聊天消息
message PushMessagePayload {
  int64  message_id        = 1; // 服务端消息 ID（全局唯一）
  int64  conversation_id   = 2; // 会话 ID
  string message_type      = 3; // 消息类型：text / image / file / system
  string content           = 4; // 消息内容
  int64  sender_id         = 5; // 发送者用户 ID
  int64  sent_at           = 6; // 发送时间戳 Unix ms
  string conversation_type = 7; // 会话类型：private / group
  string client_msg_id     = 8; // 发送者的客户端消息 ID（回传，供发送方确认）
}

// PushPresencePayload — 推送在线状态变更
message PushPresencePayload {
  int64  user_id    = 1; // 状态变更的用户 ID
  string status     = 2; // online / offline / typing
  int64  updated_at = 3; // 变更时间戳 Unix ms
}

// PushNotificationPayload — 推送系统通知
message PushNotificationPayload {
  string notification_type = 1; // 通知类型：friend_request / group_invite / system_notice
  string title             = 2; // 通知标题
  string body              = 3; // 通知内容（JSONB）
  int64  related_id        = 4; // 关联资源 ID（好友请求 ID / 群组 ID 等）
}

// PushTypingPayload — 推送输入状态
message PushTypingPayload {
  int64 user_id         = 1; // 正在输入的用户 ID
  int64 conversation_id = 2; // 会话 ID
}

// PushFriendApplicationPayload — 推送好友申请通知
message PushFriendApplicationPayload {
  int64 application_id = 1; // 好友申请 ID
  int64 from_user_id    = 2; // 发送请求的用户 ID
  string from_nickname  = 3; // 发送请求的用户昵称
  int64 created_at      = 4; // 申请时间戳 Unix ms
}

// ReconnectPayload — 网关要求客户端重连（drain 窗口）
// 网关节点下线前推送此帧，客户端应在 reconnect_delay_ms 后重新连接
message ReconnectPayload {
  int64  reconnect_delay_ms = 1; // 建议重连延迟（毫秒）
  string gateway_node_id    = 2; // 建议连接的网关节点标识（可选，空则由负载均衡决定）
}

// ServerAckPayload — 服务端确认收到客户端消息
message ServerAckPayload {
  int64  ack_seq       = 1; // 确认的客户端序列号
  string client_msg_id = 2; // 确认的客户端消息 ID（与 SendMessagePayload.client_msg_id 对应）
}
```
