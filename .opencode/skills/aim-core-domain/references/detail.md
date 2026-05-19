# 模块需求定义

只负责一件事：把消息送到对的人。

- Transfer Service（消息路由）：消息流向判断（单聊/群聊）、查询接收方所在网关节点、投递至 Kafka
- Presence Service（在线状态）：Redis heartbeat 维护用户在线/离线/输入中状态，向好友推送状态变更
- Delivery Consumer（投递消费者）：从 Kafka 消费消息，查找目标用户所在网关并投递

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

- 服务注册：`app/core/rpc/core.go` 启动时通过 `app/shared/nacos` 注册 Nacos v2 临时实例；`app/core/rpc/etc/core.yaml` 的 `Nacos` 块维护 `ServerAddr`、`Group`、`Cluster`、`ServiceName`、`AdvertiseIP`、`AdvertisePort` 等注册参数。
- Docker Compose 配置：`app/core/rpc/etc/core.yaml` 面向 `docker-compose.yaml` 内部网络，`ListenOn` 为 `0.0.0.0:8080`，`Nacos.ServerAddr` 为 `nacos:8848`，`Nacos.AdvertiseIP` 为 `aim-core`，业务缓存 Redis 使用 `CacheRedis.Addr: redis:6379`（不要命名为 `Redis`，该字段名会与 `zrpc.RpcServerConf.Redis` 冲突），Kafka 使用 `kafka:9092`，`GatewayRpc.Target` 为 `aim-gateway:9090`，`LogicRpc.ServerAddr` 为 `nacos:8848` 通过 Nacos 发现 `logic.rpc`。容器映射主机端口 `8081:8080`。
- go-zero OTel/Jaeger：`app/core/rpc/etc/core.yaml` 的 `Telemetry` 块使用 `Name: core.rpc`、`Batcher: otlphttp`、`Endpoint: jaeger:4318`、`OtlpHttpPath: /v1/traces`，由 `zrpc.RpcServerConf` 自动接入 RPC tracing。
- 配置加载测试：`app/core/rpc/internal/config/config_test.go` 覆盖 `Telemetry`、`CacheRedis`、Kafka producer 和 Nacos client 配置。
- Kafka tracing：`TransferLogic` 发布到 `aim-message-transfer` 的 JSON payload 包含 `traceparent`/`tracestate`；`DeliveryConsumer` 恢复 trace context 并创建 `core.kafka.delivery.consume` consumer span，然后用恢复后的 context 调用 GatewayService。
