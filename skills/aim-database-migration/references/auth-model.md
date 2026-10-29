# auth 数据库契约层

## 概览

`app/auth/rpc/model/` 现在是 **migration SQL + GORM ORM 结构体 + GORM DAO** 三层：

- `migrations/`：schema 唯一事实来源，由 `auth-migrate` 容器执行。
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
- 表字段时间优先使用 `TIMESTAMPTZ NOT NULL DEFAULT NOW()`。
- 改表后必须同步 `orm/` 与 `dao/`，并跑 `go test ./app/auth/...`。

## DAO 规则

- DAO 方法按聚合命名：`CreateUser`、`GetUserByEmail`、`InsertOutboxRecord` 等。
- 业务层不得直接使用 `*gorm.DB` 或手写 SQL。
- 事务统一用 `(*dao.Store).WithTx(ctx, func(tx *dao.Store) error { ... })`。
- 唯一冲突/记录不存在通过 `dao.ErrDuplicateKey` / `dao.ErrNotFound` 暴露。
- 复杂 SQL（聚合、`RETURNING`、`ON CONFLICT`）允许在 DAO 内使用 `gorm.Raw`/`Exec`，但不得泄漏到业务层。

## 验证

```bash
go test ./app/auth/...
```

Docker Compose 的 `auth-migrate` 调用 `deploy/scripts/migrate-postgres.sh`，按 `migrations/*.sql` 字典序执行；新增迁移只需使用正确的 `NNN_` 前缀，无需手工更新 Compose command。
