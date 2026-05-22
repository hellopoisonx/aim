# Kafka 运维指引

## Topic 预创建（必须）

所有项目的 Kafka topic 必须在业务服务启动前通过 `kafka-init` 容器预创建，不能依赖 auto-create。

**原因：Kafka consumer group 重平衡与 topic 自动创建的竞态条件**

当业务服务（如 `aim-logic`）的 Kafka consumer 先于 topic 创建加入 consumer group 时，会发生以下序列：

1. Consumer 发起 `JoinGroup` 请求，coordinator 创建 consumer group
2. Consumer 被选为 leader，执行 `partition.assignment.strategy`
3. **此时 topic 尚不存在**（auto-create 由 producer 或 admin client 触发）
4. Group coordinator 返回空分区分配 → `#PARTITIONS=0`
5. 之后 topic 被 auto-create（例如 auth producer 首次 publish），但 consumer group 已处于 Stable 状态，不会自动重新分配
6. 结果：consumer group 处于 Stable，有 1 个 member，但 `#PARTITIONS=0`，**consumer 永远消费不到任何消息**

### 症状

- 业务逻辑看似正常启动，但消费者从未收到消息
- `kafka-consumer-groups.sh --describe` 显示 consumer group Stable 但 `#PARTITIONS=0`
- 重启消费者容器后恢复正常（因为 rebalance 触发重新分配）

### 解决方案

在 `docker-compose.yaml` 中添加 `kafka-init` 服务，在 kafka broker 健康后、业务服务启动前执行：

```yaml
kafka-init:
  image: apache/kafka:4.0.2
  container_name: aim-kafka-init
  depends_on:
    kafka:
      condition: service_healthy
  command:
    - sh
    - -c
    - |
      /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:9092 --create --if-not-exists --topic aim.user.events --partitions 3 --replication-factor 1
      /opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:9092 --create --if-not-exists --topic aim-message-transfer --partitions 3 --replication-factor 1
  networks:
    - aim-network
```

所有业务服务（`aim-auth`、`aim-logic`、`aim-core`、`aim-gateway`）必须声明依赖：

```yaml
depends_on:
  kafka-init:
    condition: service_completed_successfully
```

`--if-not-exists` 保证幂等：重复执行或重启 `kafka-init` 不会报错。

### Broker 端默认分区数

即使配置了 `KAFKA_NUM_PARTITIONS=3`（broker 级别默认值），该值仅在 topic **被 auto-create 时**生效。预创建 topic 时必须显式指定 `--partitions`，不依赖 broker 默认值。

## 诊断命令

使用 `apache/kafka:4.0.2` 镜像自带的脚本进行诊断：

```bash
# 查看所有 consumer group 状态
docker exec aim-kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server kafka:9092 --all-groups --describe

# 查看特定 consumer group 成员和分区分配
docker exec aim-kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server kafka:9092 \
  --describe --group aim-logic-user-created --members --verbose

# 查看 topic 是否存在及分区数
docker exec aim-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka:9092 --describe --topic aim.user.events

# 手动消费 topic 验证消息是否到达
docker exec aim-kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka:9092 \
  --topic aim.user.events --from-beginning
```

### 快速健康检查清单

| 检查项 | 命令 | 预期结果 |
| --- | --- | --- |
| Topic 存在 | `kafka-topics.sh --list` | 包含 `aim.user.events`、`aim-message-transfer` |
| Topic 分区数 | `kafka-topics.sh --describe` | `PartitionCount: 3` |
| Consumer group 状态 | `kafka-consumer-groups.sh --describe --group <group>` | `State: Stable` 且 `#PARTITIONS > 0` |
| Consumer 有分配到分区 | `--members --verbose` | 每个 member 有 `CURRENT-ASSIGNMENT` |

## 完整重启流程

当需要全新初始化环境时：

```bash
# 清理所有数据卷和容器
docker compose down -v

# 重新构建并启动（kafka-init 会在 kafka 健康后自动创建 topic）
docker compose up -d --build

# 验证 topic 已创建
docker exec aim-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server kafka:9092 --list
```

## 历史排查记录

**2026-05-19：注册后 logic 不创建 user_info**

- 现象：auth 注册成功，Kafka 消息正常发布（console-consumer 可消费），但 logic 从未消费
- 诊断：`kafka-consumer-groups.sh --describe --group aim-logic-user-created --members --verbose` 显示 `State: Stable, MemberId: ..., #PARTITIONS=0` — consumer group 有 member 但分配到 0 个分区
- 根因：`aim.user.events` topic 在 consumer 加入 group 后才被 auto-create，导致初始分区分配为空且不会自动重新分配
- 修复：添加 `kafka-init` 服务预创建 topic，重启 `aim-logic` 触发 rebalance 后正常
