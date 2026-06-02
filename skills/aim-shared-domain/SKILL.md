---
name: aim-shared-domain
description: aim 的进程内共享包域。对应 `app/shared` 模块。
---
# aim-shared-domain

## 如何使用

- 涉及包边界与依赖规则 -> `references/package-rules.md`

## 参考资料

- `references/package-rules.md`

## 最近变更
- 2026-06-02: 限流从 core/logic 收敛到 gateway 一层。core 删除 `TransferQuota` 字段、`TransferQuotaConf` 配置、`TransferLogic.checkQuota` 方法；logic 删除死代码 `QuotaStore` 字段与 `QuotaConf`。gateway 新增 `app/gateway/api/internal/middleware.RateLimitMiddleware`(人类)与 `BotRateLimitMiddleware`(Bot),共享 `app/shared/quota.QuotaStore`,通过 `KeyPrefix`(`aim:gateway:quota` / `aim:gateway:quota:bot`)隔离桶。WebSocket `handleSendMessage` 在调 `core.Transfer` 前手动限流,命中时通过 `ServerAckPayload` 返回 `ACK_STATUS_REJECTED + CodeRateLimit`。
- 2026-06-01: 新增 `app/shared/quota`（Redis ZSET 滑动窗口限流）。`New(client, opts)` 在 `MaxRequests<=0` 时返回 `(nil, nil)` 让调用方免 nil 检查；`AllowPair(a, b)` 拼接 `a:b`（空 b 渲染为 `unknown`）保持与 core 旧 `aim:transfer:quota:<sender>:<device>` key 兼容。
- 2026-05-28: 新增 `app/shared/cache` 两级缓存工具：L1 使用 go-zero `collection.Cache`，L2 使用 go-zero `stores/cache` + Redis，跨实例本地缓存失效通过 go-zero Redis `DoCtx(XADD/XREAD)` 的 Redis Stream 实现。
- 2026-05-28: Nacos gRPC resolver scheme 改为 `aimnacos` 并新增 `BuildTarget`，避免抢占 Nacos SDK 内部 `nacos:9848` 直连目标导致日志反复刷 `SelectInstances for "9848"`。
- 2026-05-27: `app/shared/attachment` 新增普通文件 kind `file` 与 `RequiresDataParsing` 判定；`file` 是合法附件消息类型但不进入媒体解析链路。
- 2026-05-25: `app/shared/tracing` 新增 Kafka producer span helper 与 `RecordSpanError`，用于附件上传/解析事件的 producer/consumer span 串联和统一错误标记。
- 2026-05-25: 新增 `app/shared/attachment`（`aim.attachment.v1` 内容 schema 与校验）、`app/shared/events` 附件事件（`AttachmentUploadedEvent` / `AttachmentParsedEvent`）和 `app/shared/s3signer`（SeaweedFS S3 预签名工具）。
- 2026-05-24: `app/shared/tracing` 新增 `DetachSpanContext(ctx)`，保留 context 值/取消/超时/baggage 但清除 active span context，供 WebSocket 等长连接在 upgrade 后创建独立 per-message span，避免长连接 upgrade span 成为短生命周期子 span 的父级，减少 tracing backend 的无效父 span/时钟偏移类告警。
- 2026-05-20: 从 app/shared/AGENTS.md 迁移
