# 包边界与依赖规则

## 概览

`app/shared` 是 AIM 的进程内共享 Go 包集合，不是微服务。这里的包可以被 `app/auth`、`app/core`、`app/gateway`、`app/logic` 以及独立客户端模块引用，但不能反向依赖任何业务服务目录。

## 包地图

| 包 | 用途 | 主要使用方 |
| --- | --- | --- |
| `attachment` | 附件 content schema、JSON 解析与校验（`ParseContent`、`ParseStatus*`） | attachment/data_parsing/core/logic |
| `attachmentstore` | `aim_attachment` 库的 GORM `orm` 模型与 `dao` 查询，两个进程共用 | attachment/data_parsing |
| `botperm` | Bot OpenAPI 的 action 级权限判定 | gateway bot 接口、logic bot service |
| `bottoken` | Bot token 生成、SHA-256 哈希与常数时间比较 | gateway、logic |
| `cache` | 两级缓存 `CacheManager`/`TypedCache`、Redis Streams 失效广播、key 前缀构造 | core/gateway/logic |
| `conversation` | 会话类型常量、系统发送者、直聊 peer 校验 | core/gateway/logic |
| `errorx` | `CodeError`、业务错误码、gRPC/HTTP 映射 | 全模块 |
| `events` | RocketMQ 事件契约，每个契约的生产/消费方见下表 | 见下表 |
| `gormdb` | 统一的 GORM Postgres 连接、错误翻译与连接池 | attachment/auth/data_parsing/logic |
| `jwt` | HS256 access token 签发/验证，claims 含 user_id/device_id | auth 签发，gateway 验证 |
| `pgtypes` | JSONB 列的 GORM 扫描/写入类型（替代 `gorm.io/datatypes`） | attachment/data_parsing/logic |
| `quota` | Redis ZSET 滑动窗口限流；`Allow` / `AllowPair` / `Enabled` 三入口 | gateway |
| `rocketmq` | RocketMQ 5 gRPC 封装：`Producer`/`ConsumerService`/`SendTransactional` + `event_journal` 回查（取代发件箱轮询） | auth/logic/attachment/core/gateway/data_parsing |
| `rpc` | gRPC unary interceptor（panic 恢复、错误清洗、trace 记录）与 `Run` 统一启动 | attachment/auth/core/logic |
| `s3signer` | SeaweedFS S3 预签名工具 | attachment/data_parsing |
| `tools` | Snowflake ID 生成器 | auth/core/logic |
| `tracing` | W3C trace context 经消息 payload 传播；MQ producer/consumer span（`StartMQConsumerSpan`/`StartMQProducerSpan`）与错误标记 helper | auth/core/logic/gateway/attachment/data_parsing |

`app/shared` 里没有内容审核包；仓库当前也没有任何审核实现。

事件契约的收发方：

| 契约 | 生产方 | 消费方 |
| --- | --- | --- |
| `UserCreatedEvent` | auth（`user_registration.go`） | logic（`user_created_consumer.go`） |
| `AttachmentUploadedEvent` | attachment（`upload.go`） | data_parsing（`worker.go`） |
| `AttachmentParsedEvent` | data_parsing（`worker.go`） | core（`attachment_parsed_consumer.go`） |
| `TransferEvent` | logic（`message_deps.go`） | core（`delivery_consumer.go`）、logic（`archive_consumer.go`、`bot_webhook_consumer.go`） |
| `ConversationEvent` | logic（`conversation_service.go`） | core（`conversation_event_consumer.go`） |
| `FriendApplicationEvent` | gateway（好友申请/接受/拒绝逻辑） | core（`friend_application_consumer.go`） |
| `PushEvent` | core（`delivery_consumer.go`） | gateway（`push_consumer.go`） |

## 规则

- 新增 shared 包前确认它是横切能力；只被单一服务使用的代码留在该服务目录。
- `app/shared/*` 不得 import `app/auth`、`app/core`、`app/gateway`、`app/logic` 或客户端实现目录。
- 共享包只暴露小接口或稳定 DTO；不要把服务级 `ServiceContext`、go-zero config、生成的 pb client 泄漏进 shared。
- 业务错误统一走 `errorx.NewCodeError`；跨 gRPC 边界用 `errorx.FromGRPCError` 还原并清洗基础设施错误。
- RocketMQ 事件结构需要携带 `tracing.TraceContextFields` 时直接嵌入字段，避免依赖 RocketMQ header。
- RocketMQ producer 应先创建 producer span，再调用 `tracing.InjectTraceContext(ctx)` 写入事件 payload，确保消费侧 span 以 producer span 为父级。
- 服务注册通过 `zrpc.RpcServerConf.Etcd` 字段；`zrpc.MustNewServer` 启动时调用 `internal.NewRpcPubServer` 自动 keepalive（lease 10s，watch 续期），进程退出由 `proc.AddWrapUpListener` 自动撤销 lease。客户端通过 `zrpc.MustNewClient(c.<X>Rpc)` 或 `zrpc.NewClient(c.<X>Rpc)` 创建，target 形如 `etcd://<hosts>/<key>`。YAML 只需加 `Etcd: { Hosts, Key }` 块，无需任何业务代码改动。参考 https://go-zero.dev/guides/microservice/service-discovery/。


## 修改检查

```bash
go test ./app/shared/...
go test ./app/auth/... ./app/core/... ./app/gateway/... ./app/logic/...
golangci-lint run ./app/shared/...
```