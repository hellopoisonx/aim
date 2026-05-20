# AIM

`AIM` 是一个面向多人在线的即时通讯系统，内置可自部署的 AI 助手，将大模型能力深度集成到聊天场景中。

## 文档

技术文档由 agent + skills 维护，详见 `.opencode/skills/`：

| 技能 | 对应模块 |
|------|----------|
| `aim-repo-mapping` | 仓库导航（模块边界、入口、影响面） |
| `aim-auth-domain` | 认证服务 |
| `aim-core-domain` | 消息投递域 |
| `aim-gateway-domain` | 网关/连接层 |
| `aim-logic-domain` | 业务上下文域 |
| `aim-frontend-domain` | 桌面客户端 |
| `aim-shared-domain` | 进程内共享包 |
| `aim-proto-domain` | Protobuf 协议 |
| `aim-database-migration` | 数据库迁移 |
| `zero-skills` | go-zero 框架 |

## 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 微服务框架 | `go-zero` v1.10 | HTTP/gRPC 骨架 + goctl 代码生成 |
| WebSocket | `coder/websocket` | Protobuf 帧协议 |
| 消息队列 | Kafka（`go-queue` → `segmentio/kafka-go`） | `conversation_id` 分区保序 |
| 缓存 | Redis Stack（`go-redis/v9`） | 在线状态、网关路由、滑动窗口限频 |
| 持久化 | PostgreSQL 17 + pgvector | JSONB 文档存储、向量检索（扩展） |
| 注册中心 | Nacos v2 | 服务发现与配置管理 |
| 链路追踪 | OpenTelemetry → Jaeger | gRPC + Kafka trace 传播 |
| 数据模型 | sqlc | 类型安全的 SQL 生成 |
| 序列化 | Protobuf + gRPC | WS 帧协议 / 服务间通信 |
| 桌面客户端 | Wails v2 + Vue 3 + Element Plus + Vite | 桌面端 UI |
| 容器化 | Docker Compose | 本地基础设施 + 服务编排 |

## 架构总览

```
┌──────────┐     ┌──────────────┐
│ Desktop  │     │   Mobile /   │
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

**依赖方向**：aim-core → aim-logic（单向）。aim-core 通过 gRPC + Redis 缓存查询 aim-logic 的关系数据，aim-logic 不反向依赖 aim-core。

## 模块

> 详细接口定义和实现指南见 `.opencode/skills/` 对应技能。

### aim-gateway — 连接层 `aim-gateway-domain`

WebSocket 长连接管理、Protobuf 帧解析、心跳保活。通过 Redis 维护用户→网关节点映射，支持 PushMessage/PushPresence/KickUser 等内部 gRPC 操作，节点下线时推送 `reconnect` 帧做 drain。

### aim-auth — 认证服务 `aim-auth-domain`

JWT 签发/验证/刷新，基于 Redis SessionStore 的多设备登录管理。

### aim-core — 消息投递域 `aim-core-domain`

消息路由与投递：Transfer（gRPC → Kafka 发布）→ Delivery Consumer（Kafka 消费 → 查目标节点 → gRPC 推送）。PresenceStore 维护 Redis 在线状态。

### aim-logic — 业务上下文域 `aim-logic-domain`

提供投递判断依据：用户/好友/群组管理、消息持久化（Kafka 消费 → PostgreSQL JSONB）、历史回溯（cursor-based）、Redis 滑动窗口限频。内容审核作为共享库接口供 core 同步调用。

### aim-ai — AI 能力域（规划中）

Bot 控制、LLM 适配、RAG 知识库、工具执行。通过 Kafka 与 core 解耦。

### app/shared — 共享包 `aim-shared-domain`

进程内共享库：`errorx`（业务错误码）、`jwt`、`events`、`nacos`（注册/发现）、`tracing`（W3C 传播）、`tools`（snowflake ID）、`moderation`（审核接口）。

## 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| core/logic 边界 | 按领域所有权划分 | 单向依赖，core 只投递，logic 提供上下文 |
| 内容审核 | 共享库（进程内同步调用） | 避免 RPC 延迟，热路径同步 |
| 消息持久化 | PostgreSQL JSONB | 零额外组件，灵活 schema |
| 向量检索 | pgvector 扩展 | 零运维增量 |
| 实时限额 | Redis 滑动窗口 | PostgreSQL 往返不满足逐消息拦截的延迟 |
| 消息顺序 | Kafka 按 `conversation_id` 分区 | 同会话有序，跨会话并行 |
| 链路追踪 | OpenTelemetry → Jaeger | gRPC + Kafka trace context 全链路传播 |

## 消息投递流程（单聊）

```
1. 客户端 → aim-gateway（WS Protobuf 帧）
2. aim-gateway → aim-auth（Token 校验）
3. aim-gateway → aim-core/Transfer（gRPC）
4. Transfer → aim-logic（权限检查：是否好友/被拉黑）
5. Transfer → Kafka（发布，key=conversation_id）
6. Delivery Consumer ← Kafka（消费）
7. Delivery Consumer → Redis（查目标用户网关节点）
8. Delivery Consumer → aim-gateway（gRPC PushMessage）
9. aim-gateway → 客户端 B（WS 推送）
10. Archive Consumer ← Kafka（异步持久化至 PostgreSQL）
```

