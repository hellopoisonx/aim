# AGENTS.md

**Generated:** 2026-05-20 | **Commit:** 9ee4acf | **Branch:** main

## 语言

- 主/从 agent 必须默认使用 `简体中文` 作为自然语言。
- 写文档、总结、计划、问题澄清时优先中文；代码标识符和命令保持原文。

## 概览

AIM 是多人在线即时通讯系统，内置可自部署 AI 助手。后端为 go-zero 微服务（gateway/auth/core/logic），基础设施包含 Kafka、Redis/Redis Stack、PostgreSQL、etcd、Grafana Tempo。

## 工作流

- 代码更改后必须执行测试、覆盖率测试、lint
- 任务完成后及时更新相关文档 (`skills/`)
- 总结工作区改动并 `git commit`, prompt 必须符合 [conventional](https://www.conventionalcommits.org/en/v1.0.0/) 规范

## 结构

```text
aim/
├── app/auth/rpc/       # 认证
├── app/core/rpc/        # 消息投递
├── app/gateway/api/     # 对外入口
├── app/logic/rpc/       # 业务上下文
├── app/shared/          # 共享库
├── shared/proto/        # Protobuf 协议
├── skills/             # 领域 Skill（.opencode/skills、.pi/skills 均为 junction）
└── docker-compose.yaml  # 本地基础设施
```

## 领域 Skill 导航

领域知识查阅 `skills/` 下对应 Skill：

- aim-repo-mapping: 仓库路由
- aim-auth-domain: 认证域
- aim-core-domain: 消息投递域
- aim-gateway-domain: 网关域
- aim-logic-domain: 业务上下文域
- aim-shared-domain: 共享包域
- aim-proto-domain: Protobuf 协议域
- aim-bot-domain: Bot OpenAPI 域（第三方 Bot 接入、Token、Webhook）
- aim-database-migration: 数据库迁移
- aim-dev-tool: 开发测试工具
- zero-skills: go-zero 框架
- aim-ws-token-management: WebSocket 连接 Token 生命周期管理

## 约定

- Spec-first：REST 先改 `.api` 并 `goctl api validate`；RPC 先改 `.proto` 并用 goctl/protoc 生成。
- go-zero 代码生成使用 `--style go_zero`；不要手写 scaffold，不要让生成路径出现冗余 `github.com/hellopoisonx/aim`。
- 数据模型使用 `sqlc`；SQL 源文件在 `model/{migrations,queries}`，生成代码在 `model/`。
- API/RPC 业务错误使用 `errorx.NewCodeError` / `NewCodeErrorf`；对外基础设施错误清洗为 `internal error`。
- Request 类型必须带 `validate` tag；配置结构体用 `json:",default=value"` / `json:",optional"`。
- core 可以通过 gRPC 调 logic；logic 绝不导入 core，也不负责消息投递。
- Kafka 消息顺序靠 `conversation_id` 作为 key；跨 Kafka 链路通过 payload 中的 `traceparent`/`tracestate` 传播。
- 客户端只和 gateway 通信；客户端通过 REST/WS client 调用 gateway。

## 反模式

- 不要编辑 `*.pb.go`、sqlc 生成文件、goctl 生成的 routes/types/server/client 文件，除非重新生成。
- 不要把内容审核拆成独立微服务；它是 `app/shared/moderation` 进程内共享库。
- 不要用 PostgreSQL 做实时配额拦截；热路径使用 Redis 滑动窗口。
- 不要假设 Redis/RedisBloom 返回类型固定；RESP2/RESP3 可能不同，必须类型分支并加回归测试。
- 不要新增循环依赖；特别是 `app/logic` 不能导入 `app/core`。
- 任务完成后必须清理临时文件（如覆盖率输出 `coverage.out`、`coverage.html`、`*.tmp`、`*.bak` 等），不要将它们留在工作目录或提交到版本控制。

## 命令

```bash
go mod tidy
go build ./...
docker compose up -d postgres redis kafka etcd tempo grafana
```
