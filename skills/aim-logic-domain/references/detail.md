# 模块需求定义

提供消息投递的判断依据和业务逻辑支撑。

- User/Relationship Service(用户与社交):持有用户资料表、好友申请、黑名单、群组元数据、群成员禁言/转让
- Message Archive Service(消息持久化):从 Kafka 异步消费消息,写入 PostgreSQL 分区表 + JSONB;提供 tsvector 全文搜索、历史回溯接口
- Billing & Quota Service(计费管理):平台点数扣费、计费流水审计(PostgreSQL 持久化);实时限额通过 Redis 滑动窗口实现,避免 PostgreSQL 往返延迟
- Content Moderation(内容审核):作为共享库(in-process)供 aim-core 和 aim-ai 同步调用;异步审计日志由独立 worker 处理

## 当前落地范围

- `app/logic/rpc/logic.proto` 定义 `PermissionService`、`UserService`、`ConversationService` 与 `FriendshipService`(四个 gRPC 服务)。
- `CheckMessagePermission` 是 aim-core 在接受消息进入 Kafka 前查询的业务上下文接口。
- 当前实现通过 `PermissionChecker` 接口隔离关系/群组策略,默认 scaffold 使用 deny-all fail-closed;真实用户、好友、黑名单、群成员禁言数据模型后续通过 sqlc 接入后替换默认 checker。
- 用户资料由 `user_info` 表持有,schema 位于 `app/logic/rpc/model/migrations/003_user_info.sql`,依赖 `app/logic/rpc/model/migrations/000_extensions.sql` 中的 `pg_trgm` 扩展支持昵称模糊检索。
- `app/logic/rpc/model/queries/user_info.sql` 定义用户资料 sqlc 查询,生成包为 `app/logic/rpc/model`;业务封装在 `app/logic/rpc/internal/service/user_info_service.go`,RPC 逻辑在 `app/logic/rpc/internal/logic/userservice`。
- 用户资料 avatar 业务默认值:`UserInfoService.CreateUserInfo` 和 `UpdateUserInfoProfile` 必须在写库前将空字符串 avatar 归一化为 `https://implement.me`,否则显式空字符串会覆盖 PostgreSQL `DEFAULT`。
- `UserService.GetUserInfoByNickname` 仅提供内部昵称精确查询;nickname 不唯一,对外 gateway `GET /api/users/by-name/:name` 必须使用 `SearchUserInfoByNickname` 走 PostgreSQL `pg_trgm`/GIN 模糊查询并返回用户列表。gateway `GET /api/users/by-id/:id` 使用 `GetUserInfo` 查询单个用户详情。
- 用户注册同步:auth 服务通过 Kafka 发布 `UserCreatedEvent`(topic: `aim.user.events`),logic 服务的 `UserCreatedConsumer`(`app/logic/rpc/internal/mqs/user_created_consumer.go`)消费该事件并调用 `UserInfoService.CreateUserInfo`;事件携带 `traceparent`/`tracestate` 并在消费侧恢复 context 后创建 `logic.kafka.user_created.consume` consumer span;`ErrUserExists` 被视为幂等成功,其他错误向上传播;如果 `UserInfoService` 未配置(通常是 Postgres 不可用或未配置),消费者必须返回错误而不是静默跳过,避免 Kafka offset 提交后丢失 `user_info` 投影;`UserCreatedConsumerConf` 配置 consumer 名称为 `logic-user-created-consumer`、group 为 `aim-logic-user-created`、topic 为 `aim.user.events`。
- 消息归档同步:`ArchiveConsumer` 消费 core 发布到 `aim-message-transfer` 的 transfer event,消费侧恢复 `traceparent`/`tracestate` 并创建 `logic.kafka.archive.consume` consumer span 后写入 PostgreSQL。
- 会话管理:`ConversationService` 提供 `CreateConversation`(创建私聊或群聊)和 `GetConversationHistory`(拉取消息历史,游标分页)。服务封装在 `app/logic/rpc/internal/service/conversation_service.go`,接口为 `ConversationQuerier`;RPC 逻辑在 `app/logic/rpc/internal/logic/conversationservice/`。sqlc 查询位于 `app/logic/rpc/model/queries/conversation.sql`(对话 CRUD + 成员查询)和 `app/logic/rpc/model/queries/message.sql`(消息插入 + 历史分页查询)。
- 对话创建验证规则:`conversation_type` 必须为 `direct` 或 `group`;`direct` 类型必须恰好 2 个成员;`creator_id` 自动加入成员列表(如未在 member_ids 中)。**direct 类型会先在 `conversation_members` 表中查找已有活跃会话(Find-or-Create)**:通过 `GetDirectConversationByMembers(user_id1, user_id2)` 查询(self-JOIN `conversation_members`),找到则直接返回已有会话 ID,避免重复创建;`pgx.ErrNoRows` 表示无已有会话,继续正常创建流程。
- 消息历史分页:首页查询使用 `ListMessagesByConversationInitial`(无游标);翻页使用 `ListMessagesByConversation`(基于 `(created_at DESC, id DESC)` 游标);每页默认 50 条,最大 100 条。
- 服务注册:`app/logic/rpc/logic.go` 启动时通过 `app/shared/nacos` 注册 Nacos v2 临时实例;`app/logic/rpc/etc/logic.yaml` 的 `Nacos` 块维护 `ServerAddr`、`Group`、`Cluster`、`ServiceName`、`AdvertiseIP`、`AdvertisePort` 等注册参数,不再使用 `Etcd`。
- Docker Compose 配置:`app/logic/rpc/etc/logic.yaml` 面向 `docker-compose.yaml` 内部网络,`ListenOn` 为 `0.0.0.0:8080`,`Nacos.ServerAddr` 为 `nacos:8848`,`Nacos.AdvertiseIP` 为 `aim-logic`,PostgreSQL 使用 `postgres:5432`,业务缓存 Redis 使用 `CacheRedis.Addr: redis:6379`(不要命名为 `Redis`,该字段名会与 `zrpc.RpcServerConf.Redis` 冲突),Kafka 使用 `kafka:9092`。服务启动前由 `logic-migrate` 容器初始化 `aim_logic` 数据库并执行 `app/logic/rpc/model/migrations/` 中的 schema migration。
- go-zero OTel/Jaeger:`app/logic/rpc/etc/logic.yaml` 的 `Telemetry` 块使用 `Name: logic.rpc`、`Batcher: otlphttp`、`Endpoint: jaeger:4318`、`OtlpHttpPath: /v1/traces`,由 `zrpc.RpcServerConf` 自动接入 RPC tracing。
- 配置加载测试:`app/logic/rpc/internal/config/config_test.go` 覆盖 `Telemetry`、`CacheRedis`、PostgreSQL、Nacos 和 `UserCreatedConsumerConf` 配置。
- 好友关系：`FriendshipService` 提供 `AddFriend`（发送好友请求，幂等：已 pending/accepted 直接返回现有记录）、`ListFriendApplications`（查询当前用户收到的待处理好友申请）、`AcceptFriend`（接受好友请求）、`RejectFriend`（拒绝好友请求）和 `ListFriends`（列出已接受好友列表）。`friendships` 表 schema 位于 `app/logic/rpc/model/migrations/001_initial_permission.sql`（user_id, friend_id, status, created_at, updated_at），status 取值 `pending`/`accepted`/`blocked`。sqlc 查询位于 `app/logic/rpc/model/queries/friendship.sql`（UpsertFriendship, GetFriendshipByPair, ListPendingFriendApplications, ListFriends 等），业务封装在 `app/logic/rpc/internal/service/friendship_service.go`，RPC 逻辑在 `app/logic/rpc/internal/logic/friendshipservice/`。`database_permission_checker.go` 仍通过 `GetFriendshipBidirectional` 做投递权限校验，但对于非好友的 direct 会话，允许作为“临时会话”发送最多 `Dev.TemporaryConversationMessageLimit` 条消息（默认 10；设 0/负数表示不限制，仅供本地开发/压测使用；由 `CountMessagesByConversation` 检查已发送消息数），而非直接拒绝“not friends”。限额配置在 `app/logic/rpc/etc/logic.yaml` 的 `Dev:` 块下。
- 好友接口幂等保护:AddFriend 先通过 `GetFriendshipByPair` 检查正向关系,如已为 `pending` 或 `accepted` 则直接返回,防止 `UpsertFriendship(status=pending)` 将 `accepted` 降级。
- 测试覆盖率:`ConversationService` 服务层 91.6%(含 3 个去重测试用例),`ConversationService` RPC 逻辑层 89.4%;`FriendshipService` RPC 逻辑层 89.7%。

