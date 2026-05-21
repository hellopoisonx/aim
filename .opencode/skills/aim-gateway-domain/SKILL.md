---
name: aim-gateway-domain
description: aim 的网关域。对应 `gateway` 模块。
---
# aim-gateway-domain

## 如何使用

- 涉及功能及需求定义 -> `references/detail.md`
- 涉及接口 -> `references/api.md`
- 涉及 WebSocket 内部实现 -> `references/ws-internals.md`

## 参考资料

- `references/detail.md`
- `references/api.md`
- `references/ws-internals.md`

## 最近变更

- 2026-05-22: 打通 presence/typing 推送链路：新增 `PushTyping` gRPC、`GET /api/presence/friends` 快照接口；Manager 维护 Redis Set 聚合多设备状态；用 kq.Pusher 真发 `aim.presence.events` 和 `aim.typing.events`；PresenceTTL 默认 45s，客户端心跳 20s。
- 2026-05-21: 新增 `GET /api/conversations` 端点，返回当前用户参与的所有会话列表（包含会话基本信息和成员列表）；该端点受 `Auth` 中间件保护，调用 `LogicConversationClient.GetUserConversations` 获取数据。
- 2026-05-19: gateway RPC 容器监听改为 `0.0.0.0:9090`；Nacos resolver 在服务列表为空时不上报空地址列表，避免启动期空服务列表失败。
- 2026-05-19: 补齐 gateway 生产 WS 路由注册与 WS ACK 409 映射；接入 RPC 统一 unary 错误拦截器。
