# RocketMQ 运维指引

## Topic 预创建（必须）

所有 topic 必须在业务服务启动前由 `rocketmq-init` 容器创建，broker 配置为 `autoCreateTopicEnable = false`。

**原因：** topic 的 message type（FIFO / TRANSACTION / NORMAL）决定 broker 接受哪种发送方式：

- FIFO：producer 必须设置 MessageGroup（AIM 用 `conversation_id`），否则发送被拒
- TRANSACTION：只接受 `SendWithTransaction`（half message + commit/rollback），普通 `Send` 报 `current message type not match with topic accept message types`
- NORMAL：不收事务消息

依赖 auto-create 会得到 NORMAL topic，事务消息与有序投递都会失败，因此必须显式声明类型。

### 命名约束

broker 只接受 `^[%|a-zA-Z0-9_-]+$` 的 topic 名，**不能包含点号**。因此 AIM 的 topic 统一用连字符：

| Topic | message.type | 说明 |
| --- | --- | --- |
| `aim-message-transfer` | FIFO | 聊天投递，MessageGroup = `conversation_id` |
| `aim-read-receipt-events` | FIFO | 已读回执，MessageGroup = `conversation_id` |
| `aim-user-events` | TRANSACTION | `user.created` 事件 |
| `aim-conversation-events` | TRANSACTION | 群管理通知（放弃严格保序） |
| `aim-attachment-uploaded` | TRANSACTION | 附件上传完成 |
| `aim-presence-events` / `aim-typing-events` / `aim-friend-application-events` / `aim-attachment-parsed` | NORMAL | 瞬时/独立通知 |

### 初始化脚本

`deploy/scripts/init-rocketmq-topics.sh` 在 broker healthy 后执行，幂等（`updateTopic` / `updateSubGroup` 均可重复执行）：

```sh
sh mqadmin updateTopic -n rocketmq-namesrv:9876 -c DefaultCluster -t aim-user-events -r 4 -w 4 -a +message.type=TRANSACTION
sh mqadmin updateTopic -n rocketmq-namesrv:9876 -c DefaultCluster -t aim-message-transfer -r 4 -w 4 -a +message.type=FIFO
sh mqadmin updateSubGroup -n rocketmq-namesrv:9876 -c DefaultCluster -g aim-core-delivery -o true
```

业务服务必须声明依赖：

```yaml
depends_on:
  rocketmq-proxy:
    condition: service_started
  rocketmq-init:
    condition: service_completed_successfully
```

## 诊断命令

```bash
# 全部 topic
docker exec aim-rocketmq-broker sh mqadmin topicList -n rocketmq-namesrv:9876

# topic 路由与队列
docker exec aim-rocketmq-broker sh mqadmin topicRoute -n rocketmq-namesrv:9876 -t aim-message-transfer

# topic 堆积/offset
docker exec aim-rocketmq-broker sh mqadmin topicStatus -n rocketmq-namesrv:9876 -t aim-user-events

# 全部 consumer group 进度（Diff 即堆积）
docker exec aim-rocketmq-broker sh mqadmin consumerProgress -n rocketmq-namesrv:9876

# 单个 group
docker exec aim-rocketmq-broker sh mqadmin consumerProgress -n rocketmq-namesrv:9876 -g aim-logic-user-created

# 按 key 查消息（事务消息 key[0] 是 event_uid）
docker exec aim-rocketmq-broker sh mqadmin queryMsgByKey -n rocketmq-namesrv:9876 -t aim-user-events -k user.created:7

# 预览消息
docker exec aim-rocketmq-broker sh mqadmin consumeMessage -n rocketmq-namesrv:9876 -t aim-user-events -c 5
```

### 快速健康检查清单

| 检查项 | 命令 | 预期结果 |
| --- | --- | --- |
| topic 存在 | `topicList` | 含 9 个 `aim-` topic |
| message type 正确 | 初始化日志中的 `attributes={+message.type=...}` | FIFO/TRANSACTION 与上表一致 |
| 消费进度 | `consumerProgress` | 业务 group 的 `Diff` 不持续增长 |
| 事务回查可用 | broker 日志无 `checkTransaction` 报错 | 回查命中 `event_journal` |

## 事务消息排查

事务消息的三段式：half message → 本地事务（业务行 + `event_journal`）→ commit/rollback。

- 消息可见但业务行缺失：检查 `event_journal` 是否写入；未写入时 broker 回查会 rollback 丢弃消息（属预期）
- 业务行存在但下游未收到：等待 broker 回查（`transactionCheckInterval`，dev 为 6s），或检查 producer 是否在运行（回查由 producer 应答，producer 不在则一直 UNKNOWN）
- 排查 SQL：`select event_uid, topic, msg_key, created_at from event_journal order by created_at desc limit 20;`

## 完整重启流程

```bash
docker compose down -v          # 清空 broker store 与数据库
docker compose up -d --build    # rocketmq-init 在 broker healthy 后重建 topic 与 group
docker exec aim-rocketmq-broker sh mqadmin topicList -n rocketmq-namesrv:9876
```

## 历史排查记录

**2026-05-19（RocketMQ 时期）：注册后 logic 不创建 user_info** —— topic 在 consumer 加入 group 后才被 auto-create，分区分配为空且不会重平衡，导致消费者永远收不到消息。迁移到 RocketMQ 后同类问题表现为：未预创建的 topic 会以 NORMAL 类型自动创建（broker 已关闭 auto-create，因此直接发送失败），修复方式仍是先跑 `rocketmq-init`。