## 群管理

### 架构

群变更事件遵循统一模式：

```
┌─────────┐  DB事务   ┌────┐  事务提交后Kafka  ┌──────┐  gRPC   ┌──────────┐  WS   ┌────────┐
│  Logic  │ ────────→ │ DB │ ───────────────→ │ Core │ ──────→ │ Gateway  │ ────→ │ Client │
└─────────┘          └────┘                   └──────┘         └──────────┘       └────────┘
                         │                         │
                         │  aim.conversation.events │
                         └─────────────────────────┘
```

- **事务内**：DB 操作（成员变更 + 插入系统消息 + 计算 target_user_ids）
- **事务提交后**：生产 Kafka 事件到 `aim.conversation.events`（best-effort）
- **Core 消费**：`ConversationEventConsumer` 消费事件，通过 Gateway PushMessage 推送给每个 target_user_id
- **is_system**：`PushMessageReq.is_system = true` 区分群变更消息和普通消息

### 数据库迁移

`app/logic/rpc/model/migrations/004_group_management.sql`：

| 表 | 字段 | 说明 |
|----|------|------|
| conversations | `name VARCHAR(128) DEFAULT ''` | 会话名称；群聊为群名 |
| conversations | `avatar VARCHAR(512) DEFAULT ''` | 群聊头像 URL |
| conversations | `creator_id BIGINT DEFAULT 0` | 创建者/群主 |
| conversation_members | `role VARCHAR(16) DEFAULT 'member'` | 取值：owner / admin / member |

