# TUI 本地 SQLite 存储

## 位置

默认路径：

```text
<UserCacheDir>/aim/tui/<instance_id>.db
```

也可通过 `--db` 或 `AIM_TUI_DB` 指定。

## 表

- `tokens`：按 `instance_id` 保存 email、user_id、access_token、refresh_token、expires_at、device_id。
- `conversations`：缓存 gateway `ConversationItem`，成员 ID 以 JSON 存储；每次 REST 会话列表刷新会以服务端列表为准重建本地会话缓存，避免退群/解散后保留已失效会话。
- `messages`：缓存本地历史消息、WS 推送消息和发送乐观消息；持久化 `sender_info`、`mentions`、`read_details`，用于重启后保持昵称、@ 提及和逐消息已读展示；发送成功收到 `SERVER_ACK` 后，会用服务端 `message_id` 替换同 `client_msg_id` 的 pending 乐观消息，避免本地 SQLite 保留临时 ID；发送被拒绝时会删除同 `client_msg_id` 的本地记录，避免重启后重复展示失败草稿。
- `presence`：缓存好友在线状态。

## 并发与可靠性

- SQLite 使用 WAL、foreign_keys、busy_timeout。
- 单进程连接池限制为 1 个打开连接，降低 TUI 内并发写锁复杂度。
- 同一 `--db` 文件在**跨进程**时通过 `*.lock` 文件互斥：第二个 TUI 进程会启动失败并提示换 `--instance`/`--db`；同一进程内可打开多个 `Store`（不同 `instance_id` 分区）。
- 业务操作全部使用 `context.Context` 和参数化 SQL。
- WebSocket 客户端写入使用互斥串行化，避免心跳、已读回执、消息发送、客户端 ACK 并发调用 `Conn.Write` 引发长期运行时卡死。

## 数据语义

- 本地缓存是客户端加速和离线展示用途，不是服务端权威数据。
- token 属于敏感数据，当前实现明文存储在用户 cache 目录；后续如需生产桌面端安全能力，应接入系统 keychain/credential vault。
