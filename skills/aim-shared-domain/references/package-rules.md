# 包边界与依赖规则

## 概览

`app/shared` 是 AIM 的进程内共享 Go 包集合，不是微服务。这里的包可以被 `app/auth`、`app/core`、`app/gateway`、`app/logic` 以及独立客户端模块引用，但不能反向依赖任何业务服务目录。

## 包地图

| 包 | 用途 | 主要使用方 |
| --- | --- | --- |
| `errorx` | `CodeError`、业务错误码、gRPC/HTTP 映射 | 全模块 |
| `rpc` | gRPC unary interceptor：panic 恢复、错误清洗、trace 记录 | auth/core/logic/gateway rpc |
| `nacos` | Nacos v2 注册、注销、gRPC resolver | gateway/auth/core/logic |
| `jwt` | HS256 access token 签发/验证，claims 含 user_id/device_id | auth 签发，gateway 验证 |
| `events` | Kafka 事件契约，如 `UserCreatedEvent`、`AttachmentUploadedEvent` | auth/attachment 生产，logic/data_parsing 消费 |
| `attachment` | 附件内容 schema、JSON 解析与校验 | gateway/core/logic |
| `s3signer` | SeaweedFS S3 预签名工具 | attachment/data_parsing |
| `tracing` | W3C trace context 经 Kafka payload 传播；Kafka producer/consumer span 与错误标记 helper | auth/core/logic/gateway/attachment/data_parsing |
| `tools` | Snowflake ID 生成器 | auth/core |
| `moderation` | 内容审核 `Checker` 接口与 noop 实现 | core/ai 进程内调用 |

## 规则

- 新增 shared 包前确认它是横切能力；只被单一服务使用的代码留在该服务目录。
- `app/shared/*` 不得 import `app/auth`、`app/core`、`app/gateway`、`app/logic` 或客户端实现目录。
- 共享包只暴露小接口或稳定 DTO；不要把服务级 `ServiceContext`、go-zero config、生成的 pb client 泄漏进 shared。
- 业务错误统一走 `errorx.NewCodeError`；跨 gRPC 边界用 `errorx.FromGRPCError` 还原并清洗基础设施错误。
- Kafka 事件结构需要携带 `tracing.TraceContextFields` 时直接嵌入字段，避免依赖 Kafka header。
- Kafka producer 应先创建 producer span，再调用 `tracing.InjectTraceContext(ctx)` 写入事件 payload，确保消费侧 span 以 producer span 为父级。
- Nacos resolver 注册是进程级全局动作；`RegisterResolver` 内部使用 `sync.Once`，**只首次调用生效**。
  后续 `RegisterResolver` 调用被忽略，仅第一个 namingClient 被使用。
  因此第一个注册的 naming client 必须能 Nacos 查到所有后续 `aimnacos:///<service>` target 对应的服务。
  gateway 中 auth/core/logic/attachment 多个 `NewClientWithTarget(nacos.BuildTarget(serviceName))` 均使用相同的 resolver 实例。
  如需每个 resolver 使用独立的 naming client，需重构 `registerOnce` 逻辑（如按 naming client 分 scheme 名）。
- AIM 自定义 Nacos gRPC resolver 的 scheme 必须使用 `aimnacos`，不要使用 `nacos`。
  Nacos SDK 会在内部直连 `nacos:9848`；如果自定义 resolver 抢占全局 `nacos` scheme，grpc-go 会把 `9848` 当成服务名反复触发
  `nacos initial SelectInstances for "9848": instance list is empty!`。
  看到该日志持续重复时，优先确认镜像是否仍运行旧代码并重建相关服务。
  服务真实空实例日志应显示业务服务名（如 `auth.rpc`、`logic.rpc`），并只代表启动期订阅尚未收到健康实例。

## 修改检查

```bash
go test ./app/shared/...
go test ./app/auth/... ./app/core/... ./app/gateway/... ./app/logic/...
golangci-lint run ./app/shared/...
```