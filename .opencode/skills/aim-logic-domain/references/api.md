# 接口定义

## PermissionService RPC

Proto 定义：`app/logic/rpc/logic.proto`，生成的 pb 代码在 `app/logic/rpc/pb/`。

### PermissionService

```protobuf
service PermissionService {
  // CheckMessagePermission checks whether a sender can publish into a conversation.
  // aim-core calls this before accepting a message into Kafka.
  rpc CheckMessagePermission(CheckMessagePermissionReq) returns (CheckMessagePermissionResp);
}

message CheckMessagePermissionReq {
  int64 sender_id = 1;
  int64 conversation_id = 2;
  // message_type must be a short client-supported type such as text/image/file/voice/video.
  string message_type = 3;
  // mentions is capped by the logic service before policy evaluation.
  repeated int64 mentions = 4;
}

message CheckMessagePermissionResp {
  bool allowed = 1;
  int32 biz_code = 2;
  string reason = 3;
}
```

### 返回码约定

- `biz_code=0`：允许发送。
- `biz_code=40000`：参数校验失败。
- `biz_code=40300`：无权限，例如非会话成员、被禁言、被拉黑。
- `biz_code=40400`：会话不存在。
- 基础设施错误通过 gRPC `Internal` / `Unavailable` 返回，调用方按可重试错误处理。

### 安全约束

- 默认 `PermissionChecker` 必须 fail-closed，不允许使用 allow-all 作为生产默认值。
- `message_type` 长度上限为 32。
- `mentions` 最多 20 个用户 ID，且每个用户 ID 必须为正数。
- PermissionService 只应由授权内部服务调用；生产环境需要通过内网隔离或 mTLS 等方式保护服务间调用。

### 生成命令

从仓库根目录执行，避免生成冗余 `github.com/hellopoisonx/aim/...` 路径：

```bash
goctl rpc protoc app/logic/rpc/logic.proto --go_out=app/logic/rpc --go-grpc_out=app/logic/rpc --zrpc_out=app/logic/rpc --style go_zero -m
```

注意：由于 `logic.proto` 定义了多个 service（PermissionService、UserService、ConversationService），必须使用 `-m` 标志启用多 service 模式。

## ConversationService RPC

Proto 定义：`app/logic/rpc/logic.proto`，生成的 pb 代码在 `app/logic/rpc/pb/`。

### ConversationService

```protobuf
service ConversationService {
  // CreateConversation creates a new conversation (direct or group).
  rpc CreateConversation(CreateConversationReq) returns (CreateConversationResp);
  // GetConversationHistory retrieves message history for a conversation with cursor-based pagination.
  rpc GetConversationHistory(GetConversationHistoryReq) returns (GetConversationHistoryResp);
}
```

### CreateConversation

- 请求：`CreateConversationReq`（conversation_type: "direct" 或 "group"、creator_id、member_ids 列表）
- 响应：`CreateConversationResp`（包含 `ConversationResponse`：id, conversation_type, is_active, created_at, member_ids）
- 业务逻辑：验证对话类型（direct 必须 2 人、group 可多人）、自动将 creator 加入成员列表。**对于 direct 类型，先查找是否已有两名成员相同的活跃会话（Find-or-Create）**，存在则直接返回已有会话；不存在才创建新会话。group 类型不做去重（成员可动态变化）。
- 服务层：`ConversationService.CreateConversation` → `GetDirectConversationByMembers`（direct 去重） → sqlc `CreateConversation` + `AddConversationMembers`
- 错误码：40000（参数错误，如无效类型、空成员列表、direct 非 2 人）、40400（会话不存在）、40300（非成员）、50000（基础设施错误）

### GetConversationHistory

- 请求：`GetConversationHistoryReq`（conversation_id、cursor_created_at 游标、cursor_id 游标、limit 分页大小）
- 响应：`GetConversationHistoryResp`（messages 列表、next_cursor_created_at、next_cursor_id、has_more）
- 分页规则：当 `cursor_created_at=0 && cursor_id=0` 时返回最新一页；否则按 `(created_at, id)` 游标向前翻页；limit 默认 50，最大 100
- 服务层：`ConversationService.GetConversationHistory` → sqlc `ListMessagesByConversationInitial` 或 `ListMessagesByConversation`
- 错误码：40000（conversation_id 必须正数）、40400（会话不存在）、50000（基础设施错误）

