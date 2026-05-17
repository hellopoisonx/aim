# 模块需求定义

提供消息投递的判断依据和业务逻辑支撑。

- User/Relationship Service（用户与社交）：持有用户资料表、好友申请、黑名单、群组元数据、群成员禁言/转让
- Message Archive Service（消息持久化）：从 Kafka 异步消费消息，写入 PostgreSQL 分区表 + JSONB；提供 tsvector 全文搜索、历史回溯接口
- Billing & Quota Service（计费管理）：平台点数扣费、计费流水审计（PostgreSQL 持久化）；实时限额通过 Redis 滑动窗口实现，避免 PostgreSQL 往返延迟
- Content Moderation（内容审核）：作为共享库（in-process）供 aim-core 和 aim-ai 同步调用；异步审计日志由独立 worker 处理

## 当前落地范围

- `app/logic/rpc/logic.proto` 定义 `PermissionService`。
- `CheckMessagePermission` 是 aim-core 在接受消息进入 Kafka 前查询的业务上下文接口。
- 当前实现通过 `PermissionChecker` 接口隔离关系/群组策略，默认 scaffold 使用 deny-all fail-closed；真实用户、好友、黑名单、群成员禁言数据模型后续通过 sqlc 接入后替换默认 checker。

## 边界

- aim-core 可以调用 aim-logic：查询会话成员、好友/黑名单、群成员禁言等投递判断依据。
- aim-logic 不依赖 aim-core 内部状态，也不负责消息投递、Kafka publish、gateway 推送。
- 实时限额拦截仍由 aim-core 热路径 Redis 滑动窗口负责；logic 只持久化计费/配额配置与审计。
