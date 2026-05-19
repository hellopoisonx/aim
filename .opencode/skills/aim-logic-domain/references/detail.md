# 模块需求定义

提供消息投递的判断依据和业务逻辑支撑。

- User/Relationship Service（用户与社交）：持有用户资料表、好友申请、黑名单、群组元数据、群成员禁言/转让
- Message Archive Service（消息持久化）：从 Kafka 异步消费消息，写入 PostgreSQL 分区表 + JSONB；提供 tsvector 全文搜索、历史回溯接口
- Billing & Quota Service（计费管理）：平台点数扣费、计费流水审计（PostgreSQL 持久化）；实时限额通过 Redis 滑动窗口实现，避免 PostgreSQL 往返延迟
- Content Moderation（内容审核）：作为共享库（in-process）供 aim-core 和 aim-ai 同步调用；异步审计日志由独立 worker 处理

## 当前落地范围

- `app/logic/rpc/logic.proto` 定义 `PermissionService` 与 `UserService`。
- `CheckMessagePermission` 是 aim-core 在接受消息进入 Kafka 前查询的业务上下文接口。
- 当前实现通过 `PermissionChecker` 接口隔离关系/群组策略，默认 scaffold 使用 deny-all fail-closed；真实用户、好友、黑名单、群成员禁言数据模型后续通过 sqlc 接入后替换默认 checker。
- 用户资料由 `user_info` 表持有，schema 位于 `app/logic/rpc/sql/migrations/003_user_info.sql`，依赖 `app/logic/rpc/sql/migrations/000_extensions.sql` 中的 `pg_trgm` 扩展支持昵称模糊检索。
- `app/logic/rpc/sql/queries/user_info.sql` 定义用户资料 sqlc 查询，生成包为 `app/logic/rpc/internal/model`；业务封装在 `app/logic/rpc/internal/service/user_info_service.go`，RPC 逻辑在 `app/logic/rpc/internal/logic/userservice`。
- 用户资料 avatar 业务默认值：`UserInfoService.CreateUserInfo` 和 `UpdateUserInfoProfile` 必须在写库前将空字符串 avatar 归一化为 `https://implement.me`，否则显式空字符串会覆盖 PostgreSQL `DEFAULT`。
- `UserService.GetUserInfoByNickname` 提供昵称精确查询，供 gateway `GET /api/users/by-name/:name` 代理；昵称模糊搜索继续使用 `SearchUserInfoByNickname`。
- 用户注册同步：auth 服务通过 Kafka 发布 `UserCreatedEvent`（topic: `aim.user.events`），logic 服务的 `UserCreatedConsumer`（`app/logic/rpc/internal/mqs/user_created_consumer.go`）消费该事件并调用 `UserInfoService.CreateUserInfo`；事件携带 `traceparent`/`tracestate` 并在消费侧恢复 context 后创建 `logic.kafka.user_created.consume` consumer span；`ErrUserExists` 被视为幂等成功，其他错误向上传播；如果 `UserInfoService` 未配置（通常是 Postgres 不可用或未配置），消费者必须返回错误而不是静默跳过，避免 Kafka offset 提交后丢失 `user_info` 投影；`UserCreatedConsumerConf` 配置 consumer 名称为 `logic-user-created-consumer`、group 为 `aim-logic-user-created`、topic 为 `aim.user.events`。
- 消息归档同步：`ArchiveConsumer` 消费 core 发布到 `aim-message-transfer` 的 transfer event，消费侧恢复 `traceparent`/`tracestate` 并创建 `logic.kafka.archive.consume` consumer span 后写入 PostgreSQL。
- 服务注册：`app/logic/rpc/logic.go` 启动时通过 `app/shared/nacos` 注册 Nacos v2 临时实例；`app/logic/rpc/etc/logic.yaml` 的 `Nacos` 块维护 `ServerAddr`、`Group`、`Cluster`、`ServiceName`、`AdvertiseIP`、`AdvertisePort` 等注册参数，不再使用 `Etcd`。
- Docker Compose 配置：`app/logic/rpc/etc/logic.yaml` 面向 `docker-compose.yaml` 内部网络，`ListenOn` 为 `0.0.0.0:8080`，`Nacos.ServerAddr` 为 `nacos:8848`，`Nacos.AdvertiseIP` 为 `aim-logic`，PostgreSQL 使用 `postgres:5432`，业务缓存 Redis 使用 `CacheRedis.Addr: redis:6379`（不要命名为 `Redis`，该字段名会与 `zrpc.RpcServerConf.Redis` 冲突），Kafka 使用 `kafka:9092`。服务启动前由 `logic-migrate` 容器初始化 `aim_logic` 数据库并执行 `app/logic/rpc/sql/migrations/` 中的 schema migration。
- go-zero OTel/Jaeger：`app/logic/rpc/etc/logic.yaml` 的 `Telemetry` 块使用 `Name: logic.rpc`、`Batcher: otlphttp`、`Endpoint: jaeger:4318`、`OtlpHttpPath: /v1/traces`，由 `zrpc.RpcServerConf` 自动接入 RPC tracing。
- 配置加载测试：`app/logic/rpc/internal/config/config_test.go` 覆盖 `Telemetry`、`CacheRedis`、PostgreSQL、Nacos 和 `UserCreatedConsumerConf` 配置。

## 边界

- aim-core 可以调用 aim-logic：查询会话成员、好友/黑名单、群成员禁言等投递判断依据。
- aim-logic 不依赖 aim-core 内部状态，也不负责消息投递、Kafka publish、gateway 推送。
- 实时限额拦截仍由 aim-core 热路径 Redis 滑动窗口负责；logic 只持久化计费/配额配置与审计。
