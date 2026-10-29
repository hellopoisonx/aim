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

- 2026-10-29: 数据访问从 sqlc 迁移到 GORM DAO。`app/auth/rpc/model/dao` 提供 `CreateUser/CreateBotCredential/GetUserByEmail` 与 outbox 方法；`ServiceContext` 改为持有 `*dao.Store`，注册事务使用 `DAO.WithTx`。sqlc 生成代码、`queries/`、`sqlc.yaml` 已删除。
- 2026-06-03: 注册中心从 Nacos 切到 etcd。`app/auth/rpc/auth.go` 启动由 `zrpc.MustNewServer(c.RpcServerConf)` 自动注册 `Etcd.Key: auth.rpc`，进程退出由 `proc.AddWrapUpListener` 自动撤销 lease。`app/shared/nacos` 与 `aimnacos` scheme 整包删除。

- 2026-05-19: 接入 RPC 统一错误拦截器，清洗非业务错误并记录追踪事件。