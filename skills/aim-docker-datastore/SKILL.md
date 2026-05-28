---
name: aim-docker-datastore
description: AIM 本地 Docker 数据存储运维 Skill。Use when the user asks to inspect, connect to, query, debug, or verify Docker Compose Kafka, Redis, or PostgreSQL in this project; proactively call aim_docker_status, aim_kafka, aim_redis, and aim_pg extension tools instead of asking the user to run docker commands.
---
# aim-docker-datastore

本 Skill 让 LLM 在 AIM 项目中主动使用项目级 extension 工具连接 Docker 中的数据存储：Kafka、Redis、PostgreSQL。

## 触发场景

当用户提到以下任意内容时，优先使用本 Skill，并主动调用 extension 工具：

- “连接 docker 中的 kafka/redis/pg/postgres/postgresql”
- “查看 Kafka topic / consumer group / 消费消息”
- “查看 Redis key / value / TTL / dbsize / info”
- “查看 PostgreSQL 数据库 / 表 / 查询数据”
- “检查本地 Docker 数据存储是否正常”
- “排查注册、好友、会话、消息投递、在线状态、读回执、typing 等链路的数据落库或 Kafka 事件”

不要只给用户 shell 命令；如果相关 extension 工具可用，应直接调用工具。

## 可用工具

项目级 extension 位于：`.pi/extensions/aim-docker/index.ts`。

可用工具：

- `aim_docker_status`
  - 检查 Kafka、Redis、PostgreSQL 的容器运行与连接状态。
- `aim_kafka`
  - 连接 `aim-kafka`。
  - 支持：`list_topics`、`describe_topic`、`consume`、`list_groups`、`describe_group`。
- `aim_redis`
  - 连接 `aim-redis`。
  - 支持：`ping`、`info`、`dbsize`、`scan`、`type`、`ttl`、`get`、`set`、`del`。
- `aim_pg`
  - 连接 `aim-postgres`。
  - 支持：`list_databases`、`list_tables`、`describe_table`、`query`。
  - `query` 默认只读；除非用户明确要求修改本地开发数据，否则不要设置 `readonly=false`。

## 默认工作流

### 1. 先确认数据存储是否可用

当用户没有明确说明容器已经运行时，先调用：

```text
aim_docker_status({})
```

如果服务未运行，简要提示：

```bash
docker compose up -d postgres redis kafka
```

如需 Kafka 初始化 topic：

```bash
docker compose up kafka-init
```

如果工具不可用或刚新增扩展后未加载，提示用户执行 `/reload` 或重启 pi。

### 2. Kafka 排查

常用顺序：

1. 列出 topic：

```text
aim_kafka({ action: "list_topics" })
```

2. 描述指定 topic：

```text
aim_kafka({ action: "describe_topic", topic: "aim-message-transfer" })
```

3. 消费少量消息，默认不要读取过多：

```text
aim_kafka({ action: "consume", topic: "aim-message-transfer", fromBeginning: true, maxMessages: 10 })
```

4. 查看 consumer group：

```text
aim_kafka({ action: "list_groups" })
aim_kafka({ action: "describe_group", group: "<group>" })
```

AIM 常见 topic：

- `aim.user.events`
- `aim-message-transfer`
- `aim.conversation.events`
- `aim.presence.events`
- `aim.typing.events`
- `aim.read_receipt.events`

注意：Kafka 消息量可能很大，`consume` 默认使用小 `maxMessages`，除非用户明确要求扩大。

### 3. Redis 排查

常用顺序：

1. 连通性：

```text
aim_redis({ action: "ping" })
```

2. 关键指标：

```text
aim_redis({ action: "info" })
aim_redis({ action: "dbsize" })
```

3. 查找 key：

```text
aim_redis({ action: "scan", pattern: "*presence*" })
```

不要建议或执行 `KEYS *`；使用 `scan`。

4. 查看 key：

```text
aim_redis({ action: "type", key: "<key>" })
aim_redis({ action: "ttl", key: "<key>" })
aim_redis({ action: "get", key: "<key>" })
```

只有用户明确要求修改本地开发 Redis 数据时，才使用 `set` 或 `del`。

### 4. PostgreSQL 排查

AIM 常用数据库：

- `aim_auth`
- `aim_logic`

常用顺序：

1. 列出数据库：

```text
aim_pg({ action: "list_databases" })
```

2. 列出表：

```text
aim_pg({ action: "list_tables", database: "aim_auth" })
aim_pg({ action: "list_tables", database: "aim_logic" })
```

3. 描述表：

```text
aim_pg({ action: "describe_table", database: "aim_logic", table: "conversations" })
```

4. 只读查询：

```text
aim_pg({ action: "query", database: "aim_logic", query: "select * from conversations limit 10" })
```

安全规则：

- 默认只读查询，不要主动写库。
- 只有用户明确要求修改本地开发数据时，才使用：

```text
aim_pg({ action: "query", database: "aim_logic", query: "...", readonly: false })
```

- 写库前先说明影响范围，并确认是本地 Docker 开发环境。

## 链路排查建议

### 注册后好友请求 user not found

优先检查：

1. Kafka `aim.user.events` 是否有用户创建事件。
2. logic 是否消费成功。
3. `aim_logic` 中用户相关表是否已写入。

推荐工具顺序：

```text
aim_kafka({ action: "consume", topic: "aim.user.events", fromBeginning: true, maxMessages: 10 })
aim_pg({ action: "list_tables", database: "aim_logic" })
aim_pg({ action: "query", database: "aim_logic", query: "select * from users order by created_at desc limit 10" })
```

### 消息未投递

优先检查：

1. `aim-message-transfer` topic 是否有消息。
2. core consumer group 是否有 lag。
3. logic/core 相关表或状态是否符合预期。

推荐工具顺序：

```text
aim_kafka({ action: "describe_topic", topic: "aim-message-transfer" })
aim_kafka({ action: "consume", topic: "aim-message-transfer", fromBeginning: true, maxMessages: 10 })
aim_kafka({ action: "list_groups" })
```

### 在线状态、typing、读回执异常

相关 topic：

- `aim.presence.events`
- `aim.typing.events`
- `aim.read_receipt.events`

推荐工具顺序：

```text
aim_kafka({ action: "consume", topic: "aim.presence.events", fromBeginning: true, maxMessages: 10 })
aim_kafka({ action: "consume", topic: "aim.typing.events", fromBeginning: true, maxMessages: 10 })
aim_kafka({ action: "consume", topic: "aim.read_receipt.events", fromBeginning: true, maxMessages: 10 })
aim_redis({ action: "scan", pattern: "*presence*" })
```

### 群管理系统消息异常

相关 topic 与 consumer group：

- topic：`aim.conversation.events`
- producer：logic `ConversationEventProducerConf`
- consumer group：`aim-core-conversation-events`

推荐工具顺序：

```text
aim_kafka({ action: "describe_topic", topic: "aim.conversation.events" })
aim_kafka({ action: "consume", topic: "aim.conversation.events", fromBeginning: true, maxMessages: 10 })
aim_kafka({ action: "describe_group", group: "aim-core-conversation-events" })
```

## 输出原则

- 工具调用后，用中文总结观察结果。
- 如果服务未运行，先给最短启动命令。
- 如果工具输出被截断，说明完整输出路径。
- 不要泄露或打印不必要的敏感信息；本地开发默认账号密码仅用于 Docker 内部连接。
- 发现 schema/table 名不确定时，先 `list_tables`，不要猜表名。
