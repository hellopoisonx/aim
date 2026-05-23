# 接口定义

只暴露内部 gRPC 接口， 不含任何 REST API。

```protobuf
syntax = "proto3";

package auth;

option go_package = "github.com/hellopoisonx/aim/shared/proto/auth;pb";

// ============================================================
// 认证服务
// ============================================================

// AuthService — aim-auth 认证服务，对外暴露 gRPC 接口。
// 提供用户注册、登录、Token 管理、权限验证等功能。
service AuthService {
  // Register 用户注册（创建账号）。
  // 调用方：aim-gateway 客户端注册请求。
  // 写入本地用户凭证表后推送 `UserRegisteredEvent` 到消息队列。
  rpc Register(RegisterReq) returns (RegisterResp);

  // Login 用户登录（颁发 Token）。
  // 调用方：aim-gateway 客户端登录请求。
  rpc Login(LoginReq) returns (LoginResp);

  // RefreshToken 刷新 Access Token。
  // 调用方：aim-gateway 客户端 Token 过期时自动刷新。
  // Refresh Token 和 Access Token 都需要轮换
  rpc RefreshToken(RefreshTokenReq) returns (RefreshTokenResp);

  // Logout 用户登出（注销 Token）。
  // 调用方：aim-gateway 客户端主动登出或多端登录踢设备。
  rpc Logout(LogoutReq) returns (LogoutResp);
}

// ============================================================
// 用户注册
// ============================================================

message RegisterReq {
  string email           = 1; // 邮箱（唯一标识）
  string password       = 2; // 密码（已哈希）
  string username       = 3; // 用户名（昵称，必填）
  string avatar          = 4; // 用户头像 URL
  string device_id      = 5; // 设备 ID
}

message RegisterResp {
  int64  user_id        = 1; // 新注册用户 ID
}

// ============================================================
// 用户登录
// ============================================================

message LoginReq {
  string email     = 1; // 邮箱
  string password  = 2; // 密码（已哈希）
  string device_id = 3; // 设备 ID
}

message LoginResp {
  int64  user_id        = 1; // 登录用户 ID
  string access_token   = 2; // JWT Access Token
  string refresh_token  = 3; // UUID Refresh Token
  int64  expires_at     = 4; // Access Token 过期时间戳（Unix 秒）
}

// ============================================================
// 刷新 Token
// ============================================================

message RefreshTokenReq {
  string refresh_token = 1; // UUID Refresh Token
}

message RefreshTokenResp {
  string access_token   = 1; // 新 JWT Access Token
  string refresh_token  = 2; // 新 UUID Refresh Token（Token 轮转）
  int64  expires_at     = 3; // Access Token 过期时间戳（Unix 秒）
}

// ============================================================
// 用户登出
// ============================================================

message LogoutReq {
  int64  user_id       = 1; // 登出用户 ID
  string device_id     = 2; // 设备 ID（空字符串 = 所有设备）
}

message LogoutResp {
  bool success = 1; // 登出是否成功
}
```

## 当前实现

- 当前代码文件：`app/auth/rpc/auth.proto`。
- 当前生成路径：`app/auth/rpc/pb` 与 `app/auth/rpc/authservice`。
- 为避免 goctl 在仓库内生成冗余 `github.com/hellopoisonx/aim/...` 路径，当前实现使用 module-relative 生成方式；重新生成时从仓库根执行：

```bash
goctl rpc protoc app/auth/rpc/auth.proto --go_out=app/auth/rpc --go-grpc_out=app/auth/rpc --zrpc_out=app/auth/rpc --style go_zero
```

- 重新生成后必须确认 `app/auth/rpc/pb/auth_grpc.pb.go` 的 `Metadata` 是 `app/auth/rpc/auth.proto`，避免 protobuf descriptor 与其他 `auth.proto` 冲突。