### 事务支持

`ConversationService` 新增 `InTx` 方法，使用 `pgx` 标准库事务：

```go
func (s *ConversationService) InTx(ctx context.Context, fn func(txQueries *model.Queries) error) error
```

- `ConversationService` 持有 `pool *pgxpool.Pool` 引用（与 `store *model.Queries` 并存）
- 事务内使用 `model.New(tx)` 创建临时 Queries，`defer tx.Rollback(ctx)`
- 事务提交失败自动回滚；业务错误返回后回滚

### 系统消息

群变更消息统一使用 `message_type="system"`、`sender_id=0` 写入 messages 表：

| 事件类型 | 触发场景 |
|---------|---------|
| `member_joined` | 添加成员 |
| `member_left` | 成员主动退群 |
| `member_removed` | 管理员移除成员 |
| `group_renamed` | 群聊改名 |
| `group_dismissed` | 群聊解散 |
| `group_avatar_changed` | 群头像变更 |

### target_user_ids 计算规则

| 事件 | target_user_ids 来源 |
|------|---------------------|
| `member_joined` | 变更后的全部成员（含新增） |
| `member_removed` | 变更前的全部成员（含将被移除的） |
| `member_left` | 变更前的全部成员（含退出者） |
| `group_renamed` | 当前全部成员 |
| `group_avatar_changed` | 当前全部成员 |
| `group_dismissed` | 解散前的全部成员 |

### CreateConversation 改造

- `CreateConversation` 新增 `name` 参数
- 创建者使用 `AddConversationMemberWithRole` 设置 `role = "owner"`
- 其他成员使用 `AddConversationMembers`（默认 role = "member"）

### 配置

`app/logic/rpc/internal/config/config.go` 新增：

```go
type KqPusherConf struct {
    Brokers []string
    Topic   string
}

type Config struct {
    // ... 现有字段 ...
    ConversationEventProducerConf KqPusherConf `json:",optional"`
}
```

`ServiceContext` 新增 `ConversationEventPusher *kq.Pusher`。logic.yaml 需添加对应 Kafka 配置块。

## 边界

- aim-core 可以调用 aim-logic:查询会话成员、好友/黑名单、群成员禁言等投递判断依据。
- aim-logic 不依赖 aim-core 内部状态,也不负责消息投递、Kafka publish、gateway 推送。
- 实时限额拦截仍由 aim-core 热路径 Redis 滑动窗口负责;logic 只持久化计费/配额配置与审计。
