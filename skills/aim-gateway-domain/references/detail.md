# 模块需求定义

维护长连接（WebSocket / TCP）、Protobuf 协议解析、心跳保活、本地快速验证 JWT。

## 外部接口边界

- AIM 的客户端入口统一收敛到 gateway：REST API 只在 `app/gateway/api/gateway.api` 声明并由 `app/gateway/api` 实现；WebSocket 只由 gateway 暴露 `/ws`。
- 非 gateway 模块（auth/core/logic/attachment/data_parsing 等）不得面向客户端或宿主机新增 REST/WS 入口；需要客户端访问的能力必须通过 gateway 代理或编排内部 gRPC/Kafka 完成。
- 服务间接口统一优先使用 gRPC：`aim-attachment` 暴露 `AttachmentService` gRPC（Nacos 服务名 `attachment.rpc`），gateway/core 通过 `AttachmentRpc` 调用；`docker-compose.yaml` 只能 `expose` 内部端口，不能 `ports` publish 到宿主机。
- 排查边界时重点检查：非 gateway 目录是否新增 `.api`、`rest.MustNewServer`、`http.ListenAndServe`/`http.Server`、`websocket.Accept`，以及 Compose 是否将非 gateway REST/WS 端口映射到宿主机；服务间能力应落到 gRPC proto + Nacos 配置。

- 有状态服务，根据 User_ID 做一致性哈希，确保同一用户落在固定网关节点
- 使用 150+ 虚拟节点/物理节点，减少 rebalancing 影响
- 节点下线前推送 reconnect 帧，提供 5-10s drain 窗口，避免惊群重连

## 已实现 auth REST 代理

