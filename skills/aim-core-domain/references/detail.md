# 模块需求定义

只负责一件事：把消息送到对的人。

- Transfer Service（消息路由）：消息流向判断（单聊/群聊）、查询接收方所在网关节点、投递至 Kafka
- Delivery Consumer（投递消费者）：从 Kafka 消费消息，查找目标用户所在网关并投递
- Presence Consumer：消费 `aim.presence.events`，查好友列表 → 查 `aim:user_gateway:{friend_id}` → 调 `gateway.PushPresence`
- Typing Consumer：消费 `aim.typing.events`，查会话成员 → 查 `aim:user_gateway:{member_id}` → 调 `gateway.PushTyping`
- GatewayRouter：按 `node_id` 路由 gRPC 请求到目标网关实例。Gateway 节点注册走 etcd（`Etcd.Key: gateway.rpc`），但 client 端不依赖 etcd 发现 —— 走 `GatewayRpcConf.Target` / `GatewayRpcConf.Nodes` 静态配置，部署期在 YAML 写死 `node_id` → `target` 映射。

## 关键修复：PresenceConsumer 推送寻址

**Bug 背景**：`PushPresenceReq` 最初只有 `user_id`（状态变更者的 userId），`GatewayServer.PushPresence` 用 `req.UserId` 查 Manager 连接。由于状态变更者本人不在目标好友的网关节点上，推送永远到达不了目标客户端。

**根因**：proto 缺少区分"谁变了状态"和"推送给谁"这两个语义的字段。

**修复**：
1. `PushPresenceReq` 新增 `target_user_id` 字段（proto 字段号 4）
2. `PresenceConsumer` 遍历好友时，对每个 `friendID` 设置 `TargetUserId = friendID`
3. `GatewayServer.PushPresence` 改用 `TargetUserId` 查连接；兼容 `TargetUserId == 0`（旧版 caller）时回退到 `UserId`

**测试覆盖**：新增 `TestGatewayServerPushPresenceFallbackToUserId` 验证回退路径。

## TransferService RPC 契约

Proto 定义：`app/core/rpc/core.proto`，生成的 pb 代码在 `app/core/rpc/pb/`。

```protobuf
service TransferService {
  rpc Transfer(TransferReq) returns (TransferResp);
}

message TransferReq {
  int64  sender_id       = 1; // 发送者用户 ID
  string device_id       = 2; // 设备 ID（用于设备维度去重和限流）
  int64  conversation_id = 3; // 会话 ID
  string message_type    = 4; // 消息类型：text/image/file/voice/video等
  string content         = 5; // 消息内容
  string client_msg_id   = 6; // 客户端消息 ID（用于去重和幂等）
  repeated string mentions = 7; // 被 @ 的用户 ID 列表
}

message TransferResp {
  int64  message_id    = 1; // 服务端分配的消息 ID
  string client_msg_id = 2; // 客户端消息 ID（回传用于确认）
  int64  accepted_at   = 3; // 消息被服务端接受的时间戳（Unix 毫秒）
}
```

### Transfer 职责

1. **验证发送者资格**：调用 aim-logic `PermissionService.CheckMessagePermission` 查询发送者是否为会话成员、是否被禁言/拉黑
2. **配额检查**：通过 Redis 滑动窗口实现实时限流
3. **内容审核**：同步调用 in-process 内容审核库（硬审核同步拒绝，软审核异步记录）
4. **消息去重**：基于 `(sender_id, device_id, client_msg_id)` 做幂等，重复请求返回已有 message_id
5. **生成 message_id**：全局唯一 ID 生成器（snowflake 或 uuid）
6. **Kafka 发布**：发布消息到 Kafka，key = `conversation_id`（保证同会话消息有序），事件 payload 必须携带 `traceparent`/`tracestate`，供 `DeliveryConsumer` 和 logic `ArchiveConsumer` 恢复 Kafka 链路追踪。
7. **等待 Kafka 确认**：TransferResp 只在 Kafka 接受消息后返回

### gRPC 错误 → biz code 映射

