---
name: aim-auth-domain
description: aim 的认证域。对应 `auth` 模块。
---
# aim-auth-domain

## 如何使用

- 涉及数据库(postgres) -> `references/schema.md`
- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`

## 参考资料

- `references/detail.md`
- `references/api.md`

## 最近变更

- 2026-11-01: 迁移到 RocketMQ 事务消息。`UserEventProducerConf`（`Endpoint/Group/Topic`，topic `aim-user-events` 为 TRANSACTION 类型）取代 `KqPusherConf`；`service.UserRegistrar` 先发 half message，再在同一本地事务里写 `user_credentials` 与 `event_journal`（`event_uid = user.created:<id>`）；`OutboxPoller` 与 `outbox_records` 表删除（migration `003_event_journal.sql`）。
- 2026-10-29: 数据访问从 sqlc 迁移到 GORM DAO。`app/auth/rpc/model/dao` 提供 `CreateUser/CreateBotCredential/GetUserByEmail`；`ServiceContext` 改为持有 `*dao.Store`，注册路径改用 RocketMQ 事务消息（`service.UserRegistrar` + `event_journal`）。sqlc 生成代码、`queries/`、`sqlc.yaml` 已删除。
- 2026-06-03: 注册中心从 Nacos 切到 etcd。`app/auth/rpc/auth.go` 启动由 `zrpc.MustNewServer(c.RpcServerConf)` 自动注册 `Etcd.Key: auth.rpc`，进程退出由 `proc.AddWrapUpListener` 自动撤销 lease。`app/shared/nacos` 与 `aimnacos` scheme 整包删除。

- 2026-05-19: 接入 RPC 统一错误拦截器，清洗非业务错误并记录追踪事件。