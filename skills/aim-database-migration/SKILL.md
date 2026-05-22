---
name: aim-database-migration
description: AIM 数据库迁移规范与 Docker Compose 集成。当修改 migration SQL 文件、新增迁移、修改 docker-compose.yaml 的 migrate 服务、或排查迁移失败时使用。
---

# aim-database-migration

## 背景

AIM 使用 PostgreSQL + sqlc 栈，迁移文件在 `app/{auth,logic}/rpc/model/migrations/` 下，由 Docker Compose 的 `*-migrate` init 容器执行。当前实现存在若干问题，本文档记录经验教训和规范。

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
2. ✅ 更新 `docker-compose.yaml` 中对应 `*-migrate` 服务的 `command`，追加新文件
3. ✅ 运行 `sqlc generate` 重新生成 model 代码
4. ✅ 运行 `go test ./app/{module}/rpc/...` 验证

## Docker Compose migrate 服务规范

### 当前架构

```
auth-migrate  → psql -f /migrations/000_init.sql
logic-migrate → CREATE DATABASE aim_logic && psql -f 000 && psql -f 001 && ...
```

### 已知问题与修复方向

#### 问题 1：Shell 运算符优先级（logic-migrate）

**现状：**
```bash
psql -c "SELECT 1 FROM pg_database WHERE datname = 'aim_logic'" | grep -q 1 \
  || psql -c "CREATE DATABASE aim_logic" \
  && psql -f /migrations/000_extensions.sql \
  && psql -f /migrations/001_initial_permission.sql \
  && ...
```

**风险：** 如果 `CREATE DATABASE` 失败，`&&` 仍会尝试执行后续 psql -f，对不存在的库执行 SQL，产生误导性错误信息。

**修复方案：** 用显式 `if/then` 代替 `||`/`&&` 链：
```bash
if ! psql -h postgres -U user -d postgres -c "SELECT 1 FROM pg_database WHERE datname = 'aim_logic'" | grep -q 1; then
  psql -h postgres -U user -d postgres -c "CREATE DATABASE aim_logic" || exit 1
fi
psql -h postgres -U user -d aim_logic -f /migrations/000_extensions.sql || exit 1
psql -h postgres -U user -d aim_logic -f /migrations/001_initial_permission.sql || exit 1
# ... 后续迁移
```

#### 问题 2：数据库创建不一致

- `auth-migrate`：依赖 postgres 服务的 `POSTGRES_DB: aim_auth` 隐式创建
- `logic-migrate`：显式 `CREATE DATABASE aim_logic`

**规范：** 所有数据库应显式创建，不依赖 `POSTGRES_DB` 环境变量。`POSTGRES_DB` 仅用于 postgres 初始化时的默认库，不应作为迁移依赖。

#### 问题 3：迁移文件列表硬编码

每次新增迁移都要手动更新 docker-compose.yaml 的 command 字符串。

**可选改进方案：**

- **方案 A（推荐 - 最小改动）：** 保持当前显式列举方式，但在 AGENTS.md 中设置 checklist 提醒
- **方案 B：** 使用 shell glob 按序执行：
  ```bash
  for f in /migrations/*.sql; do psql -h postgres -U user -d aim_logic -f "$f" || exit 1; done
  ```
  注意：需要确保文件名排序正确（`NNN_` 前缀保证字典序 = 执行序）
- **方案 C：** 引入迁移工具（golang-migrate、goose），自带追踪和幂等执行

#### 问题 4：无迁移追踪

当前没有 `schema_migrations` 表，无法判断哪些迁移已执行。

**影响：**
- 无法安全地回滚/重跑
- 部分失败后无法确定恢复点
- 幂等 SQL 是"穷人的追踪"，仅能防止重复建表，不能防止数据迁移重复执行

**方案 C（迁移工具）可彻底解决此问题。**

### auth-migrate 与 logic-migrate 的差异

| 维度 | auth-migrate | logic-migrate |
|------|-------------|---------------|
| 数据库创建 | 隐式（POSTGRES_DB） | 显式（CREATE DATABASE） |
| 迁移文件数 | 1 个 | 4 个 |
| SQL 幂等性 | ❌ 缺少 IF NOT EXISTS | ✅ 表有 IF NOT EXISTS，❌ 部分索引缺少 |
| Shell 安全性 | ✅ 简单命令 | ❌ 运算符优先级风险 |

## 检查清单

修改迁移相关内容时，逐项确认：

- [ ] SQL 使用 `IF NOT EXISTS`（CREATE TABLE / CREATE INDEX / CREATE EXTENSION）
- [ ] 新增迁移文件后同步更新 docker-compose.yaml 的 command
- [ ] logic-migrate 的 shell 命令使用 `if/then` 而非 `||`/`&&` 链
- [ ] 数据库创建显式化，不依赖 POSTGRES_DB
- [ ] `sqlc generate` 已重新执行
- [ ] `go test` 通过

## 参考资料

- `references/auth-model.md`
- `references/logic-model.md`