| gRPC 状态码 | biz code | 含义 | 可重试 |
| --- | --- | --- | --- |
| `InvalidArgument` | `40000` | 参数校验失败 | 否 |
| `Unauthenticated` | `40100` | 身份未认证 | 否 |
| `PermissionDenied` | `40300` | 无权限（禁言/拉黑/非成员） | 否 |
| `NotFound` | `40400` | 会话不存在 | 否 |
| `ResourceExhausted` | `42900` | 配额/限流 | 否 |
| `Internal` / `Unavailable` / `DataLoss` | `50000` | 基础设施错误 | 是 |

网关根据 gRPC status code 映射到 WebSocket `AckStatus`（REJECTED / RETRYABLE）。

### 依赖方向

- aim-core → aim-logic（单向）：core 通过 gRPC 查询 logic 的好友/群组关系（Redis TTL 缓存），logic 不反向依赖 core
- aim-core → aim-gateway：core 通过 gRPC 调用 GatewayService 推送消息到客户端；当前客户端使用自定义 raw gRPC client，必须保留 unary client interceptor 注入 W3C trace metadata，避免绕过 go-zero zrpc tracing。

### 部署配置

- 服务注册：`app/core/rpc/core.go` 启动时 `zrpc.MustNewServer(c.RpcServerConf, ...)` 自动调用 `internal.NewRpcPubServer` 把 `Etcd.Key: core.rpc` + `ListenOn: 0.0.0.0:8080` 注册到 etcd（由 `figureOutListenOn` 自动把 `0.0.0.0` 解析为容器 IP / `POD_IP`）；`app/core/rpc/etc/core.yaml` 的 `Etcd` 块维护 `Hosts` / `Key` 两个字段。
- Docker Compose 配置：分层 Compose 通过 `${AIM_CONFIG_DIR:-../config/local}/core.yaml` 挂载到 `/app/etc/core.yaml`，本地默认配置副本位于 `deploy/config/local/core.yaml`；`app/core/rpc/etc/core.yaml` 保留给本地 `go run` / 单服务调试。Compose 内 `ListenOn` 为 `0.0.0.0:8080`，`Etcd.Hosts` 为 `[etcd:2379]`，`Etcd.Key` 为 `core.rpc`，业务缓存 Redis 使用 `CacheRedis.Addr: redis:6379`（不要命名为 `Redis`，该字段名会与 `zrpc.RpcServerConf.Redis` 冲突），Kafka 使用 `kafka:9092`，`GatewayRpc.Target` 为 `aim-gateway:9091`，`LogicRpc` 通过 etcd 服务发现（`etcd://etcd:2379/logic.rpc`）。本地 dev overlay 映射主机端口 `8081:8080`。
- go-zero OTel/Tempo：`app/core/rpc/etc/core.yaml` 的 `Telemetry` 块使用 `Name: core.rpc`、`Batcher: otlphttp`、`Endpoint: tempo:4318`、`OtlpHttpPath: /v1/traces`，由 `zrpc.RpcServerConf` 自动接入 RPC tracing。
- 配置加载测试：`app/core/rpc/internal/config/config_test.go` 覆盖 `Telemetry`、`CacheRedis`、Kafka producer 和 Etcd client / `LogicRpc.Etcd.Key` / `AttachmentRpc.Etcd.Key` 配置。
- Kafka tracing：`TransferLogic` 发布到 `aim-message-transfer` 的 JSON payload 包含 `traceparent`/`tracestate`；`DeliveryConsumer` 恢复 trace context 并创建 `core.kafka.delivery.consume` consumer span，然后用恢复后的 context 调用 GatewayService。

## ConversationEventConsumer

### 概述

`ConversationEventConsumer` 消费 `aim.conversation.events` topic，将 logic 生产的群变更系统消息推送给 Gateway，再由 Gateway 通过 WebSocket 推送给客户端。

实现位置：`app/core/rpc/internal/mqs/conversation_event_consumer.go`。

### 事件格式

```json
{
  "traceparent": "00-...",
  "tracestate": "",
  "message_id": 123,
  "conversation_id": 456,
  "sender_id": 0,
  "message_type": "system",
  "content": "{\"event\":\"member_removed\",...}",
  "target_user_ids": [101, 102, 103],
  "timestamp": 1234567890
}
```

### 处理流程

