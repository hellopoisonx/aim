# 探索报告：消息顺序保证机制

## 一、数据库层

### 表结构
**文件**: `app/logic/rpc/model/migrations/002_messages.sql`
```sql
CREATE TABLE IF NOT EXISTS messages (
    id BIGINT PRIMARY KEY,              -- Snowflake ID，单调递增
    conversation_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    message_type VARCHAR(32) NOT NULL,
    content JSONB NOT NULL DEFAULT '{}',
    client_msg_id VARCHAR(256),
    mentions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_messages_conv_time ON messages(conversation_id, created_at DESC);
```

### 查询排序
**文件**: `app/logic/rpc/model/queries/message.sql`
```sql
ORDER BY created_at DESC, id DESC       -- 双向排序：时间 + Snowflake ID
```

### 数据库层结论
- **没有独立的 `sequence` 或 `order` 字段**，排序依赖 `created_at + id`。
- 复合索引 `(conversation_id, created_at DESC)` 覆盖了按会话查询的排序场景。
- Snowflake ID (`id`) 本身是时间前缀的单调递增 ID，相同 `created_at` 内按 `id` 二次排序可消除时间精度不足导致的歧义。
- **风险**: 仅在读取时排序，写入无顺序约束；消息先入 Kafka 再异步写入 DB，DB 中 `created_at` 由 `NOW()` 决定（数据库写入时间），不等于消息的实际发送时间。

---

## 二、消息队列（Kafka）层

### 2.1 生产者 — TransferLogic

**文件**: `app/core/rpc/internal/logic/transfer_logic.go`（第 65-100 行）
```go
// 关键行
kafkaKey := strconv.FormatInt(in.ConversationId, 10)  // 以 conversation_id 为 key
l.publisher.Publish(l.ctx, kafkaKey, event)
```

意图：相同 `conversation_id` 的消息使用相同的 Kafka key，期望路由到同一分区，从而保证分区内顺序。

### 2.2 生产者默认 Balancer — ⚠️ 关键风险

**文件**: `C:\Users\hpxx\go\pkg\mod\github.com\zeromicro\go-queue@v1.2.2\kq\pusher.go`（第 57-62 行）
```go
producer := &kafka.Writer{
    Addr:        kafka.TCP(addrs...),
    Topic:       topic,
    Balancer:    &kafka.LeastBytes{},  // ← 默认！不按 key 哈希分区
    Compression: kafka.Snappy,
}
```

**文件**: `C:\Users\hpxx\go\pkg\mod\github.com\segmentio\kafka-go@v0.4.51\balancer.go`（第 87-110 行）
`LeastBytes.Balance()` **只按分区当前待发送字节数做负载均衡**，完全不考虑 key 的内容。`msg.Key` 仅被计入字节计数，但**不用于决定分区**。

**结论**: 消息 key = `conversation_id` 这个设计意图（同会话消息进入同一分区以保序）**因默认 `LeastBytes` Balancer 而无效**。同会话的消息可能被分发到不同分区，失去 Kafka 的分区内顺序保障。

**可用的 Key-aware Balancer（存在于 kafka-go 中但未使用）**:
- `Hash` — 使用 key 的 Go 指针哈希
- `CRC32Balancer` — CRC32 哈希
- `Murmur2Balancer` — Murmur2 哈希（与 Java Kafka 生产者兼容）

### 2.3 ChunkExecutor 批处理

**文件**: `C:\Users\hpxx\go\pkg\mod\github.com\zeromicro\go-queue@v1.2.2\kq\pusher.go`（第 77-86 行）

默认使用异步 `ChunkExecutor` 攒批发送。同一 Chunk 内的消息调用 `WriteMessages(ctx, chunk...)` 一次性发送，balancer 在本次调用内为每条消息独立分配分区。如果同会话的两条消息在同一个 chunk 中，它们仍可能被分到不同分区。

---

## 三、消费者层

### 3.1 核心投递消费者 (core)

**文件**: `app/core/rpc/etc/core.yaml`
```yaml
KqConsumerConf:
  Group: aim-core-delivery
  Topic: aim-message-transfer
  Offset: first
  Consumers: 1       # 1 个读取 goroutine
  Processors: 1      # 1 个处理 goroutine
  CommitInOrder: false
```

**文件**: `C:\Users\hpxx\go\pkg\mod\github.com\zeromicro\go-queue@v1.2.2\kq\queue.go`

- `Consumers=1`：单个 goroutine 从 Kafka fetch 消息
- `Processors=1`：单个 goroutine 处理消息
- `CommitInOrder=false`：不保证按 Kafka offset 顺序提交

**效果**：处理是串行的（单 goroutine），但 kafka-go Reader（`Conns` 默认 1）从 Kafka 分配到的**所有分区**拉取消息。不同分区的消息在 fetch 阶段被**交织混排**，处理顺序是 Kafka 返回的原始顺序（分区内有序，分区间无序）。

**文件**: `app/core/rpc/internal/mqs/delivery_consumer.go`
消费者拿到消息后通过 gRPC 调用 gateway 的 `PushMessage` 推送给前端用户。

### 3.2 归档消费者 (logic)

