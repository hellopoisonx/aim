# Message Ingest & Push Pipeline

## 概览

消息热路径已整体迁到 logic：`MessageService.IngestMessage` 负责校验、授权、幂等、ID 分配与发布，core 只负责把 `aim-message-transfer` 事件扩散成 `aim-gateway-push` 事件，gateway 消费后写本地连接。

## Ingest Pipeline（logic）

1. 校验 `sender_id`、`conversation_id`、`message_type`、`client_msg_id`、`mentions`。
2. 用 `idempotency:transfer:{sender_id}:{device_id}:{client_msg_id}` 查 Redis 幂等键（TTL 24h）；命中直接返回已有 `message_id`。
3. 进程内 `PermissionChecker.CheckMessagePermission`（`DatabasePermissionChecker` + Casbin `message.send` 授权）；checker 未配置时返回 `CodeInternal`（fail-closed）。
4. 用 `mentions` 过滤结果回写事件（非成员 @ 只保留正文）。
5. 附件消息经 `AttachmentService.ValidateReference` 校验。
6. shared Snowflake 生成 `message_id`。
7. 构造事件 JSON（含 `traceparent`/`tracestate`、`device_id`），发布到 `aim-message-transfer`，`Key` 与 MessageGroup 都是 `conversation_id`。
8. 发布成功后 best-effort 写幂等键，失败只记日志。

## Push Pipeline（core → gateway）

1. `DeliveryConsumer` 消费 `aim-message-transfer`，查成员快照与 `conversation_type`。
2. 单聊：逐成员发布 `PushEvent`（`target_user_id`）。
3. 群聊：单条 `PushEvent`（`member_ids`）。
4. gateway `PushConsumer` 按寻址字段分流：`DeliverLocal`（单目标）或 `DeliverLocalMembers`（单次编码、本地成员扇出）。
5. gateway 跳过发送者源设备（`source_device_id`），其余设备照常收到。

## 本地规则

- `app/logic/rpc/internal/service/message_service.go` 的 `IdempotencyStore` / `MessagePublisher` / `AttachmentReferenceValidator` 是测试缝，不要在测试里直接打 Redis/RocketMQ。
- 新字段进入 Transfer/Push event 时，同步更新 `app/shared/events`、gateway payload 与客户端文档。
- 权限失败、参数错误返回 `errorx.CodeError`；Redis/RocketMQ/Snowflake 基础设施失败返回 `CodeInternal`。
- core 可以调用 logic 的 pb/client；logic 绝不能导入 core。
- 幂等键 TTL 当前为 24h；修改 TTL 需同步重复发送测试。

## 测试

```bash
go test ./app/logic/rpc/internal/service/...
go test ./app/core/rpc/internal/mqs/...
go test ./app/gateway/api/internal/mqs/...
```

新增分支必须覆盖：重复 client_msg_id、权限拒绝、checker 未配置、mentions 过滤、publish 失败、幂等 set 失败但发布成功、单聊/群聊扩散形态。
