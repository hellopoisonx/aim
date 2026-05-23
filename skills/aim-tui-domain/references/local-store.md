# TUI 本地 SQLite 存储

## 位置

默认路径：

```text
<UserCacheDir>/aim/tui/<instance_id>.db
```

也可通过 `--db` 或 `AIM_TUI_DB` 指定。

## 表

- `tokens`：按 `instance_id` 保存 email、user_id、access_token、refresh_token、expires_at、device_id。
- `conversations`：缓存 gateway `ConversationItem`，成员 ID 以 JSON 存储。
- `messages`：缓存本地历史消息、WS 推送消息和发送乐观消息。
- `presence`：缓存好友在线状态。

## 并发与可靠性

- SQLite 使用 WAL、foreign_keys、busy_timeout。
- 单进程连接池限制为 1 个打开连接，降低 TUI 内并发写锁复杂度。
- 业务操作全部使用 `context.Context` 和参数化 SQL。

## 数据语义

- 本地缓存是客户端加速和离线展示用途，不是服务端权威数据。
- token 属于敏感数据，当前实现明文存储在用户 cache 目录；后续如需生产桌面端安全能力，应接入系统 keychain/credential vault。