**文件**: `app/logic/rpc/etc/logic.yaml`
```yaml
KqConsumerConf:
  Group: aim-logic-archive
  Topic: aim-message-transfer
  Offset: first
  Consumers: 1
  Processors: 1
```

**文件**: `app/logic/rpc/internal/mqs/archive_consumer.go`
消费者将消息 JSON 反序列化后插入 PostgreSQL `messages` 表。与 core delivery consumer **消费同一个 Topic** (`aim-message-transfer`)，但**不同的消费者组** (`aim-core-delivery` vs `aim-logic-archive`)，因此各自独立消费，互不影响。

---

## 四、消息流转全链路

```
客户端 → Gateway (REST/WS)
       → Core (gRPC Transfer)
         → [1] 幂等性检查 (Redis)
         → [2] 权限检查 (Logic RPC)
         → [3] Snowflake 生成 msgID
         → [4] Kafka Publish (topic=aim-message-transfer, key=conversation_id)
         → [5] 设置幂等 key (Redis, 24h TTL)
         → 返回 msgID 给客户端

Kafka topic "aim-message-transfer"
  ├── Consumer Group "aim-core-delivery" (Conns=1, Consumers=1, Processors=1)
  │   → DeliveryConsumer → gRPC PushMessage → Gateway → WebSocket 推送给在线用户
  └── Consumer Group "aim-logic-archive" (Conns=1, Consumers=1, Processors=1)
      → ArchiveConsumer → INSERT INTO messages
```

---

## 五、并发消费乱序风险分析

| 风险点 | 描述 | 严重程度 |
|--------|------|----------|
| **Balancer 不按 Key 路由** | `LeastBytes` 忽略 key，同会话消息可能去不同分区 | 🔴 **高** |
| **跨分区交织消费** | 单 Reader 拉取所有分区，分区间消息交织，无顺序保障 | 🟡 **中** |
| **Consumer 单 goroutine** | `Processors=1` 避免了消费端并行乱序，但不能修复跨分区交织 | ✅ 缓解 |
| **水平扩展** | 如果未来增加 `Conns` 或启动多实例，不同实例消费不同分区，同会话消息可能在不同实例上乱序处理 | 🟡 **中（待扩展时）** |
| **ChunkExecutor 攒批** | 异步批发送，不同会话消息可能同一批发出 | 🟢 **低**（对顺序无直接影响） |
| **DB 异步写入** | 消息先入 Kafka 再异步写入 DB，DB 的 `created_at` 是 `NOW()`（写入时间），与发送时间可能不同 | 🟡 **中**（仅影响 DB 查询排序） |

---

## 六、关键文件清单

| 文件 | 作用 |
|------|------|
| `app/core/rpc/internal/logic/transfer_logic.go` | 生产者：构建 Kafka 事件，key=conversation_id |
| `app/core/rpc/internal/mqs/delivery_consumer.go` | 消费者：core 侧投递消息给 gateway |
| `app/logic/rpc/internal/mqs/archive_consumer.go` | 消费者：logic 侧写入 DB 归档 |
| `app/core/rpc/etc/core.yaml` | core 的 Kafka 配置（Consumers=1, Processors=1） |
| `app/logic/rpc/etc/logic.yaml` | logic 的 Kafka 配置（Consumers=1, Processors=1） |
| `app/logic/rpc/model/migrations/002_messages.sql` | messages 表结构 |
| `app/logic/rpc/model/queries/message.sql` | 消息查询（含排序逻辑） |
| `C:\Users\hpxx\go\pkg\mod\github.com\zeromicro\go-queue@v1.2.2\kq\pusher.go` | kq.Pusher 源码（默认 LeastBytes balancer） |
| `C:\Users\hpxx\go\pkg\mod\github.com\zeromicro\go-queue@v1.2.2\kq\queue.go` | kq.Queue 源码（消费者 goroutine 模型） |
| `C:\Users\hpxx\go\pkg\mod\github.com\segmentio\kafka-go@v0.4.51\balancer.go` | kafka-go balancer 实现 |

---

## 七、结论

1. **数据库层**：无独立 `sequence` 字段，排序依赖 `(created_at DESC, id DESC)`。Snowflake ID 提供单调递增性，但 `created_at` 是数据库写入时间而非消息发送时间。

2. **MQ 层**：生产者以 `conversation_id` 为 key，意图实现同会话分区内有序。但**默认 `kq.Pusher` 使用 `kafka.LeastBytes` balancer，不按 key 哈希路由**，导致同会话消息可能分布在多个分区，**丧失分区级顺序保证**。

3. **消费并发**：`Consumers=1, Processors=1` 使消费端串行处理，避免了处理器级别的乱序。但 kafka-go Reader 拉取所有分区的消息会**交织混排**，不同分区的消息处理顺序不可控。

4. **根本解决方案**：将 `kq.Pusher` 的 balancer 改为 key-aware 实现（如 `kafka.Murmur2Balancer` 或 `kafka.Hash`），确保相同 `conversation_id` 的消息路由到同一分区。当前代码已设置正确的 key，但 balancer 选择抵消了 key 的效果。