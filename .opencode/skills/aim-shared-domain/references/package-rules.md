# 包边界与依赖规则

## 概览

`app/shared` 是 AIM 的进程内共享 Go 包集合，不是微服务。这里的包可以被 `app/auth`、`app/core`、`app/gateway`、`app/logic`、`app/frontend` 引用，但不能反向依赖任何业务服务目录。

## 包地图

| 包 | 用途 | 主要使用方 |
| --- | --- | --- |
| `errorx` | `CodeError`、业务错误码、gRPC/HTTP 映射 | 全模块 |
| `rpc` | gRPC unary interceptor：panic 恢复、错误清洗、trace 记录 | auth/core/logic/gateway rpc |
| `nacos` | Nacos v2 注册、注销、gRPC resolver | gateway/auth/core/logic |
| `jwt` | HS256 access token 签发/验证，claims 含 user_id/device_id | auth 签发，gateway 验证 |
| `events` | Kafka 事件契约，如 `UserCreatedEvent` | auth 生产，logic 消费 |
| `tracing` | W3C trace context 经 Kafka payload 传播 | auth/core/logic |
| `tools` | Snowflake ID 生成器 | auth/core |
| `moderation` | 内容审核 `Checker` 接口与 noop 实现 | core/ai 进程内调用 |

## 规则

- 新增 shared 包前确认它是横切能力；只被单一服务使用的代码留在该服务目录。
- `app/shared/*` 不得 import `app/auth`、`app/core`、`app/gateway`、`app/logic`、`app/frontend`。
- 共享包只暴露小接口或稳定 DTO；不要把服务级 `ServiceContext`、go-zero config、生成的 pb client 泄漏进 shared。
- 业务错误统一走 `errorx.NewCodeError`；跨 gRPC 边界用 `errorx.FromGRPCError` 还原并清洗基础设施错误。
- Kafka 事件结构需要携带 `tracing.TraceContextFields` 时直接嵌入字段，避免依赖 Kafka header。
- Nacos resolver 注册是进程级全局动作；每个 `nacos:///<service>` target 必须按 endpoint 独立订阅，避免服务串线。

## 修改检查

```bash
go test ./app/shared/...
go test ./app/auth/... ./app/core/... ./app/gateway/... ./app/logic/...
golangci-lint run ./app/shared/...
```