- 实现位置：`app/gateway/api`。
- REST 规格：`app/gateway/api/gateway.api`。
- 服务入口：`app/gateway/api/gateway.go`。
- Auth RPC 客户端注入：`app/gateway/api/internal/svc/service_context.go`。
- Auth RPC 服务发现：`app/gateway/api/internal/svc/service_context.go` 通过 `app/shared/nacos` 注册 Nacos 适配的 gRPC resolver（scheme `aimnacos`），创建 `zrpc` 客户端使用 `aimnacos.BuildTarget("auth.rpc")`（目标形如 `aimnacos:///auth.rpc`）。Resolver 订阅 Nacos 实例变更，auth 后启动不会导致 gateway panic，实例上线后自动发现；`app/gateway/api/etc/gateway-api.yaml` 的 `AuthRpc` 块是 Nacos 配置，不再使用 `AuthRpc.Etcd`。
- Nacos gRPC resolver（scheme `aimnacos`）实现：`app/shared/nacos/resolver.go`，通过 `NamingClient.Subscribe` 监听服务实例变更并更新 gRPC address list；同一进程内只注册一次全局 scheme，但每个 `aimnacos:///<serviceName>` 客户端必须按 target endpoint 独立订阅对应服务名，避免 `auth.rpc`、`core.rpc`、`logic.rpc` 串线；scheme 不得使用 `nacos`，避免抢占 Nacos SDK 内部 `nacos:9848` 直连目标；支持空初始实例（auth 后启动不 panic）、实例上线动态添加、下线动态移除。
- Docker Compose 配置：分层 Compose 通过 `${AIM_CONFIG_DIR:-../config/local}/gateway-api.yaml` 挂载到容器内 `/app/etc/gateway-api.yaml`，本地默认配置副本位于 `deploy/config/local/gateway-api.yaml`；`app/gateway/api/etc/gateway-api.yaml` 保留给本地 `go run` / 单服务调试。`AuthRpc.ServerAddr` 在 Compose 网络内使用 `nacos:8848`。
- JWT 本地验签：`app/shared/jwt`。
- `Authorization` header 上下文传递：`app/gateway/api/internal/authctx`。
- REST Auth 中间件：`app/gateway/api/internal/middleware/auth_middleware.go`，对 `/api/conversations` 和 `/api/users` 路由组生效。中间件通过 `wsauth.ExtractAndValidate` 验签 Bearer token，成功后将 `ws.Identity{UserID, DeviceID}` 注入 `ws.WithIdentity` 上下文。失败返回 401 JSON `{code, msg}`。
- REST 认证路由组在 `gateway.api` 的 `@server` 块中声明 `middleware: Auth`，goctl 自动在 `routes.go` 中注册 `serverCtx.Auth` 中间件。`ServiceContext.Auth` 字段类型为 `rest.Middleware`，在 `NewServiceContext` 中初始化为 `middleware.NewAuthMiddleware(c.Auth.AccessSecret).Handle`。
- 逻辑层通过 `ws.IdentityFromContext(l.ctx)` 获取认证用户身份；未认证时返回 `errorx.CodeAuth/"unauthorized"`。
- REST 统一响应包装：`app/gateway/api/internal/handler/response.go` 通过 go-zero `httpx.SetOkHandler` / `httpx.SetErrorHandlerCtx` 输出 `{code,msg,body}`。
- 用户 REST 代理：`GET /api/users/by-name/:name` 位于 `app/gateway/api/internal/logic/users/get_user_by_name_logic.go`，通过 `LogicRpc` 调用 `aim-logic UserService.SearchUserInfoByNickname` 做 PostgreSQL `pg_trgm`/GIN 支撑的昵称模糊查询，返回 `users(id,email,avatar)` 列表；nickname 不唯一，不要对外返回单个用户详情。`GET /api/users/by-id/:id` 位于 `app/gateway/api/internal/logic/users/get_user_by_id_logic.go`，通过 `UserService.GetUserInfo` 返回单个用户详情。`POST /api/users/friends/:id` 位于 `app/gateway/api/internal/logic/users/add_friend_logic.go`，从 `ws.IdentityFromContext` 读取认证用户 ID 后调用 `aim-logic FriendshipService.AddFriend`。非 gRPC 客户端错误必须记录日志并对外返回 `50000/"internal error"`。AddFriend 成功后，如目标用户当前连接在本地网关节点，gateway 通过 `GatewayService.PushFriendApplication` 推送 `FRAME_TYPE_PUSH_FRIEND_APPLICATION` 帧通知目标用户。
- 目录结构决策：gateway 作为 go-zero API 服务保留 `app/gateway/api` 布局；auth/core/logic 作为 go-zero RPC 服务保留 `app/{module}/rpc` 布局。不要为了“统一”机械搬迁 `api/`/`rpc/`，新增 REST handler/logic 应按 `gateway.api` 的 `group` 生成到对应分组目录（如 `internal/handler/users`、`internal/logic/users`）。
- go-zero OTel/Tempo：`app/gateway/api/etc/gateway-api.yaml` 的顶层 `Telemetry` 块覆盖 REST 服务，`GatewayRpc.Telemetry` 块覆盖 GatewayService RPC 服务；两者均使用 `Batcher: otlphttp`、`Endpoint: tempo:4318`、`OtlpHttpPath: /v1/traces`。REST 依赖 `rest.RestConf` 内嵌的 `service.ServiceConf.Telemetry` 自动启动 trace agent，GatewayService 依赖 `zrpc.RpcServerConf.Telemetry` 自动接入 go-zero RPC tracing interceptor。
- go-zero Prometheus：YAML 中 `Prometheus` 配置块（`Host: 0.0.0.0`、`Port: 9191`、`Path: /metrics`）由 `rest.RestConf` / `zrpc.RpcServerConf` 自动启动独立 HTTP server。REST/RPC middleware 默认开启，自动采集请求 QPS、延迟、错误率等指标。Grafana 仪表盘见 `deploy/grafana/dashboards/`。
- Core → GatewayService 的客户端位于 `app/core/rpc/internal/rpc/gateway_client.go`，不是 go-zero `zrpc` client；该 raw gRPC client 必须保留自定义 unary interceptor 注入 W3C trace metadata，使 `core.kafka.delivery.consume` 后续的 `PushMessage` 调用能进入 GatewayService trace。

### 闭环边界

