# 包边界与依赖规则

## 概览

`app/shared` 是 AIM 的进程内共享 Go 包集合，不是微服务。这里的包可以被 `app/auth`、`app/core`、`app/gateway`、`app/logic` 以及独立客户端模块引用，但不能反向依赖任何业务服务目录。

## 包地图

| 包 | 用途 | 主要使用方 |
| --- | --- | --- |
| `errorx` | `CodeError`、业务错误码、gRPC/HTTP 映射 | 全模块 |
| `rpc` | gRPC unary interceptor：panic 恢复、错误清洗、trace 记录 | auth/core/logic/gateway rpc |
| `discov (go-zero)` | etcd 注册/发现（zrpc 内置，`etcd://` scheme） | gateway/auth/core/logic/attachment |
| `jwt` | HS256 access token 签发/验证，claims 含 user_id/device_id | auth 签发，gateway 验证 |
| `events` | Kafka 事件契约，如 `UserCreatedEvent`、`AttachmentUploadedEvent` | auth/attachment 生产，logic/data_parsing 消费 |
| `attachment` | 附件内容 schema、JSON 解析与校验 | gateway/core/logic |
| `s3signer` | SeaweedFS S3 预签名工具 | attachment/data_parsing |
| `tracing` | W3C trace context 经 Kafka payload 传播；Kafka producer/consumer span 与错误标记 helper | auth/core/logic/gateway/attachment/data_parsing |
| `tools` | Snowflake ID 生成器 | auth/core |
| `quota` | Redis ZSET 滑动窗口限流；`Allow` / `AllowPair` / `Enabled` 三入口 | gateway |
| `moderation` | 内容审核 `Checker` 接口与 noop 实现 | core/ai 进程内调用 |
| `outbox` | 事务发件箱模式：DB 事务内写事件、后台 Poller 投递到 Kafka，保证至少一次投递 | logic（群管理事件）/ auth（用户创建事件）

## 规则

- 新增 shared 包前确认它是横切能力；只被单一服务使用的代码留在该服务目录。
- `app/shared/*` 不得 import `app/auth`、`app/core`、`app/gateway`、`app/logic` 或客户端实现目录。
- 共享包只暴露小接口或稳定 DTO；不要把服务级 `ServiceContext`、go-zero config、生成的 pb client 泄漏进 shared。
- 业务错误统一走 `errorx.NewCodeError`；跨 gRPC 边界用 `errorx.FromGRPCError` 还原并清洗基础设施错误。
- Kafka 事件结构需要携带 `tracing.TraceContextFields` 时直接嵌入字段，避免依赖 Kafka header。
- Kafka producer 应先创建 producer span，再调用 `tracing.InjectTraceContext(ctx)` 写入事件 payload，确保消费侧 span 以 producer span 为父级。
- 服务注册通过 `zrpc.RpcServerConf.Etcd` 字段；`zrpc.MustNewServer` 启动时调用 `internal.NewRpcPubServer` 自动 keepalive（lease 10s，watch 续期），进程退出由 `proc.AddWrapUpListener` 自动撤销 lease。客户端通过 `zrpc.MustNewClient(c.<X>Rpc)` 或 `zrpc.NewClient(c.<X>Rpc)` 创建，target 形如 `etcd://<hosts>/<key>`。YAML 只需加 `Etcd: { Hosts, Key }` 块，无需任何业务代码改动。参考 https://go-zero.dev/guides/microservice/service-discovery/。


## 修改检查

```bash
go test ./app/shared/...
go test ./app/auth/... ./app/core/... ./app/gateway/... ./app/logic/...
golangci-lint run ./app/shared/...
```