# 模块需求定义

独立的认证服务，位于网关之后、业务服务之前。

- JWT 签发与刷新
- 多设备登录策略（单设备踢下线 / 多设备共存）
- 复杂鉴权

## AccessToken

- JWT Token
- 委托 `gateway` 进行本地快速验签
- 无状态
- TTL: 5 min

## RefreshToken

- UUID
- 由 `auth` 管理
- TTL: 7 day
- 有状态(redis)
  - `auth:rt:{token}`: `{user_id}:{device_id}` ttl: 7 days
  - `auth:device:{user_id}:{device_id}`: `{token}` no ttl

## 已实现闭环

- 实现位置：`app/auth/rpc`。
- 服务入口：`app/auth/rpc/auth.go`。
- 接口规格：`app/auth/rpc/auth.proto`，通过 `goctl rpc protoc ... --style go_zero` 生成。
- 业务逻辑：`app/auth/rpc/internal/logic`。
- 领域服务：`app/auth/rpc/internal/service/auth_service.go`。
- 数据访问：`app/auth/rpc/model`，由 `sqlc` 根据 `schema.sql` 和 `query.sql` 生成。
- 网关调用客户端：`app/auth/rpc/authservice`。
- 服务注册：`app/auth/rpc/auth.go` 启动时通过 `app/shared/nacos` 使用 `github.com/nacos-group/nacos-sdk-go/v2` 注册 Nacos v2 临时实例；`app/auth/rpc/etc/auth.yaml` 的 `Nacos` 块维护 `ServerAddr`、`Group`、`Cluster`、`ServiceName`、`AdvertiseIP`、`AdvertisePort` 等注册参数，不再使用 go-zero 默认 `Etcd` 注册。
- Docker Compose 配置：`app/auth/rpc/etc/auth.yaml` 面向 `docker-compose.yaml` 内部网络，`ListenOn` 为 `0.0.0.0:8989`，`Nacos.ServerAddr` 为 `nacos:8848`，`Nacos.AdvertiseIP` 为 `aim-auth`，PostgreSQL 使用 `postgres:5432`，Redis 使用 `redis:6379`。
- go-zero OTel/Jaeger：`app/auth/rpc/etc/auth.yaml` 的 `Telemetry` 块使用 `Name: auth.rpc`、`Batcher: otlphttp`、`Endpoint: jaeger:4318`、`OtlpHttpPath: /v1/traces`，由 `zrpc.RpcServerConf` 自动接入 RPC tracing。

### 行为

- `Register`：规范化 email，使用 bcrypt 哈希密码，写入 `user_credentials`，返回 `user_id`；成功后发布 `UserCreatedEvent` 到 Kafka（KqPusherConf），使用 `user_id` 作为 Kafka key，事件字段包括 `traceparent`/`tracestate`（W3C trace context）、`user_id`、`email`、`nickname`（来自 `username`）、`avatar`、`created_at`（Unix毫秒）；若发布失败则返回 `CodeInternal` 错误，确保 auth 与 logic 数据一致。由于 `go-queue/kq` 的消费接口不向业务层暴露 Kafka header，Kafka trace context 通过事件 JSON payload 传递。
- `Login`：校验 bcrypt 密码和用户状态，签发 5 分钟 JWT AccessToken，创建 UUID RefreshToken。
- `RefreshToken`：校验 Redis 中的 refresh token，删除旧 token，写入新 token，同时签发新 AccessToken。
- `Logout`：按 `user_id` + `device_id` 删除 `auth:device:{user_id}:{device_id}` 与对应 `auth:rt:{token}`。

### 测试与验证

- 闭环测试：`app/auth/rpc/internal/logic/auth_logic_test.go`。
- Redis 轮换/注销测试：`app/auth/rpc/internal/service/session_store_test.go`。
- Nacos 注册/发现适配测试：`app/shared/nacos`。
- 配置加载测试：`app/auth/rpc/internal/config/config_test.go` 覆盖 `Telemetry` 字段，避免链路追踪配置腐化。
- 错误返回使用 `errorx.NewCodeError`（40000/40100/40900/50000），`CodeError` 实现 `GRPCStatus()` 自动转换为对应 gRPC status code（InvalidArgument/Unauthenticated/AlreadyExists/Internal）。
- 关键覆盖率目标：`app/auth/rpc/internal/logic` 和 `app/auth/rpc/internal/service` 均需保持 80% 以上。
