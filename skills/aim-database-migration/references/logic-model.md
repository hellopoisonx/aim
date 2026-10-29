# logic 数据库契约层

## 概览

`app/logic/rpc/model/` 现在是 **migration SQL + GORM ORM 结构体 + GORM DAO** 三层：

- `migrations/`：schema 唯一事实来源，由 `logic-migrate` 容器执行。
- `orm/`：GORM 结构体，与迁移 SQL 逐列对齐；禁止 `AutoMigrate`。
- `dao/`：唯一允许写查询/SQL 的位置，业务层只依赖 `*dao.Store`。

sqlc 已移除：不要新增 `sqlc.yaml`、`queries/` 或 `*.sql.go`。

## 结构

```text
model/
├── migrations/        # 按编号执行的 PostgreSQL schema migration
├── orm/               # GORM 结构体（表映射，不生成 DDL）
└── dao/               # GORM DAO（业务唯一数据访问入口）
```

## 迁移规则

- 文件名使用 `NNN_description.sql`，只追加新编号，不改已发布迁移语义。
- `000_extensions.sql` 管扩展；用户昵称/消息检索依赖 `pg_trgm`。
- 消息归档使用 PostgreSQL + JSONB；不要引入独立文档库或向量库。
- `client_msg_id` 用于消息幂等；会话历史分页依赖 `(created_at DESC, id DESC)` 游标。
- 表字段时间优先使用 `TIMESTAMPTZ NOT NULL DEFAULT NOW()`；对外响应再转换 Unix 毫秒。

## 迁移文件清单

| 编号 | 文件名 | 说明 |
|------|--------|------|
| 000 | `000_extensions.sql` | PostgreSQL 扩展（pg_trgm 等） |
| 001 | `001_initial_permission.sql` | 好友关系表、会话表、会话成员表 |
| 002 | `002_messages.sql` | 消息表（含 client_msg_id 幂等索引） |
| 003 | `003_user_info.sql` | 用户资料表 |
| 004 | `004_group_management.sql` | 群管理：conversations 新增 name/avatar/creator_id；conversation_members 新增 role |
| 005 | `005_conversation_read_states.sql` | 会话已读游标 |
| 006 | `006_user_type.sql` | human/bot/system 身份标记 |
| 007 | `007_bot_tokens.sql` | Bot Token |
| 008 | `008_bot_webhooks.sql` | Bot Webhook |
| 009 | `009_bot_actions.sql` | Bot Action / Token 权限 / 事件映射 |
| 010 | `010_bot_attachment_actions.sql` | 附件相关 Bot action |
| 011 | `011_bot_conversation_read_actions.sql` | 已读相关 Bot action |
| 012 | `012_friend_tags_and_search.sql` | 好友标签与搜索索引 |
| 013 | `013_user_bots.sql` | 用户侧 Bot 所有权 |
| 014 | `014_outbox_records.sql` | 事务发件箱 |

### 004_group_management.sql 详细

```sql
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS avatar VARCHAR(512) NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS creator_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS role VARCHAR(16) NOT NULL DEFAULT 'member';
```

| 表 | 字段 | 类型 | 默认值 | 说明 |
|----|------|------|--------|------|
| conversations | `name` | VARCHAR(128) | `''` | 会话名称；群聊为群名 |
| conversations | `avatar` | VARCHAR(512) | `''` | 群聊头像 URL |
| conversations | `creator_id` | BIGINT | `0` | 创建者/群主 |
| conversation_members | `role` | VARCHAR(16) | `'member'` | 取值：owner / admin / member |

Docker Compose 的 `logic-migrate` 调用 `deploy/scripts/migrate-postgres.sh`，会按 `migrations/*.sql` 字典序执行所有迁移；Kafka topic 由 `deploy/scripts/init-kafka-topics.sh` 统一创建，包含 `aim.conversation.events`。

## DAO 规则

- DAO 方法按聚合命名：`CreateConversation`、`GetConversationMembers`、`UpsertFriendship`、`SearchMessagesGlobal` 等。
- 列表查询返回稳定排序；分页查询必须明确 limit 和 cursor 条件。
- nullable 列在 `orm/` 中用指针表达，业务层用 `nil` 判断，不要猜零值含义。
- 复杂查询（全文检索、`DISTINCT ON`、`UPDATE ... FROM ... RETURNING`）在 DAO 内用 `Raw`/`Exec` 实现。
- 业务层不得直接使用 `*gorm.DB` 或手写 SQL；事务统一用 `(*dao.Store).WithTx`。
- 唯一冲突/记录不存在通过 `dao.ErrDuplicateKey` / `dao.ErrNotFound` 暴露。

## 验证

```bash
go test ./app/logic/...
```

Docker Compose 的 `logic-migrate` 通过 `deploy/scripts/migrate-postgres.sh` 自动按 `NNN_` 前缀顺序执行迁移；新增迁移无需手工更新 Compose command。