- `register/login/refresh` 只做 REST -> auth RPC 转发，不在 gateway 持有凭证状态。
- `logout` 是 gateway 的本地 JWT 验签边界：从 Bearer token 解析 `user_id` 和 `device_id`，再调用 auth RPC 注销 refresh token。
- AccessToken 签发仍属于 auth 域；gateway 只负责快速验签和代理转发。
- auth RPC 返回的错误由 `sanitizeAuthRPCError` 处理：通过 `errorx.FromGRPCError` 从 gRPC status 中提取 biz code（40100/unauthenticated, 40900/conflict 等业务错误保留原文透传），基础设施错误（Internal/Unavailable/DataLoss 等）sanitize 为 `"internal error"`；非 gRPC 错误（网络超时等）记录日志后返回 `50000/"internal error"`。
- `CodeError` 实现了 `GRPCStatus()` 方法，gRPC 框架自动将 biz code 类别映射为对应的 gRPC status code（如 40100→Unauthenticated, 40900→AlreadyExists），message 保留原文。
- HTTP 错误响应 `response.go`：优先级依次尝试 `*errorx.CodeError` → `errorx.FromGRPCError` → gRPC status 兜底 sanitize → 普通 error。
- `httpStatusFromCode` 支持 40xxx-59xxx 范围（code/100 自动映射，clamp [400,511]）和 sentinel codes（1001-1006，显式语义映射：1001/1004/1005→401, 1002→404, 1003→409, 1006→403）。

### 测试与验证

- 代理逻辑测试：`app/gateway/api/internal/logic/auth/proxy_logic_test.go`。
- Logout JWT 测试：`app/gateway/api/internal/logic/auth/logout_logic_test.go`。
- 统一响应测试：`app/gateway/api/internal/handler/response_test.go`。
- 配置加载测试：`app/gateway/api/internal/config/config_test.go` 覆盖 REST Telemetry 字段和 `GatewayRpc` zrpc/Telemetry 字段。
- Nacos 注册/发现适配测试：`app/shared/nacos`，包括 register、deregister、BuildDirectTarget、BuildTarget、gRPC resolver 构建、按 target 服务名订阅、空实例回调、实例上线/下线动态更新。
- 关键覆盖率目标：`app/gateway/api/internal/logic/auth` 保持 80% 以上。
- 用户代理测试：`app/gateway/api/internal/logic/users/get_user_by_name_logic_test.go` 覆盖 by-name 模糊列表响应与 by-id 详情响应；好友新增代理逻辑位于 `app/gateway/api/internal/logic/users/add_friend_logic.go`，需保持 `LogicFriendshipClient` 注入和 `ws.Identity` 鉴权路径可测。
- 关键验证命令：`goctl api validate -api app/gateway/api/gateway.api`、`go mod tidy`、`go build ./...`、`go test -coverprofile=count.out ./...`、`go vet ./...`、`golangci-lint run ./...`。

## 已实现 WebSocket 端点

### WebSocket `/ws` 端点

- 实现位置：`app/gateway/api/internal/handler/ws/ws_handler.go`。
- 路由注册：`app/gateway/api/internal/handler/routes.go`，`GET /ws` 路由独立挂载在根路径 `/ws`，不使用 `/api/auth` 前缀。
- JWT 验签：仅支持 `Authorization: Bearer <token>` header，验签失败在 upgrade 前返回 JSON `CodeError`；禁止 `?token=<token>`，避免 JWT 泄露到访问日志、浏览器历史和代理日志。
- Protobuf WS 帧协议：`shared/proto/ws/ws.proto` 定义 `FrameType` 枚举和 `WsFrame` 主帧格式，以及所有 Payload 消息类型。
- Protobuf 代码生成：`shared/proto/ws/pb/ws.pb.go`，使用 `protoc --go_out=. ws.proto` 生成。
- 连接管理：`app/gateway/api/internal/ws/manager.go`，`Manager` 按 `user_id + device_id` 注册/注销连接，支持多设备同一用户。
- 帧编解码：`app/gateway/api/internal/ws/frame.go`，`EncodeFrame`/`DecodeFrame` 负责 WsFrame 序列化，`EncodePayload`/`DecodePayload` 负责各 Payload 类型序列化，`BuildFrame`/`NewServerAck` 构建响应帧。
- JWT Token 提取：`app/gateway/api/internal/ws/auth/token.go`，`ExtractAndValidate` 组合提取和验签，返回 `*jwt.Claims`。
- 心跳处理：收到 `FRAME_TYPE_HEARTBEAT` 返回 `FRAME_TYPE_SERVER_ACK`。
- 消息处理：收到 `FRAME_TYPE_SEND_MESSAGE` 返回 `FRAME_TYPE_SERVER_ACK`（不转发 core）。
- 依赖：`github.com/coder/websocket`（已添加），`google.golang.org/protobuf`。

