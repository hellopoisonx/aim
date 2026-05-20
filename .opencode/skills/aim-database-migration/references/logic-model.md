# logic 数据库契约层

## 概览

本目录是 logic 的数据库契约层：SQL migration + sqlc query。`*.go` 由 sqlc 生成，业务代码不得手写 model scaffold。

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
- `000_extensions.sql` 管扩展；用户昵称模糊检索依赖 `pg_trgm`。
- 消息归档使用 PostgreSQL + JSONB；不要引入独立文档库或向量库。
- `client_msg_id` 用于消息幂等；会话历史分页依赖 `(created_at DESC, id DESC)` 游标。
- 表字段时间优先使用 `TIMESTAMPTZ NOT NULL DEFAULT NOW()`；对外响应再转换 Unix 毫秒。

## 查询规则

- 查询名用动词前缀：`Create*`、`Get*`、`List*`、`Count*`、`Update*`、`Upsert*`。
- 列表查询返回稳定排序；分页查询必须明确 limit 和 cursor 条件。
- 对 nullable 字段依赖 sqlc pointer 输出，不要在业务层猜零值含义。
- 修改 SQL 后必须重新生成 `model/`，不要手改生成文件。

## 生成与验证

```bash
cd app/logic/rpc/model
sqlc generate
cd ../../..
go test ./app/logic/rpc/...
```

Docker Compose 的 `logic-migrate` 会按 migrations 文件显式顺序执行；新增迁移后同步更新 `docker-compose.yaml` 中的命令。