### 数据与查询

- 对话创建：`app/logic/rpc/model/queries/conversation.sql` 包含 `CreateConversation`、`AddConversationMembers`、`GetConversationMembers`、`GetConversationsByUserID`、`GetDirectConversationByMembers`（direct 去重查询）
- 消息历史：`app/logic/rpc/model/queries/message.sql` 包含 `ListMessagesByConversation`（游标分页）、`ListMessagesByConversationInitial`（首页）、`CountMessagesByConversation`
- 服务封装：`app/logic/rpc/internal/service/conversation_service.go` 提供 `ConversationQuerier` 接口，封装 sqlc 查询和业务逻辑（验证、错误映射、分页规范化）
- ID 生成：`app/logic/rpc/internal/service/conversation_helpers.go` 提供 `GenerateConversationID()`（时间戳+原子计数器，TODO: 替换为 snowflake）、`PGTimestamptzFromUnix`/`UnixFromPGTimestamptz` 时间戳转换

### 返回码约定

- 参数错误通过 `errorx.NewCodeError(40000, ...)` 返回 gRPC `InvalidArgument`。
- 会话不存在通过 `errorx.NewCodeError(40400, "conversation not found")` 返回 gRPC `NotFound`。
- 非成员访问通过 `errorx.NewCodeError(40300, "not a member of the conversation")` 返回 gRPC `PermissionDenied`。
- 基础设施错误通过 `errorx.NewCodeError(50000, "internal error")` 返回 gRPC `Internal`，不泄漏底层错误细节。

### 边界

- `ConversationService` 只负责会话管理和消息历史查询，不负责消息投递（由 `aim-core` Transfer 负责）。
- 对话创建返回的 `member_ids` 来自 `conversation_members` 表，确保与实际成员一致。
- 消息历史查询的游标基于 `(created_at DESC, id DESC)` 排序，保证消息时序有序且游标稳定。
- `aim-logic` 不导入 `aim-core`；`aim-core` 仍只通过 gRPC 查询业务上下文。

## UserService RPC

`UserService` 属于 User/Relationship Service，只维护用户资料数据；认证凭证、密码哈希、RefreshToken 仍由 `aim-auth` 的 `user_credentials` 和 Redis 会话持有。

### UserService

```protobuf
service UserService {
  rpc CreateUserInfo(CreateUserInfoReq) returns (CreateUserInfoResp);
  rpc GetUserInfo(GetUserInfoReq) returns (GetUserInfoResp);
  rpc GetUserInfoByEmail(GetUserInfoByEmailReq) returns (GetUserInfoResp);
  rpc GetUserInfoByNickname(GetUserInfoByNicknameReq) returns (GetUserInfoResp);
  rpc UpdateUserInfoProfile(UpdateUserInfoProfileReq) returns (UpdateUserInfoProfileResp);
  rpc UpdateUserInfoStatus(UpdateUserInfoStatusReq) returns (UpdateUserInfoStatusResp);
  rpc SearchUserInfoByNickname(SearchUserInfoByNicknameReq) returns (SearchUserInfoByNicknameResp);
}
```

### 数据与查询

- 表：`app/logic/rpc/model/migrations/003_user_info.sql` 中的 `user_info(id, email, status, nickname, avatar, created_at, updated_at)`。
- 扩展：`app/logic/rpc/model/migrations/000_extensions.sql` 中的 `pg_trgm`，用于 `idx_user_info_nickname_trgm` 和昵称相似度排序。
- 查询：`app/logic/rpc/model/queries/user_info.sql`，通过 `sqlc generate` 生成到 `app/logic/rpc/model`。
- `GetUserInfoByNickname` 是内部昵称精确查询；由于 nickname 不唯一，gateway `GET /api/users/by-name/:name` 必须调用 `SearchUserInfoByNickname`，使用 `pg_trgm`/GIN 支撑的模糊查询并返回列表。gateway `GET /api/users/by-id/:id` 调用 `GetUserInfo` 返回用户详情。

### 返回码约定

