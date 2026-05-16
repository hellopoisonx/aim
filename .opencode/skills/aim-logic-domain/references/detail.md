# 模块需求定义

提供消息投递的判断依据和业务逻辑支撑。

- User/Relationship Service（用户与社交）：持有用户资料表、好友申请、黑名单、群组元数据、群成员禁言/转让
- Message Archive Service（消息持久化）：从 Kafka 异步消费消息，写入 PostgreSQL 分区表 + JSONB；提供 tsvector 全文搜索、历史回溯接口
- Billing & Quota Service（计费管理）：平台点数扣费、计费流水审计（PostgreSQL 持久化）；实时限额通过 Redis 滑动窗口实现，避免 PostgreSQL 往返延迟
- Content Moderation（内容审核）：作为共享库（in-process）供 aim-core 和 aim-ai 同步调用；异步审计日志由独立 worker 处理