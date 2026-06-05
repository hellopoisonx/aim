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
- 2026-06-05: 新增 `app/shared/outbox`（事务发件箱模式）。`Store` 接口定义 outbox 表 CRUD；`Poller` 后台轮询 + 快速路径 `Wake()` 投递到 Kafka，保证至少一次投递。Nil-safe 构造（Store/PublisherFunc 为 nil 时 Poller 为 no-op）。配置 `Config.WithDefaults()` 与 cleanup 自动清理。
- 2026-06-03: 弃用 Nacos v2，改用 go-zero 内置 etcd 作为服务注册中心。删除 `app/shared/nacos` 与 `aimnacos://` 自定义 resolver scheme；服务注册/发现由 `zrpc.RpcServerConf.Etcd` / `zrpc.RpcClientConf.Etcd` 接管，`zrpc.MustNewServer` 自动 keepalive，进程退出由 `proc.AddWrapUpListener` 自动撤销。YAML 加 `Etcd: { Hosts, Key }` 块即可，业务代码零改动。参考 https://go-zero.dev/guides/microservice/service-discovery/。


- 2026-06-01: 新增 `app/shared/quota`（Redis ZSET 滑动窗口限流）。`New(client, opts)` 在 `MaxRequests<=0` 时返回 `(nil, nil)` 让调用方免 nil 检查；`AllowPair(a, b)` 拼接 `a:b`（空 b 渲染为 `unknown`）保持与 core 旧 `aim:transfer:quota:<sender>:<device>` key 兼容。
- 2026-05-28: （已废弃）原 Nacos gRPC resolver scheme 改为 `aimnacos` 并新增 `BuildTarget`，避免抢占 Nacos SDK 内部 `nacos:9848` 直连目标导致日志反复刷 `SelectInstances for "9848"`。2026-06-03 切到 etcd 后此问题不再存在，`app/shared/nacos` 与 `aimnacos` scheme 整包删除。

- 2026-05-28: 新增 `app/shared/cache` 两级缓存工具：L1 使用 go-zero `collection.Cache`，L2 使用 go-zero `stores/cache` + Redis，跨实例本地缓存失效通过 go-zero Redis `DoCtx(XADD/XREAD)` 的 Redis Stream 实现。


- 2026-05-25: `app/shared/tracing` 新增 Kafka producer span helper 与 `RecordSpanError`，用于附件上传/解析事件的 producer/consumer span 串联和统一错误标记。
- 2026-05-25: 新增 `app/shared/attachment`（`aim.attachment.v1` 内容 schema 与校验）、`app/shared/events` 附件事件（`AttachmentUploadedEvent` / `AttachmentParsedEvent`）和 `app/shared/s3signer`（SeaweedFS S3 预签名工具）。
- 2026-05-24: `app/shared/tracing` 新增 `DetachSpanContext(ctx)`，保留 context 值/取消/超时/baggage 但清除 active span context，供 WebSocket 等长连接在 upgrade 后创建独立 per-message span，避免长连接 upgrade span 成为短生命周期子 span 的父级，减少 tracing backend 的无效父 span/时钟偏移类告警。
- 2026-05-20: 从 app/shared/AGENTS.md 迁移
