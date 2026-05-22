---
name: aim-ws-token-management
description: WebSocket 连接 Token 生命周期管理。覆盖 Token 过期导致 WS 断连的排查与修复、JWT TTL 配置链路、WSClient 自动刷新机制、压测长跑场景下的 Token 续期策略。
---
# aim-ws-token-management

## 如何使用

- 排查 WS 压测大面积断连 → `references/token-pipeline.md`
- 了解 Token TTL 配置从 YAML 到 JWT 签发的完整链路 → `references/token-pipeline.md`
- 新增/修改 WSClient 重连逻辑 → `references/token-pipeline.md`

## 参考资料

- `references/token-pipeline.md`

## 核心结论

**WS 连接的生命周期受 AccessToken 5 分钟 TTL 约束**，但这条链路有两个坑：

1. **配置 TTL 被静默忽略**：`auth.yaml` 的 `Token.AccessTTL` 传到了 `JWTIssuer`，但 `JWTIssuer.Issue()` 直接调用 `sharedjwt.NewManager(secret).GenerateAccessToken()`，后者硬编码 `AccessTokenTTL = 5 * time.Minute`，完全无视配置。
2. **WSClient 重连不刷新 Token**：Token 过期后服务端主动踢连接（`sendTokenExpired`），客户端 `_on_close` / `send_one` 拿过期 Token 重连必然失败，全部变成 `ws_disconnected`。

## 最近变更

- 2026-05-22: 修复 `shared/jwt` 的 `Manager`：新增 `accessTTL` 字段和 `NewManagerWithTTL()` 构造函数，`GenerateAccessToken()` 使用 `m.accessTTL` 替代硬编码常量。修复 `auth/rpc` 的 `JWTIssuer`：存储并传递配置 TTL 到 `NewManagerWithTTL()`。详见 `references/token-pipeline.md`。
- 2026-05-22: `WSClient` 新增 `_rest_client` 引用和 `_try_refresh_token()` 方法：`reconnect()` 重连前自动检测 Token 过期并通过 REST API 刷新。benchmark 数据流扩展：`user_creds`/`conv_pairs` 元组增加 `refresh_token`/`expires_at`/`device_id`，确保 `WSClient` 拥有完整 token 信息用于刷新。`WsMessageScenario` 和 `MixedScenario` 同步注入 `RESTClient`。压测配置 `dev-tool/etc/auth.yaml` 的 `AccessTTL` 从 `5m` 增大到 `30m`。
