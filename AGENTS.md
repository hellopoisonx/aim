# AGENTS.md

## 必须在所有操作之前先阅读此文档

## 项目概览

AIM — multi-user real-time IM system with self-deployed AI assistant. Go-zero microservices (gateway/auth/core/logic/ai) + Kafka + Redis + PostgreSQL/pgvector + Nacos.

## 工作流程
1. 理解需求
2. 定位代码
3. 生成计划
4. 执行代码审查
5. 更新仓库 skills

## 约束

- **golangci-lint**: Refer to `golang-lint` skill
- **Spec-first development**: Always create `.api` spec before code; validate with `goctl api validate`
- **TDD**: Refer to `test-driven-development` and `golang-testing` skill, the coverage should be above 80%
- **goctl for generation**: Use `goctl api go` / `goctl rpc protoc` / `goctl docker` — never handwritten scaffolding
- **model**: Use `sqlc` — never handwritten scaffolding
- **Post-generation**: Always `go mod tidy` → verify imports → `go build ./...`
- **Naming style**: `--style go_zero` (snake_case filenames, go_zero naming)
- **Error pattern**: `errorx.NewCodeError(code, msg)` — not `fmt.Errorf`
- **Validation**: `validate:"required,email"` tags on request structs
- **Context first**: `func(ctx context.Context, req *types.Request)`
- **Config**: `json:",default=value"` for config defaults
- **Dependency direction**: aim-core → aim-logic (unidirectional); logic never imports core

## 反设计的模式

- 一定要执行测试覆盖率检查 至少80%以上
- 永远采用 `TDD` 开发模式
- 虽然本项目的模块路径为 `github.com/hellopoisonx/aim`，但是对于 `goctl` 生成的文件路径不应出现冗余的 `github.com/hellopoisonx/aim` 路径
- 必须参考 `golang-modernize` 去除传统风格的 API
- 对于 `Request` 请求类型不要遗漏 `validation` tags
- 对于 API errors 不要使用 `fmt.Errorf`, 应当使用 `errorx.NewCodeError`
- 不要手动展开 go-zero 相关的代码, 使用 `goctl`
- 代码生成或修改后不要跳过 mod tidy, build verify, test, coverage test, vet test, lint
- 不要造成循环依赖
- 不要生成毫无根据、无法溯源的代码。每此生成/更改代码后必须在 `.opencode/skills/` 目录下生成/更新对应的文档
- 不要假设 Redis/RedisBloom 命令在不同协议版本下的返回类型固定。例如 `BF.EXISTS` 在 RESP2 下可能返回 `int64(0/1)`，在 RESP3/go-redis 下可能返回 `bool`；必须用类型分支兼容并添加回归测试，避免将本地解码错误折叠成难排查的 `internal error`

## UNIQUE STYLES

- Content moderation = shared library (in-process), NOT separate microservice
- Real-time quota enforcement via Redis sliding window (not PostgreSQL)
- Consistent hashing with 150+ virtual nodes for gateway routing
- Session drain: 5-10s reconnect window on gateway node shutdown
- Message ordering: Kafka keyed by `conversation_id` partition
- PostgreSQL partitioned tables + JSONB for message archive
- pgvector extension for RAG/vector search (no separate vector DB)

## COMMANDS

```bash
# Scaffolding
goctl api new <service> --style go_zero     # New API service
goctl rpc new <service> --style go_zero     # New RPC service
goctl api go -api <file>.api -dir . --style go_zero  # Gen from spec
goctl rpc protoc <file>.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero

# Build
go mod tidy && go build ./...

# Validate
goctl api validate -api <file>.api

# test
go test ./...

# coverage test
go test -cover ./...
```

## NOTES

- `go.mod` declares `go 1.26` — verify Go version compatibility
- `.gitignore` excludes `/AGENTS.md` — this file is for local AI context only
