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