### WebSocket 配置

- 配置定义：`app/gateway/api/internal/config/config.go` 的 `WebSocket` struct，字段包括 `OriginPatterns`、`ReadLimit`、`WriteLimit`、`PongWait`、`PingPeriod`、`MaxMsgSize`、`ServerAckDelay`、`HeartbeatInterv`；`OriginPatterns` 传给 `coder/websocket.AcceptOptions`，默认仅允许同源请求。
- 配置文件：`app/gateway/api/etc/gateway-api.yaml` 的 `WebSocket` 块。

### 测试

- 帧编解码测试：`app/gateway/api/internal/ws/frame_test.go`（8 个测试用例）。
- 连接管理测试：`app/gateway/api/internal/ws/manager_test.go`（11 个测试用例，含并发测试）。
- Token 提取测试：`app/gateway/api/internal/ws/auth/token_test.go`（9 个测试用例）。

### 闭环边界

- `/ws` 不转发消息到 core/logic，仅返回 `FRAME_TYPE_SERVER_ACK`。
- 连接注销在 context cancel 或 disconnect 时触发。
- 心跳和消息 ACK 使用 `FRAME_TYPE_SERVER_ACK` 帧类型。

## 已实现 GatewayService gRPC 服务端

实现位置：`app/gateway/api` 内嵌的 `GatewayRpc` zrpc 服务；入口为 `app/gateway/api/gateway.go`，配置块为 `app/gateway/api/etc/gateway-api.yaml` 的 `GatewayRpc`。不要再使用手写 `grpc.NewServer` 启动 GatewayService，避免绕过 go-zero tracing interceptor、health check 和统一生命周期管理。

### 六个推送 RPC 方法

Proto 定义：`shared/proto/gateway/gateway.proto`，生成的 pb 代码在 `shared/proto/gateway/pb/`。

| RPC | 用途 | 调用方 |
| --- | --- | --- |
| `PushMessage` | 将聊天消息投递到目标用户的 WebSocket 连接 | aim-core/Delivery Consumer |
| `PushPresence` | 推送用户在线状态变更通知给目标用户的好友 | aim-core/Presence Consumer |
| `PushTyping` | 推送输入状态给目标用户 | aim-core/Typing Consumer |
| `KickUser` | 踢下线指定用户设备（多设备管理/被迫下线） | aim-auth / 管理员后台 |
| `DrainNotify` | 通知网关节点进行优雅迁移（会话 drain） | Nacos 服务发现 / 运维工具 |
| `PushFriendApplication` | 推送好友申请通知给目标用户 | aim-logic/FriendshipService |

### PushMessage

- 请求：`PushMessageReq`（message_id, conversation_id, conversation_type, message_type, content, sender_id, sent_at, client_msg_id, mentions, target_user_id, is_system, sender_info, source_device_id）
- 响应：`PushMessageResp`（success）
- 内部实现：查找 `target_user_id` 对应的本地 WebSocket 连接，写入 `FRAME_TYPE_PUSH_MESSAGE` 帧
- 多端同步：普通消息的 core fan-out 会包含发送者用户；当 `target_user_id == sender_id` 且连接 `device_id == source_device_id` 时，gateway 跳过原始发送设备，仍推送给发送者其他设备
- 投递成功（用户在线）返回 `success=true`；用户不在线也返回 `success=true`（消息已在投递链路中处理）

### PushPresence

- 请求：`PushPresenceReq`（user_id, status, updated_at）
- 响应：`PushPresenceResp`（success）
- 内部实现：向 `notify_user_ids` 列表中的用户推送 `FRAME_TYPE_PUSH_PRESENCE` 帧

