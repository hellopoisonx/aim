# AIM

`AIM` 是一个面向多人在线的即时通讯系统，内置可自部署的 AI 助手，将大模型能力深度集成到聊天场景中，实现"通讯 + AI"的深度融合。

## 技术栈

| 技术        | 依赖库                                        | 目标                     |
|-----------|--------------------------------------------|------------------------|
| WebSocket | `github.com/coder/websocket`               | 全双工长连接通信               |
| 微服务框架     | `github.com/zeromicro/go-zero`             | HTTP/gRPC 微服务骨架        |
| 序列化       | `protobuf`                                 | 前后端 WS 通信格式 / 服务内部调用格式 |
| 桌面客户端     | `wails` + `vue3` + `element-plus` + `vite` | 桌面端 UI                 |
| 消息队列      | `kafka`                                    | 异步消息投递、服务解耦            |
| 缓存/分布式锁   | `redis`                                    | 在线状态、会话路由、实时限额、分布式锁    |
| 配置/注册中心   | `github.com/nacos-group/nacos-sdk-go/v2`   | 服务发现、配置管理              |
| 持久化       | `postgresql` + `pgvector`                  | 关系数据 / JSONB 文档 / 向量检索 |
| 容器化       | `docker`                                   | 部署                     |

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
       │  aim-gateway    │  一致性哈希 + 虚拟节点 + 会话 drain
       │  (有状态网关)    │
       └───────┬────────┘
               │  gRPC
     ┌─────────▼──────────────┐
     │                         │
 ┌───▼────┐             ┌──────▼─────┐
 │aim-auth│             │  aim-core  │
 │(认证)   │             │ (消息投递域) │
 └───┬────┘             └──────┬─────┘
     │                        │  gRPC（缓存 aim-logic 数据）
     │              ┌─────────▼──────────┐
     │              │      Kafka          │
     │              └────┬──────────┬─────┘
     │                   │          │
     │           ┌───────▼──┐  ┌────▼──────┐
     │           │aim-logic │  │  aim-ai   │
     │           │(业务上下文)│  │(AI 能力域) │
     │           └──────────┘  └───────────┘
     │                ▲
     └────────────────┘  （aim-logic 不依赖 aim-core 内部状态）
```

**依赖方向**：aim-core → aim-logic（单向）。aim-core 通过缓存（Redis TTL）查询 aim-logic 的好友/群组关系来路由消息，aim-logic 不反向依赖 aim-core。

## 模块划分

### aim-gateway — 连接层

维护长连接（WebSocket / TCP）、Protobuf 协议解析、心跳保活。

- 有状态服务，根据 User_ID 做一致性哈希，确保同一用户落在固定网关节点
- 使用 150+ 虚拟节点/物理节点，减少 rebalancing 影响
- 节点下线前推送 `reconnect` 帧，提供 5-10s drain 窗口，避免惊群重连

### aim-auth — 认证服务

独立的认证服务，位于网关之后、业务服务之前。

- JWT 签发与刷新
- 多设备登录策略（单设备踢下线 / 多设备共存）

### aim-core — 消息投递域

只负责一件事：**把消息送到对的人**。

- **Transfer Service（消息路由）**：消息流向判断（单聊/群聊）、查询接收方所在网关节点、投递至 Kafka
- **Presence Service（在线状态）**：Redis heartbeat 维护用户在线/离线/输入中状态，向好友推送状态变更
- **Delivery Consumer（投递消费者）**：从 Kafka 消费消息，查找目标用户所在网关并投递

> 依赖方向：aim-core → aim-logic（gRPC 查询好友/群组关系，本地 Redis 缓存短期 TTL）

### aim-logic — 业务上下文域

提供消息投递的判断依据和业务逻辑支撑。

- **User/Relationship Service（用户与社交）**：好友申请、黑名单、群组元数据、群成员禁言/转让
- **Message Archive Service（消息持久化）**：从 Kafka 异步消费消息，写入 PostgreSQL 分区表 + JSONB；提供 `tsvector` 全文搜索、历史回溯接口
- **Billing & Quota Service（计费管理）**：平台点数扣费、计费流水审计（PostgreSQL 持久化）；**实时限额**通过 Redis 滑动窗口实现，避免 PostgreSQL 往返延迟
- **Content Moderation（内容审核）**：作为**共享库**（in-process）供 aim-core 和 aim-ai 同步调用；异步审计日志由独立 worker 处理

### aim-ai — AI 能力域

通过 Kafka 与 aim-core 解耦；LLM 供应商宕机不影响消息投递。

- **Bot Controller（机器人大脑）**：管理 Bot 人设、多 Bot 协作路由，通过 MQ 消费 @消息，决定调用哪个 Bot
- **LLM Connector（模型适配器）**：屏蔽 OpenAI / Anthropic 等厂商接口差异，处理流式输出（SSE / WS 回传）、Token 计数
- **RAG & Vector Service（知识库服务）**：文档切片、向量化（Embedding）、pgvector 存储与相似度检索
- **Task/MCP Service（工具执行器）**：执行工具调用

### 暂不考虑

- ~~Push Service（离线推送）~~：对接 FCM / APNs 的系统通知
- ~~可观测性~~：分布式追踪、指标采集、日志聚合

## 关键设计决策

| 决策            | 选择                         | 理由                                   |
|---------------|----------------------------|--------------------------------------|
| core/logic 边界 | 按领域所有权划分（非实时/非实时）          | 避免双向依赖；aim-core 只管投递，aim-logic 提供上下文 |
| 内容审核          | 共享库 + async worker（非独立服务）  | 热路径同步调用避免 RPC 延迟，审计异步解耦              |
| 消息持久化         | PostgreSQL 分区表 + JSONB     | MVP 阶段零额外组件；分区按月滚动，JSONB 存非结构化字段     |
| 向量检索          | pgvector 扩展                | 零运维增量，无需单独部署向量数据库                    |
| 实时限额          | Redis 滑动窗口                 | PostgreSQL 往返不满足逐条消息拦截的延迟要求          |
| 消息顺序          | Kafka 按 conversation_id 分区 | 保证同一会话内消息有序，跨会话并行消费                  |
| 一致性哈希         | 150+ 虚拟节点 + 会话 drain       | 减少节点增减时的连接迁移风暴                       |

## 消息投递流程（单聊示例）

```
1. 客户端 → aim-gateway（WS 发送 Protobuf 消息）
2. aim-gateway → aim-auth（校验 token & 权限）
3. aim-gateway → aim-core/Transfer（gRPC 转发消息）
4. Transfer → aim-logic（缓存查询：A 和 B 是否好友？）
5. Transfer → Kafka（投递消息，key = conversation_id）
6. Delivery Consumer → Kafka（消费消息）
7. Delivery Consumer → Redis（查询 B 在哪个网关节点）
8. Delivery Consumer → aim-gateway（gRPC 投递至 B 的连接节点）
9. aim-gateway → 客户端 B（WS 推送）
10. Transfer → aim-logic/Message Archive（异步持久化）
```

