# 模块需求定义

独立的认证服务，位于网关之后、业务服务之前。

- JWT 签发与刷新
- 多设备登录策略（单设备踢下线 / 多设备共存）
- 复杂鉴权

## AccessToken

- JWT Token
- 委托 `gateway` 进行本地快速验签
- 无状态
- TTL: 由 `Token.AccessTTL` 配置控制（默认 5min）；通过 `JWTIssuer` → `sharedjwt.NewManagerWithTTL` → `GenerateAccessToken` 生效

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
- 数据访问：`app/auth/rpc/model`，由 `sqlc` 根据 `model/migrations/` 和 `model/queries/` 生成。
- 网关调用客户端：`app/auth/rpc/authservice`。
- 服务注册：`app/auth/rpc/auth.go` 启动时 `zrpc.MustNewServer(c.RpcServerConf, ...)` 自动调用 `internal.NewRpcPubServer` 把 `Etcd.Key: auth.rpc` + `ListenOn: 0.0.0.0:8989` 注册到 etcd（由 `figureOutListenOn` 自动把 `0.0.0.0` 解析为容器 IP / `POD_IP`）；`app/auth/rpc/etc/auth.yaml` 的 `Etcd` 块维护 `Hosts` / `Key` 两个字段。
- Docker Compose 配置：分层 Compose 通过 `${AIM_CONFIG_DIR:-../config/local}/auth.yaml` 挂载到 `/app/etc/auth.yaml`，本地默认配置副本位于 `deploy/config/local/auth.yaml`；`app/auth/rpc/etc/auth.yaml` 保留给本地 `go run` / 单服务调试。Compose 内 `ListenOn` 为 `0.0.0.0:8989`，`Etcd.Hosts` 为 `[etcd:2379]`，`Etcd.Key` 为 `auth.rpc`，PostgreSQL 使用 `postgres:5432`，Redis 使用 `redis:6379`。
- go-zero OTel/Tempo：`app/auth/rpc/etc/auth.yaml` 的 `Telemetry` 块使用 `Name: auth.rpc`、`Batcher: otlphttp`、`Endpoint: tempo:4318`、`OtlpHttpPath: /v1/traces`，由 `zrpc.RpcServerConf` 自动接入 RPC tracing。

### 行为

- `Register`：规范化 email，校验 `username/name` 必填并去除首尾空白，使用 bcrypt 哈希密码，将 `name` 写入 `user_credentials`（数据库 `NOT NULL`），返回 `user_id`；成功后发布 `UserCreatedEvent` 到 Kafka（KqPusherConf），使用 `user_id` 作为 Kafka key，事件字段包括 `traceparent`/`tracestate`（W3C trace context）、`user_id`、`email`、`nickname`（来自 `username`）、`avatar`、`created_at`（Unix毫秒）；若发布失败则返回 `CodeInternal` 错误，确保 auth 与 logic 数据一致。由于 `go-queue/kq` 的消费接口不向业务层暴露 Kafka header，Kafka trace context 通过事件 JSON payload 传递。
- `Login`：校验 bcrypt 密码和用户状态，签发 5 分钟 JWT AccessToken，创建 UUID RefreshToken。
- `RefreshToken`：校验 Redis 中的 refresh token，删除旧 token，写入新 token，同时签发新 AccessToken。
- `Logout`：按 `user_id` + `device_id` 删除 `auth:device:{user_id}:{device_id}` 与对应 `auth:rt:{token}`。

### 测试与验证

- 闭环测试：`app/auth/rpc/internal/logic/auth_logic_test.go`。
- Redis 轮换/注销测试：`app/auth/rpc/internal/service/session_store_test.go`。
- Etcd 注册/发现由 go-zero 内置 `core/discov` 提供，无 AIM 自定义测试代码。端到端验证：`docker exec aim-etcd etcdctl get --prefix --keys-only /` 能看到 `auth.rpc` 等 4 个 key；`app/auth/rpc/internal/config/config_test.go` 覆盖 `Etcd.Hosts` / `Etcd.Key` 字段。
- 配置加载测试：`app/auth/rpc/internal/config/config_test.go` 覆盖 `Telemetry` 字段，避免链路追踪配置腐化。
- 错误返回使用 `errorx.NewCodeError`（40000/40100/40900/50000），`CodeError` 实现 `GRPCStatus()` 自动转换为对应 gRPC status code（InvalidArgument/Unauthenticated/AlreadyExists/Internal）。
- 关键覆盖率目标：`app/auth/rpc/internal/logic` 和 `app/auth/rpc/internal/service` 均需保持 80% 以上。