### KickUser

- 请求：`KickUserReq`（user_id, device_id, reason）
- 响应：`KickUserResp`（kicked_count）
- `device_id` 为空字符串踢掉所有设备，否则只踢指定设备
- reason 取值：`duplicate_login` / `admin_kick` / `security`

### DrainNotify

- 请求：`DrainNotifyReq`（drain_timeout_ms, gateway_node_id）
- 响应：`DrainNotifyResp`（affected_count）
- 内部实现：向所有连接推送 `FRAME_TYPE_RECONNECT` 帧，等待 drain_timeout_ms 后关闭连接

### PushFriendApplication

- 请求：`PushFriendApplicationReq`（user_id, application_id, from_user_id, from_nickname, created_at）
- 响应：`PushFriendApplicationResp`（success）
- 内部实现：查找 `user_id` 对应的本地 WebSocket 连接，写入 `FRAME_TYPE_PUSH_FRIEND_APPLICATION` 帧
- 调用方：aim-logic/FriendshipService 在好友申请创建后通过 gRPC 调用

## 已实现 CoreRpc 客户端

实现位置：`app/gateway/api/internal/svc/service_context.go`（gateway API 进程内的 gRPC client，调用 aim-core）。

### CoreRpc 配置

配置定义：`app/gateway/api/internal/config/config.go` 的 `CoreRpc aimnacos.Config` 字段。Docker Compose 部署使用 `deploy/config/<env>/gateway-api.yaml`（容器内 `/app/etc/gateway-api.yaml`），本地 `go run` / 单服务调试使用 `app/gateway/api/etc/gateway-api.yaml`。

目标地址通过 AIM Nacos resolver 发现（目标形如 `aimnacos:///core.rpc`），`gateway-api.yaml` 的 `CoreRpc` 块配置 Nacos 注册中心地址、服务名 `core.rpc` 等发现参数。

### Transfer 调用流程

1. WebSocket 收到 `FRAME_TYPE_SEND_MESSAGE`（SendMessagePayload）
2. JWT 验签通过后，提取 `sender_id` / `device_id`
3. 调用 `core.Transfer(TransferReq)` gRPC
4. 根据返回结果映射到 `ServerAckPayload` 回传给客户端

## ACK 映射表

`core.Transfer` gRPC 返回结果映射到 `FRAME_TYPE_SERVER_ACK`（ServerAckPayload）：

| Core gRPC 结果 | WS AckStatus | WS code | 客户端行为 |
| --- | --- | --- | --- |
| Transfer 成功 | `ACK_STATUS_ACCEPTED` | `0` | 标记消息已发送/已接受 |
| 重复 client_msg_id（已有 message_id） | `ACK_STATUS_ACCEPTED` | `0` | 使用返回的已有 message_id |
| 无效输入（参数校验失败） | `ACK_STATUS_REJECTED` | `40000` | 显示校验错误，不自动重试 |
| 身份未认证（token 无效/已过期） | `ACK_STATUS_REJECTED` | `40100` | 刷新 token 或重新登录 |
| 被禁言/拉黑/非会话成员 | `ACK_STATUS_REJECTED` | `40300` | 显示业务错误，不自动重试 |
| 会话不存在 | `ACK_STATUS_REJECTED` | `40400` | 显示业务错误，不自动重试 |
| 配额/限流（滑动窗口触发） | `ACK_STATUS_REJECTED` | `42900` | 显示配额错误，仅在策略延迟后重试 |
| core 不可用/deadline/Kafka 瞬时故障 | `ACK_STATUS_RETRYABLE` | `50000` | 保留本地 pending 消息，用相同 client_msg_id 重试 |

### ServerAckPayload 结构

```protobuf
enum AckStatus {
  ACK_STATUS_UNSPECIFIED = 0;
  ACK_STATUS_ACCEPTED = 1;
  ACK_STATUS_REJECTED = 2;
  ACK_STATUS_RETRYABLE = 3;
}

message ServerAckPayload {
  int64     ack_seq       = 1; // 确认的客户端序列号
  string    client_msg_id = 2; // 确认的客户端消息 ID
  int32     code          = 3; // 0=ok，否则为 shared/errorx biz code
  string    msg           = 4; // 错误描述
  AckStatus status        = 5; // ACCEPTED / REJECTED / RETRYABLE
  int64     message_id    = 6; // 服务端分配的消息 ID（accepted 时 > 0）
}
```

