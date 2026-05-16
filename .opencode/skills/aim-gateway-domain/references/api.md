# 接口定义

## REST API - `/api`

### JWT 鉴权

- Headers.Authorization: Bearer <Token>
- payload: user_id && device_id

### 代理转发  `auth` - `/auth`

- `/register` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**
- `/login` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**
- `/logout` 解析 JWT payload 填入 gRPC 请求参数 **需要鉴权**
- `/refresh` 参考 `aim-auth-domain` 中的 gRPC api 定义 **无需鉴权**

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

  // PushPresence 推送用户在线状态变更通知给目标用户的好友。
  // 调用方：aim-core/Presence Service 检测到用户状态变更后，通过此方法推送到相关网关。
  rpc PushPresence(PushPresenceReq) returns (PushPresenceResp);

  // KickUser 踢下线指定用户设备。
  // 调用方：aim-auth 多端登录策略触发 / 管理员后台操作。
  rpc KickUser(KickUserReq) returns (KickUserResp);

  // DrainNotify 通知网关节点进行优雅迁移（会话 drain）。
  // 调用方：Nacos 服务发现 / 运维管理工具，在网关节点下线前通知其推送 reconnect 帧给客户端。
  rpc DrainNotify(DrainNotifyReq) returns (DrainNotifyResp);
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
```

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
  FRAME_TYPE_PUSH_NOTIFICATION  = 103; // 推送系统通知
  FRAME_TYPE_PUSH_TYPING       = 104; // 推送输入状态
  FRAME_TYPE_RECONNECT        = 105; // 网关要求重连（drain 窗口）
  FRAME_TYPE_SERVER_ACK       = 106; // 服务端确认
}

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