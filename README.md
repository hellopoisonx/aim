# AIM

面向多人在线的即时通讯系统，内置可自部署的 AI 助手。

## 快速开始

```bash
# 启动本地 Docker 环境（核心服务 + 基础设施，端口仅绑定 127.0.0.1）
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --build

# 可选：启动 Prometheus / Loki / Promtail / Grafana
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  -f deploy/compose/observability.yaml \
  up -d

# 构建
go mod tidy
go build ./...

# 按需本地 go run 单服务（app/*/etc/*.yaml 保留为单服务默认配置）
cd app/gateway/api && go run gateway.go
cd app/auth/rpc && go run auth.go
cd app/core/rpc && go run core.go
cd app/logic/rpc && go run logic.go
```

部署拆分说明见 `deploy/README.md`。根目录 `docker-compose.yaml` 仅作为本地兼容入口保留。

## 架构

```
┌──────────┐     ┌──────────────┐
│  Client  │     │    Web       │
└────┬─────┘     └──────┬───────┘
     │  WS/Protobuf      │
     └─────────┬─────────┘
               │
       ┌───────▼────────┐
       │  aim-gateway   │  WebSocket + 会话管理 + drain
       │   (有状态网关)  │
       └───────┬────────┘
               │  gRPC
     ┌─────────▼──────────────┐
     │                         │
 ┌───▼────┐             ┌──────▼─────┐
 │aim-auth│             │  aim-core  │
 │ (认证) │             │ (消息投递域) │
 └───┬────┘             └──────┬─────┘
     │                        │  gRPC
     │              ┌─────────▼──────────┐
     │              │      Kafka          │
     │              └────┬──────────┬─────┘
     │                   │          │
     │           ┌───────▼──┐  ┌────▼──────┐
     │           │aim-logic │  │  aim-ai   │
     │           │(业务上下文)│  │  (规划中)  │
     │           └──────────┘  └───────────┘
     │                ▲
     └────────────────┘  aim-core → aim-logic（单向依赖）
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 微服务框架 | go-zero |
| WebSocket | coder/websocket（Protobuf 帧协议） |
| 消息队列 | Kafka（conversation_id 分区保序） |
| 缓存 | Redis Stack |
| 持久化 | PostgreSQL + pgvector |
| 文件存储 | SeaweedFS |
| 注册中心 | Nacos |
| 链路追踪 | OpenTelemetry → Grafana Tempo（Grafana Explore 查询） |
| 指标采集 | Prometheus（go-zero 内置） → Grafana 仪表盘 |
| 日志聚合 | Loki + Promtail（Docker stdout JSON） → Grafana |
| 数据模型 | sqlc |

## 模块

| 模块 | 说明 | Skill |
|------|------|-------|
| aim-gateway | 连接层（WebSocket + 会话管理） | `aim-gateway-domain` |
| aim-auth | JWT 认证、多设备登录 | `aim-auth-domain` |
| aim-core | 消息路由与投递 | `aim-core-domain` |
| aim-logic | 用户/好友/群组、历史消息 | `aim-logic-domain` |
| aim-attachment | 附件上传/下载授权 | `aim-attachment-domain` |
| aim-data-parsing | 附件元数据提取 | `aim-data-parsing-domain` |
| app/shared | 共享库（errorx/jwt/tracing 等） | `aim-shared-domain` |
| dev-tool | 开发测试工具 | `aim-dev-tool` |

> 详细文档见 `skills/` 下对应 Skill。

## 开发指南

- **Spec-first**：REST 先改 `.api`，RPC 先改 `.proto`
- 代码生成使用 goctl / protoc / sqlc；不手写生成文件
- 业务错误使用 `errorx.NewCodeError`
- core → logic 单向依赖；logic 绝不导入 core
- 客户端只与 gateway 通信

详见 `AGENTS.md`。
