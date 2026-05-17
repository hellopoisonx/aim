# 接口定义

## TransferService RPC

Proto 定义：`app/core/rpc/core.proto`，生成的 pb 代码在 `app/core/rpc/pb/`。

### TransferService

```protobuf
service TransferService {
  // Transfer 将消息路由到核心服务进行投递
  // 网关调用此 RPC 将用户发送的消息交给核心处理
  rpc Transfer(TransferReq) returns (TransferResp);
}

// TransferReq 消息发送请求
message TransferReq {
  int64  sender_id       = 1; // 发送者用户 ID
  string device_id       = 2; // 设备 ID（用于设备维度去重和限流）
  int64  conversation_id = 3; // 会话 ID
  string message_type    = 4; // 消息类型：text/image/file/voice/video等
  string content         = 5; // 消息内容
  string client_msg_id   = 6; // 客户端消息 ID（用于去重和幂等）
  repeated string mentions = 7; // 被 @ 的用户 ID 列表
}

// TransferResp 消息发送响应
message TransferResp {
  int64  message_id    = 1; // 服务端分配的消息 ID
  string client_msg_id = 2; // 客户端消息 ID（回传用于确认）
  int64  accepted_at   = 3; // 消息被服务端接受的时间戳（Unix 毫秒）
}
```

### 返回码约定

- `code=0`：成功，返回 message_id
- `code>0`：业务错误码（40000/40100/40300/40400/42900）
- 基础设施错误（Internal/Unavailable）映射为 `code=50000`，客户端应重试

## GatewayService（调用方）

aim-core 的 Delivery Consumer 和 Presence Service 调用 `shared/proto/gateway/gateway.proto` 定义的 GatewayService：

Proto 定义：`shared/proto/gateway/gateway.proto`，生成的 pb 代码在 `shared/proto/gateway/pb/`。

| RPC | 用途 |
| --- | --- |
| `PushMessage(PushMessageReq) returns (PushMessageResp)` | 推送聊天消息到目标用户 |
| `PushPresence(PushPresenceReq) returns (PushPresenceResp)` | 推送用户在线状态变更 |
| `KickUser(KickUserReq) returns (KickUserResp)` | 踢出用户连接 |
| `DrainNotify(DrainNotifyReq) returns (DrainNotifyResp)` | 通知网关节点即将关闭 |

## PermissionService（调用方）

aim-core 的 Transfer Service 调用 `app/logic/rpc/logic.proto` 定义的 PermissionService，查询消息进入 Kafka 前所需的业务上下文。

| RPC | 用途 |
| --- | --- |
| `CheckMessagePermission(CheckMessagePermissionReq) returns (CheckMessagePermissionResp)` | 校验发送者是否可以向会话发送消息，包括会话成员、好友/黑名单、群成员禁言等 logic 域规则 |