`ACK_STATUS_REJECTED`：业务无效，不自动重试。
`ACK_STATUS_RETRYABLE`：瞬时故障，客户端可用相同 client_msg_id 重试。

## Presence 在线状态流程

### 连接管理（Manager）

- `Manager.Register` 时 SADD `aim:user_gateway:{user_id}`（node_id）和 `aim:presence:{user_id}`（device_id），检测 SCARD 0→≥1 则触发 `online` 事件。
- `Manager.Unregister` 时 SREM 对应 member，若本节点无该用户其他连接则 SREM gateway set，检测 SCARD ≥1→0 则触发 `offline` 事件。
- 仅状态 0↔1 切换时发布 Kafka 事件（topic: `aim.presence.events`，key=`user_id`）。

### 心跳

- `FRAME_TYPE_HEARTBEAT` 调用 `Manager.RenewPresenceTTL` 续约两个 Set 的 TTL，不发布事件。
- 客户端心跳间隔 20s，Redis TTL 默认 45s（≈ 2× 心跳 + 缓冲）。

### 状态快照

- `GET /api/presence/friends`（Auth 保护）批量 SCARD `aim:presence:{friend_id}` 返回好友在线状态列表。
- 客户端在 WS 连接建立 / 重连成功后调用此接口填充初始在线状态。

### Typing 输入状态

- `FRAME_TYPE_TYPING` 由网关直接发布到 Kafka（topic: `aim.typing.events`，key=`conversation_id`），不查成员。
- core 的 `TypingConsumer` 消费后查会话成员 → 查 `aim:user_gateway:{member_id}` → 调 `gateway.PushTyping`。
- 网关 `PushTyping` gRPC 按 `target_user_id` 将 `FRAME_TYPE_PUSH_TYPING` 投到本节点所有连接。
- 客户端按 2.5s 节流发送 typing 帧；收到 PUSH_TYPING 后 4s 超时自动清除。

### 群成员角色边界

- `conversation_members.role` 已入库，取值为 `owner / admin / member`。
- 当前 gateway 侧群管理接口仅负责参数校验与身份透传，不在 gateway 内做角色判断；权限边界统一由 logic 层强制执行。
- 现阶段群管理能力按“群主强约束”落地：
  - `POST /api/conversations/:id/members`：仅 `owner` 可邀请成员。
  - `DELETE /api/conversations/:id/members/:uid`：仅 `owner` 可移除成员。
  - `POST /api/conversations/:id/leave`：`owner` 不能直接退群。
  - `DELETE /api/conversations/:id`：仅 `owner` 可解散群聊。
  - `PUT /api/conversations/:id`：仅 `owner` 可修改群名/群头像。
- `admin` 角色目前仅作为成员详情返回字段保留，不授予额外管理权限；后续如果开放管理员能力，需要同步更新 logic 侧权限矩阵、gateway 文档和客户端交互。

## Token 过期生命周期

### Token 过期帧

```protobuf
FRAME_TYPE_TOKEN_EXPIRED = 107;

message TokenExpiredPayload {
  int64  expired_at = 1; // 过期时间戳
  string reason     = 2; // access_token_expired
}
```

### 处理流程

1. JWT 验签时记录 `exp`（过期时间戳）
2. 在 WebSocket 读循环或定时器中检查 token 是否即将过期
3. 推送 `FRAME_TYPE_TOKEN_EXPIRED` 给客户端
4. 关闭 WebSocket 连接
5. 客户端调用 `/api/auth/refresh` 获取新 token，然后重新连接 `/ws`

### 依赖方向

- aim-core → aim-logic（单向）：core 通过 gRPC 查询 logic 的好友/群组关系（Redis TTL 缓存），logic 不反向依赖 core
- gateway → core：gateway 通过 gRPC 调用 core.Transfer，gateway 作为 Ingress 不反向依赖 core 内部状态
