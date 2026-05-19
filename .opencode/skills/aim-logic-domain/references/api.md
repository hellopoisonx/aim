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
goctl rpc protoc app/logic/rpc/logic.proto --go_out=app/logic/rpc --go-grpc_out=app/logic/rpc --zrpc_out=app/logic/rpc --style go_zero
```

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

- 表：`app/logic/rpc/sql/migrations/003_user_info.sql` 中的 `user_info(id, email, status, nickname, avatar, created_at, updated_at)`。
- 扩展：`app/logic/rpc/sql/migrations/000_extensions.sql` 中的 `pg_trgm`，用于 `idx_user_info_nickname_trgm` 和昵称相似度排序。
- 查询：`app/logic/rpc/sql/queries/user_info.sql`，通过 `sqlc generate` 生成到 `app/logic/rpc/internal/model`。
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
