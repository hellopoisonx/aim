# auth 数据库契约层

## 概览

本目录是 auth 的数据库契约层：SQL migration + sqlc query。`*.go` 由 sqlc 生成，业务代码不得手写 model scaffold。

## 结构

```text
model/
├── sqlc.yaml
├── migrations/        # 按编号执行的 PostgreSQL schema migration
├── queries/           # sqlc 查询，生成到同目录 *.sql.go
├── db.go              # sqlc 生成
├── models.go          # sqlc 生成
├── querier.go         # sqlc 生成
└── *.sql.go           # sqlc 生成
```

`model/sqlc.yaml` 固定：PostgreSQL、pgx/v5、`emit_json_tags`、`emit_empty_slices`、`emit_interface`、`emit_pointers_for_null_types`，输出目录为 `.`（即 `model/` 自身）。

## 迁移规则

- 文件名使用 `NNN_description.sql`，只追加新编号，不改已发布迁移语义。
- 表字段时间优先使用 `TIMESTAMPTZ NOT NULL DEFAULT NOW()`。

## 查询规则

- 查询名用动词前缀：`Create*`、`Get*`、`Update*`。
- 修改 SQL 后必须重新生成 `model/`，不要手改生成文件。

## 生成与验证

```bash
cd app/auth/rpc/model
sqlc generate
cd ../../..
go test ./app/auth/rpc/...
```

Docker Compose 的 `auth-migrate` 会按 migrations 文件显式顺序执行；新增迁移后同步更新 `docker-compose.yaml` 中的命令。