- 参数错误通过 `errorx.NewCodeError(40000, ...)` 返回 gRPC `InvalidArgument`。
- 用户不存在通过 `errorx.NewCodeError(40400, "user not found")` 返回 gRPC `NotFound`。
- 邮箱或 ID 唯一约束冲突通过 `errorx.NewCodeError(40900, "user already exists")` 返回 gRPC `AlreadyExists`。
- 数据库等基础设施错误通过 `errorx.NewCodeError(50000, "internal error")` 返回 gRPC `Internal`，不泄漏底层错误细节。
- `CreateUserInfo` 与 `UpdateUserInfoProfile` 在业务层将空字符串 avatar 归一化为 `https://implement.me`，与 `user_info.avatar` 的数据库默认值一致，避免显式 `""` 覆盖 DB default。

### 边界

- `UserService` 不存储 `password_hash`，也不签发 JWT。
- `aim-logic` 不导入 `aim-core`；`aim-core` 仍只通过 gRPC 查询业务上下文。

## FriendshipService RPC

`FriendshipService` 属于 User/Relationship Service，负责用户好友关系写入；`aim-core` 的投递校验继续通过 `PermissionService` 查询关系上下文。

### FriendshipService

```protobuf
service FriendshipService {
  // AddFriend sends a friend request or returns an existing pending/accepted direct relationship.
  rpc AddFriend(AddFriendReq) returns (AddFriendResp);

  // ListFriendApplications lists pending friend applications where the current user is the receiver.
  rpc ListFriendApplications(ListFriendApplicationsReq) returns (ListFriendApplicationsResp);
}

message AddFriendReq {
  int64 user_id = 1;
  int64 friend_id = 2;
}

message FriendshipResponse {
  int64 user_id = 1;
  int64 friend_id = 2;
  string status = 3;
  int64 created_at = 4;
  int64 updated_at = 5;
}

message AddFriendResp {
  FriendshipResponse friendship = 1;
}

message ListFriendApplicationsReq {
  int64 user_id = 1; // 认证用户 ID（作为接收方查询 pending 申请）
}

message FriendApplication {
  int64 application_id = 1;
  int64 from_user_id    = 2;
  string from_nickname  = 3;
  string from_avatar    = 4;
  int64 created_at      = 5;
}

message ListFriendApplicationsResp {
  repeated FriendApplication applications = 1;
}
```

### AddFriend

- 请求：`AddFriendReq`（`user_id` 为认证用户 ID，由 gateway 从 JWT 上下文传入；`friend_id` 为 REST 路径参数 `:id`）。
- 响应：`AddFriendResp.friendship`，返回 `user_id/friend_id/status/created_at/updated_at`，时间戳为 Unix milliseconds。
- 行为：`user_id` 和 `friend_id` 必须为正数且不能相等；先通过 `UserInfoService.GetUserInfo(friend_id)` 校验目标用户存在；如双向任一关系为 `blocked`，返回 403；如正向关系已为 `pending` 或 `accepted`，幂等返回原记录，避免将 `accepted` 降级为 `pending`；否则写入 `pending` 关系。
- 数据：`app/logic/rpc/model/migrations/001_initial_permission.sql` 的 `friendships(user_id, friend_id, status, created_at, updated_at)`；查询位于 `app/logic/rpc/model/queries/friendship.sql`，通过 `sqlc generate` 生成 `UpsertFriendship`、`GetFriendshipByPair` 等方法到 `app/logic/rpc/model`。
- RPC 逻辑：`app/logic/rpc/internal/logic/friendshipservice/add_friend_logic.go`；server/client 由 `goctl rpc protoc ... -m` 生成到 `internal/server/friendshipservice` 与 `client/friendshipservice`。

### ListFriendApplications

- 请求：`ListFriendApplicationsReq`（`user_id` 为认证用户 ID，作为接收方查询 pending 申请）。
- 响应：`ListFriendApplicationsResp.applications`，返回 `application_id/from_user_id/from_nickname/from_avatar/created_at` 列表。
- 行为：查询 `friendships` 表中 `friend_id=user_id AND status=pending` 的记录，按 `created_at DESC` 排序。
- 数据：sqlc 查询 `ListPendingFriendApplications` 位于 `app/logic/rpc/model/queries/friendship.sql`。
- 错误码：40000（user_id 必须正数）、50000（基础设施错误）。

### 返回码约定

- 参数错误或自加好友：`errorx.CodeBadInput`（40000）。
- 目标用户不存在：`errorx.CodeNotFound`（40400）。
- 任一方向存在 `blocked` 关系：`errorx.CodeForbidden`（40300）。
- 数据库等基础设施错误：`errorx.CodeInternal`（50000），不泄漏底层错误细节。
