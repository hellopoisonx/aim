# Transfer Pipeline

## 概览

本目录承载 core 的 Transfer 热路径。目标只有一个：验证消息、保证幂等、查 logic 权限、生成 message_id、按 conversation_id 投递 Kafka，再返回可被 gateway 映射为 WS ACK 的结果。

## Transfer Pipeline

1. 校验 `sender_id`、`conversation_id`、`message_type`、`client_msg_id`、`mentions`。
2. 用 `idempotency:transfer:{sender_id}:{device_id}:{client_msg_id}` 查 Redis 幂等键。
3. 调 logic `PermissionService.CheckMessagePermission`；gRPC 业务错误用 `errorx.FromGRPCError` 保留。
4. 用 shared Snowflake 生成 `message_id`。
5. 构造 Kafka JSON payload，必须包含 `traceparent`/`tracestate`。
6. `PushWithKey(ctx, conversation_id, payload)`，保证同会话有序。
7. Kafka 成功后 best-effort 写幂等键，失败只记日志，不回滚已发布消息。

## 本地规则

- `interfaces.go` 中的 `idempotencyStore`、`messagePublisher` 是测试缝，不要绕过后直接在测试中打 Redis/Kafka。
- 新字段进入 Transfer event 时，同步更新 logic archive consumer、gateway/客户端需要的 ACK 或 push payload。
- 权限失败、参数错误、限流等业务拒绝返回 `errorx.CodeError`；Kafka/Redis/Snowflake 基础设施失败返回 `CodeInternal`。
- core 可以调用 logic 的 pb/client；logic 绝不能导入 core。
- 幂等键 TTL 当前为 24h；修改 TTL 需要更新重复发送测试。

## 测试

```bash
go test ./app/core/rpc/internal/logic/...
go test -run TestTransfer ./app/core/rpc/internal/logic/...
```

新增分支必须覆盖：重复 client_msg_id、logic 权限拒绝、Kafka publish 失败、Redis 幂等检查失败、幂等 set 失败但发布成功。