1. 反序列化 Kafka value 为 `conversationEvent`
2. 恢复 W3C trace context（`traceparent`/`tracestate`）
3. 创建 `core.kafka.conversation_event.consume` consumer span
4. 遍历 `target_user_ids`，对每个用户调用 `GatewayClient.PushMessage`（`is_system=true`）
5. 任一推送失败记录 `span.RecordError` 并返回错误

### 配置

`app/core/rpc/internal/config/config.go` 新增：

```go
type Config struct {
    // ... 现有字段 ...
    ConversationEventConsumerConf kq.KqConf `json:",optional"`
}
```

`app/core/rpc/internal/mqs/consumers.go` 条件注册：仅当 `ConversationEventConsumerConf` 配置了 Brokers 和 Topic 时才启动 consumer。

`app/core/rpc/etc/core.yaml` 需添加对应 Kafka consumer 配置块；当前 docker compose 配置已启用：

```yaml
ConversationEventConsumerConf:
  Name: core-conversation-event-consumer
  Brokers:
    - kafka:9092
  Group: aim-core-conversation-events
  Topic: aim.conversation.events
  Offset: first
  Consumers: 1
  Processors: 1
```

### 关键设计决策

- logic 在事务内预先计算 `target_user_ids`，core 无需再次查询成员列表，避免二次 DB 查询
- `sender_id=0`、`message_type=system` 标识系统消息，前端可据此区分展示
- 推送失败不重试，依赖客户端拉取历史时可见（消息已持久化到 DB）

## AttachmentParsedConsumer

### 概述

`AttachmentParsedConsumer` 消费 `aim.attachment.parsed` topic，将 data_parsing 解析完成的附件更新（缩略图、尺寸、时长等）以系统消息形式推送给会话所有成员。

实现位置：`app/core/rpc/internal/mqs/attachment_parsed_consumer.go`。

### 事件格式

```json
{
  "traceparent": "00-...",
  "tracestate": "",
  "file_id": "uuid",
  "kind": "image",
  "parse_status": "ready",
  "thumbnail_object_key": "attachments/derived/{file_id}/thumbnail.png",
  "thumbnail_file_id": "",
  "duration_ms": 0,
  "width": 1920,
  "height": 1080,
  "metadata": {"format": "png"},
  "error": "",
  "parsed_at": 1234567890
}
```

### 处理流程

1. 反序列化 Kafka value，恢复 W3C trace context
2. 仅处理 `parse_status=ready` 的事件（失败事件暂不推送）
3. 调用 `AttachmentClient.GetFile` 获取完整文件信息（含 `conversation_id`、原始名称、mime 等）
4. 构建更新后的 `Content` JSON（schema=aim.attachment.v1，含缩略图、尺寸等）
5. 通过 `LogicConversationClient.GetConversationMembers` 查询会话所有成员
6. 生成 Snowflake 系统消息 ID
7. 遍历成员 → 查网关节点 → 通过 `GatewayClient.PushMessage` 推送（`is_system=true`）
8. 推送失败记录 span error 并返回错误（依赖 Kafka 重试）

### 配置

`app/core/rpc/internal/config/config.go` 新增：

```go
type Config struct {
    // ... 现有字段 ...
    AttachmentParsedConsumerConf kq.KqConf `json:",optional"`
}
```

`app/core/rpc/etc/core.yaml`：

```yaml
AttachmentParsedConsumerConf:
  Name: core-attachment-parsed-consumer
  Brokers:
    - kafka:9092
  Group: aim-core-attachment-parsed
  Topic: aim.attachment.parsed
  Offset: first
  Consumers: 1
  Processors: 1
```

### 关键设计决策

- 消费端通过 `AttachmentClient.GetFile(user_id=0)` 获取文件元数据（利用 `OR f.status='uploaded'` 条件），避免在事件 payload 中冗余携带原始文件信息
- `message_type=event.Kind`（`image`/`video`/`audio`）使客户端能直接匹配到对应的附件消息类型
- `is_system=true` 标识为系统消息，客户端可据此与用户发送的原始消息区分展示
- 仅推送 `parse_status=ready` 的事件；失败事件依赖客户端通过 `GET /api/attachments/{id}` 轮询感知
- 推送消息中 `file_id` 保持不变，客户端可通过 `file_id` 匹配并原地更新已有附件消息的展示（缩略图、尺寸等）