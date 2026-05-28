---
name: aim-database-migration
description: AIM 数据库迁移规范与 Docker Compose 集成。当修改 migration SQL 文件、新增迁移、修改 docker-compose.yaml 的 migrate 服务、或排查迁移失败时使用。
---

# aim-database-migration

## 背景

AIM 使用 PostgreSQL + sqlc 栈，迁移文件在 `app/{auth,logic}/rpc/model/migrations/` 下，由 Docker Compose 的 `*-migrate` init 容器调用 `deploy/scripts/migrate-postgres.sh` 执行。脚本会显式创建数据库，并按 `NNN_*.sql` 字典序执行目录内所有 SQL，避免在 Compose command 中硬编码迁移文件列表。

## 域特定参考

涉及数据库契约层（migration/sqlc）时，按域选择对应参考：

- 涉及 auth 数据库契约层（migration/sqlc） -> `references/auth-model.md`
- 涉及 logic 数据库契约层（migration/sqlc） -> `references/logic-model.md`

## 迁移文件规范

### 1. 幂等性（最关键）

**所有 DDL 必须可重复执行。** Docker volume 持久化后，`docker compose up --force-recreate` 会重新执行 migrate 容器，如果 SQL 不幂等就会失败。

```sql
-- ✅ 正确：CREATE TABLE IF NOT EXISTS
CREATE TABLE IF NOT EXISTS messages (
    id BIGINT PRIMARY KEY,
    ...
);

-- ❌ 错误：缺少 IF NOT EXISTS，第二次执行必报 "relation already exists"
CREATE TABLE messages (
    id BIGINT PRIMARY KEY,
    ...
);
```

```sql
-- ✅ 正确：CREATE INDEX IF NOT EXISTS
CREATE INDEX IF NOT EXISTS idx_messages_conv_time ON messages(conversation_id, created_at DESC);

-- ❌ 错误：缺少 IF NOT EXISTS
CREATE INDEX idx_messages_conv_time ON messages(conversation_id, created_at DESC);
```

**适用范围：**
- `CREATE TABLE` → 必须 `IF NOT EXISTS`
- `CREATE INDEX` → 必须 `IF NOT EXISTS`
- `CREATE EXTENSION` → 必须 `IF NOT EXISTS`（已遵守）
- `ALTER TABLE ADD COLUMN` → 使用 `IF NOT EXISTS`
- `ALTER TABLE ADD CONSTRAINT` → 先检查 `pg_constraint` 或用 DO 块

### 2. 文件命名

```
NNN_description.sql
```

- 只追加新编号，不改已发布迁移的语义
- 编号从 `000` 开始，顺序递增
- `000_extensions.sql` 固定用于 PostgreSQL 扩展

### 3. 新增迁移后的清单

新增迁移文件后必须同步：

1. ✅ 创建 `NNN_xxx.sql` 文件（幂等 DDL）
2. ✅ 确认文件名使用 `NNN_` 前缀；Compose 通过 `deploy/scripts/migrate-postgres.sh` 自动按序执行，无需手工追加 command
3. ✅ 运行 `sqlc generate` 重新生成 model 代码
4. ✅ 运行 `go test ./app/{module}/rpc/...` 验证

## Docker Compose migrate 服务规范

### 当前架构

```
auth-migrate  → deploy/scripts/migrate-postgres.sh → app/auth/rpc/model/migrations/*.sql
logic-migrate → deploy/scripts/migrate-postgres.sh → app/logic/rpc/model/migrations/*.sql
```

`migrate-postgres.sh` 的关键约定：

- 通过 `AIM_DATABASE` 指定目标库（如 `aim_auth` / `aim_logic`）。
- 使用 `POSTGRES_HOST` / `POSTGRES_PORT` / `POSTGRES_USER` / `POSTGRES_PASSWORD` 连接 PostgreSQL。
- 先显式检查并创建目标数据库，不依赖 `POSTGRES_DB` 初始化副作用。
- 使用 shell glob 按字典序执行 `$AIM_MIGRATIONS_DIR/*.sql`，`NNN_` 前缀保证执行顺序。
- 每个 SQL 使用 `psql -v ON_ERROR_STOP=1`，任一迁移失败即退出。

### 已解决的问题

- ✅ shell 运算符优先级风险：不再在 Compose command 中使用长 `||`/`&&` 链。
- ✅ 数据库创建不一致：auth / logic 都由迁移脚本显式创建。
- ✅ 迁移文件列表硬编码：新增 `011_xxx.sql` 等文件后不需要修改 Compose command。

### 仍需注意

当前脚本仍没有 `schema_migrations` 表，依赖 SQL 自身幂等来支持重复执行。后续如引入 goose / golang-migrate，需要同步更新本 Skill 和 `deploy/scripts/`。

## 检查清单

修改迁移相关内容时，逐项确认：

- [ ] SQL 使用 `IF NOT EXISTS`（CREATE TABLE / CREATE INDEX / CREATE EXTENSION）
- [ ] 新增迁移文件名使用 `NNN_` 前缀，确保 glob 排序正确
- [ ] `deploy/scripts/migrate-postgres.sh` 仍使用显式数据库创建逻辑，未退回 Compose 内联 shell
- [ ] 数据库创建显式化，不依赖 POSTGRES_DB
- [ ] `sqlc generate` 已重新执行
- [ ] `go test` 通过

## 参考资料

- `references/auth-model.md`
- `references/logic-model.md`
