# AGENTS.md

**Generated:** 2026-11-01 | **Commit:** e34920d | **Branch:** v2

## 语言

- 主/从 agent 必须默认使用 `简体中文` 作为自然语言。
- 写文档、总结、计划、问题澄清时优先中文；代码标识符和命令保持原文。

## 概览

AIM 是多人在线即时通讯系统，内置可自部署 AI 助手。后端为 go-zero 微服务（gateway/auth/core/logic/attachment/data_parsing），基础设施包含 RocketMQ 5（namesrv + broker + gRPC proxy）、Redis/Redis Stack、PostgreSQL、SeaweedFS、etcd、Grafana Tempo。

## 工作流

- 代码更改后必须执行测试、覆盖率测试、lint
- 任务完成后及时更新相关文档 (`skills/`)
- 总结工作区改动并 `git commit`, prompt 必须符合 [conventional](https://www.conventionalcommits.org/en/v1.0.0/) 规范

## 结构

```text
aim/
├── app/auth/rpc/        # 认证
├── app/core/rpc/        # 消息投递
├── app/gateway/api/     # 对外入口
├── app/logic/rpc/       # 业务上下文
├── app/attachment/      # 附件服务
├── app/data_parsing/    # 数据解析
├── app/shared/          # 共享库
├── bot_sdk/             # Bot Go SDK
├── dev-tool/            # 调试压测工具
├── deploy/              # 部署编排
├── docs/                # 文档与协议规范
├── shared/proto/        # Protobuf 协议
├── skills/              # 领域 Skill（.opencode/skills、.pi/skills 均为 junction）
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
- aim-attachment-domain: 附件上传/下载/存储域
- aim-data-parsing-domain: 附件媒体数据解析域

## 约定

- Spec-first：REST 先改 `.api` 并 `goctl api validate`；RPC 先改 `.proto` 并用 goctl/protoc 生成。
- go-zero 代码生成使用 `--style go_zero`；不要手写 scaffold，不要让生成路径出现冗余 `github.com/hellopoisonx/aim`。
- 数据模型使用 GORM：迁移 SQL 在 `app/*/model/migrations`，结构体在 `model/orm`，查询在 `model/dao`；禁止 `AutoMigrate`。
- API/RPC 业务错误使用 `errorx.NewCodeError` / `NewCodeErrorf`；对外基础设施错误清洗为 `internal error`。
- Request 类型必须带 `validate` tag；配置结构体用 `json:",default=value"` / `json:",optional"`。
- core 可以通过 gRPC 调 logic；logic 绝不导入 core，也不负责消息投递。
- 有序投递靠 FIFO topic + `conversation_id` 作为 MessageGroup/ShardingKey；跨消息链路通过 payload 中的 `traceparent`/`tracestate` 传播。
- 需要与数据库事务一致的事件走 RocketMQ 事务消息（`app/shared/rocketmq`）：本地事务同时写 `event_journal`，broker 回查据此决定 commit/rollback；不要再引入发件箱轮询。
- 客户端只和 gateway 通信；客户端通过 REST/WS client 调用 gateway。

## 反模式

- 不要编辑 `*.pb.go`、goctl 生成的 routes/types/server/client 文件，除非重新生成；DAO 查询只写在 `model/dao`。
- 不要把内容审核拆成独立微服务；它是 `app/shared/moderation` 进程内共享库。
- 不要用 PostgreSQL 做实时配额拦截；热路径使用 Redis 滑动窗口。
- 不要假设 Redis/RedisBloom 返回类型固定；RESP2/RESP3 可能不同，必须类型分支并加回归测试。
- 不要新增循环依赖；特别是 `app/logic` 不能导入 `app/core`。
- 任务完成后必须清理临时文件（如覆盖率输出 `coverage.out`、`coverage.html`、`*.tmp`、`*.bak` 等），不要将它们留在工作目录或提交到版本控制。

## 命令

```bash
go mod tidy
go build ./...
docker compose up -d postgres redis rocketmq-namesrv rocketmq-broker rocketmq-proxy rocketmq-init etcd tempo grafana
```
