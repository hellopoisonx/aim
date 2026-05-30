# Dev Tool 命令参考

## CLI 模式

```bash
cd dev-tool
# Windows: 必须加 PYTHONIOENCODING=utf-8 避免 Unicode 编码错误
PYTHONIOENCODING=utf-8 python aim_test.py <command> [--args]
```

### 认证

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `register` | `--email` `--password` `--username` [`--avatar`] | 无 | `POST /api/auth/register` |
| `login` | `--email` `--password` [`--profile`] | 无 | `POST /api/auth/login` |
| `refresh` | [`--profile`] | 无 | `POST /api/auth/refresh` |
| `logout` | [`--profile`] | Bearer | `POST /api/auth/logout` |

### 用户

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `search` | `--name` | 无 | `GET /api/users/by-name/:name` |
| `get-user` | `--id` | Bearer | `GET /api/users/by-id/:id` |

### 好友

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `friend-add` | `--id` | Bearer | `POST /api/users/friends/:id` |
| `friend-applications` | — | Bearer | `GET /api/friends/applications` |
| `friend-accept` | `--id` | Bearer | `POST /api/friends/accept/:id` |
| `friend-reject` | `--id` | Bearer | `POST /api/friends/reject/:id` |
| `friend-list` | — | Bearer | `GET /api/friends/me` |
| `presence-friends` | — | Bearer | `GET /api/presence/friends` |

### 会话

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `conv-create` | `--member-id` 或 `--member-ids` [`--name`] | Bearer | `POST /api/conversations` |
| `group-create` | `--member-id` 或 `--member-ids` `--name` [`--avatar`] | Bearer | `POST /api/conversations/group` |
| `conv-members` | `--conversation-id` | Bearer | `GET /api/conversations/:id/members` |
| `conv-add-members` | `--conversation-id` `--member-ids` | Bearer | `POST /api/conversations/:id/members` |
| `conv-remove-member` | `--conversation-id` `--user-id` | Bearer | `DELETE /api/conversations/:id/members/:uid` |
| `conv-leave` | `--conversation-id` | Bearer | `POST /api/conversations/:id/leave` |
| `conv-dismiss` | `--conversation-id` | Bearer | `DELETE /api/conversations/:id` |
| `conv-update` | `--conversation-id` [`--name`] [`--avatar`] | Bearer | `PUT /api/conversations/:id` |
| `history` | `--conversation-id` [`--limit`] [`--cursor-created-at`] [`--cursor-id`] | Bearer | `GET /api/conversations/history/:id` |

注：`direct` 会话（单个成员）不需要客户端上传 `name` 参数；`group` 会话必须上传 `name`。`conv-create` 在 `--member-ids` 指定多个成员时等价创建群聊，需同时传 `--name`；推荐直接使用 `group-create --name <name>`。

### WebSocket

| 命令 | 参数 | 鉴权 | 帧类型 |
|------|------|------|--------|
| `ws-connect` | [`--profile`] | Bearer (header) | — |
| `ws-send` | `--conversation-id` `--content` [`--message-type`] [`--mentions`] [`--profile`] | Bearer | `SEND_MESSAGE` |
| `ws-heartbeat` | [`--profile`] [`--last-seq`] | Bearer | `HEARTBEAT` |
| `ws-typing` | `--conversation-id` [`--profile`] | Bearer | `TYPING` |
| `ws-read-receipt` | `--conversation-id` `--last-msg-id` [`--profile`] | Bearer | `READ_RECEIPT` |
| `ws-ack` | `--ack-seq` [`--profile`] | Bearer | `ACK` |


`ws-heartbeat` 默认使用 `WSClient` 跟踪到的“已连续处理的最大白名单 pending 推送 seq”作为 `HeartbeatPayload.last_seq`，用于触发服务端当前连接 L1 pending 补发；CLI 可用 `--last-seq N` 覆盖，交互模式可用 `ws-heartbeat N` 覆盖。

### 元命令

| 命令 | 说明 |
|------|------|
| `interactive` | 进入 REPL 交互模式 |
| `run-all` | 运行全流程集成测试（注册→登录→好友→会话→WS→注销） |

### 用户侧 Bot 管理

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `user-bot-create` | `--nickname` [`--email`] [`--avatar`] | Bearer | `POST /api/user/bots` |
| `user-bot-list` | — | Bearer | `GET /api/user/bots` |
| `user-bot-get` | `--bot-id` | Bearer | `GET /api/user/bots/:id` |
| `user-bot-update` | `--bot-id` `--nickname` [`--avatar`] | Bearer | `PUT /api/user/bots/:id` |
| `user-bot-enable` | `--bot-id` | Bearer | `POST /api/user/bots/:id/enable` |
| `user-bot-disable` | `--bot-id` | Bearer | `POST /api/user/bots/:id/disable` |
| `user-bot-delete` | `--bot-id` | Bearer | `DELETE /api/user/bots/:id` (软删除) |
| `user-bot-token-create` | `--bot-id` [`--name`] [`--expires-at`] [`--actions`] | Bearer | `POST /api/user/bots/:id/tokens` |
| `user-bot-token-list` | `--bot-id` | Bearer | `GET /api/user/bots/:id/tokens` |
| `user-bot-token-update` | `--bot-id` `--token-id` `--actions` [`--name`] [`--expires-at`] | Bearer | `PUT /api/user/bots/:id/tokens/:token_id` |
| `user-bot-token-rotate` | `--bot-id` `--token-id` | Bearer | `POST /api/user/bots/:id/tokens/:token_id/rotate` |
| `user-bot-token-revoke` | `--bot-id` `--token-id` | Bearer | `DELETE /api/user/bots/:id/tokens/:token_id` |
| `user-bot-add-conv` | `--bot-id` `--conversation-id` | Bearer | `POST /api/user/bots/:id/conversations/:conversation_id` |
| `user-bot-direct-conv` | `--bot-id` | Bearer | `POST /api/user/bots/:id/direct-conversation` |
| `bot-actions` | — | Bearer | `GET /api/user/bot-actions` |
| `bot-events` | — | Bearer | `GET /api/user/bot-events` |

注：`--actions` 接受逗号分隔的 action 名称（如 `bot.message.send,bot.conversation.list`），默认 `bot.message.send`。Token 创建/轮换返回的 `plaintext_token` 只在响应中出现一次。软删除 Bot 会禁用 Bot 并撤销所有 Token，不物理删除历史消息。

### --profile 参数

支持多用户并行测试。不同 profile 的 token 独立持久化。

```bash
# Alice
python aim_test.py login --email alice@t.com --password 12345678 --profile alice
python aim_test.py ws-connect --profile alice

# Bob（另一个终端）
python aim_test.py login --email bob@t.com --password 12345678 --profile bob
python aim_test.py ws-connect --profile bob
```

---

## 交互模式

```bash
python aim_test.py interactive
```

交互提示符格式：`aim [<profile>] [#<user_id>] <ws_state>>`

| 符号 | 含义 |
|------|------|
| `⚡` | WebSocket 已连接 |
| `·` | WebSocket 未连接 |
| `#?` | 未登录 |

### 交互命令

```
┌─ Auth ───────────────────────────────────────────┐
│  register <email> <password> <username>           │
│  login <email> <password> [--profile NAME]        │
│  refresh | logout                                 │
├─ Users & Friends ────────────────────────────────┤
│  search <name>          user <id>                 │
│  friend-add <id>        friend-apps               │
│  friend-accept <id>     friend-reject <id>        │
│  friend-list          presence-friends             │
├─ Conversations ──────────────────────────────────┤
│  conv-create <member_id> [name]  (or comma-sep)    │
│  group-create <member_id> [name] (or comma-sep)    │
│  conv-members <conv_id>                            │
│  conv-add-members <conv_id> <uid,uid,...>          │
│  conv-remove-member <conv_id> <uid>                │
│  conv-leave <conv_id>                              │
│  conv-dismiss <conv_id>                            │
│  conv-update <conv_id> [--name N] [--avatar A]     │
│  history <conversation_id> [limit]                │
├─ WebSocket ───────────────────────────────────────┤
│  ws-connect [--profile NAME]                      │
│  ws-send <conv_id> <text> [--profile NAME]        │
│  ws-heartbeat [last_seq] [--profile NAME]         │
│  ws-typing <id> [--profile NAME]                  │
│  ws-read-receipt <conv_id> <last_msg_id>          │
│  ws-ack <ack_seq> [--profile NAME]                │
│  ws-recv (wait for incoming frames)               │
├─ Profiles ────────────────────────────────────────┤
│  switch <profile>   change active profile          │
│  status             show all profiles              │
├─ Meta ────────────────────────────────────────────┤
│  help | quit | exit | status                      │
└───────────────────────────────────────────────────┘
```

### 群聊示例（交互模式）

```bash
aim [default] [#1] ·> conv-create 2,3,4 MyGroup
✓ Conversation #5 created (group)
  Members: [2, 3, 4]
  Name: MyGroup
aim [default] [#1] ·> conv-members 5
✓ 4 member(s) in conversation #5
aim [default] [#1] ·> conv-add-members 5 6,7
✓ Added 2 member(s)
aim [default] [#1] ·> conv-update 5 --name "New Name"
✓ Updated conversation #5
aim [default] [#1] ·> conv-remove-member 5 7
✓ Removed user #7
```

### Bot 管理（交互模式）

```bash
# 创建 Bot
aim [default] [#1] ·> bot-create my-bot bot@example.com
✓ Bot created: #9000000001 (my-bot)

# 查看 Bot 列表
aim [default] [#1] ·> bot-list
✓ 2 bot(s):
  #9000000001: my-bot [1]

# 查看 Bot 详情
aim [default] [#1] ·> bot-get 9000000001
{ "bot_user_id": 9000000001, ... }

# 更新 Bot 昵称
aim [default] [#1] ·> bot-update 9000000001 new-name
✓ Bot updated: new-name

# 启用/禁用
aim [default] [#1] ·> bot-enable 9000000001
aim [default] [#1] ·> bot-disable 9000000001

# 删除（软删除）
aim [default] [#1] ·> bot-delete 9000000001
✓ Deleted: true

# 创建 Token（默认 bot.message.send）
aim [default] [#1] ·> bot-token-create 9000000001
✓ Token created: token_id=...
⚠  Token: aim_bot_... (shown once)

# 创建 Token 带自定义 action
aim [default] [#1] ·> bot-token-create 9000000001 bot.message.send,bot.conversation.list

# 列出 Token
aim [default] [#1] ·> bot-token-list 9000000001

# 轮换 Token
aim [default] [#1] ·> bot-token-rotate 9000000001 <token_id>
✓ Token rotated: ...
⚠  New token: aim_bot_... (shown once)

# 撤销 Token
aim [default] [#1] ·> bot-token-revoke 9000000001 <token_id>
✓ Revoked: true

# 加入群聊
aim [default] [#1] ·> bot-add-conv 9000000001 5
✓ Bot #9000000001 added to conversation #5

# 创建 user-bot direct 会话
aim [default] [#1] ·> bot-direct-conv 9000000001
✓ Direct conversation: #6

# 查看可用 action
aim [default] [#1] ·> bot-actions

# 查看可用 webhook event
aim [default] [#1] ·> bot-events
```

### Profile 切换

```bash
aim [default] [#1] ·> switch bob
✓ Switched to profile 'bob'
aim [bob] [#2] ·> login bob@t.com 12345678
```

---

## run-all 集成测试流程

```
1.  Register & Login (alice + bob)
2.  Search users
3.  Friend request (Alice → Bob)
4.  Accept friend (Bob → Alice)
4.5 Friend lists (verify both sides)
4.6 Friend presence endpoint
5.  Create conversation
6.  Get history (empty)
7.  WebSocket connect (both)
8.  Alice sends → Bob receives (push verify)
9.  Get history (after send)
10. Refresh token
11. Disconnect WS
12. Logout (both)
```

---

## WebSocket 帧类型速查

### 客户端 → 网关

| 帧类型 | 枚举值 | Payload |
|--------|--------|---------|
| `SEND_MESSAGE` | 1 | `SendMessagePayload` (conversation_id, message_type, content, client_msg_id) |
| `HEARTBEAT` | 2 | `HeartbeatPayload` (last_seq) |
| `TYPING` | 3 | `TypingPayload` (conversation_id) |
| `READ_RECEIPT` | 4 | `ReadReceiptPayload` (conversation_id, last_msg_id) |
| `ACK` | 5 | `ClientAckPayload` (ack_seq) |

### 网关 → 客户端

| 帧类型 | 枚举值 | Payload |
|--------|--------|---------|
| `PUSH_MESSAGE` | 101 | `PushMessagePayload` |
| `PUSH_PRESENCE` | 102 | `PushPresencePayload` |
| `PUSH_NOTIFICATION` | 103 | `PushNotificationPayload` |
| `PUSH_TYPING` | 104 | `PushTypingPayload` |
| `RECONNECT` | 105 | `ReconnectPayload` (drain 窗口) |
| `SERVER_ACK` | 106 | `ServerAckPayload` |
| `TOKEN_EXPIRED` | 107 | `TokenExpiredPayload` |
| `PUSH_FRIEND_APPLICATION` | 108 | `PushFriendApplicationPayload` |
| `PUSH_READ_RECEIPT` | 109 | `PushReadReceiptPayload` (conversation_id, user_id, last_read_message_id, updated_at) |

---

## 新增命令检查清单

当 gateway 新增 REST 端点时，dev tool 需同步更新：

- [ ] `RESTClient` 添加对应方法
- [ ] 添加 `cmd_*` CLI handler 函数
- [ ] `argparse` 添加子命令解析器
- [ ] `commands` dict 添加映射
- [ ] `_print_help()` 更新帮助文本
- [ ] 交互模式添加 `elif cmd == "..."` 分支
- [ ] `run-all` 流程中加入验证步骤（如适用）
- [ ] 模块 docstring 添加使用示例
- [ ] 更新本文档的对应表格

---

## Benchmark 压测命令

```bash
cd dev-tool
python benchmark.py <scenario> [--args]
```

### 压测场景

| 命令 | 参数 | 说明 |
|------|------|------|
| `register` | `--users` [`--rps`] [`--ramp-up`] [`--quiet`] [`--output`] | 批量注册用户 |
| `login` | `--users` [`--rps`] [`--ramp-up`] [`--quiet`] [`--output`] | 批量登录 |
| `friend-chain` | `--users` [`--rps`] [`--ramp-up`] [`--quiet`] [`--output`] | 好友链全流程 |
| `ws-message` | `--users` `--messages-per-user` [`--rps`] [`--duration`] [`--ramp-up`] [`--quiet`] [`--output`] | WS 消息并发 |
| `presence` | `--users` [`--rps`] [`--ramp-up`] [`--quiet`] [`--output`] | 好友在线状态查询压测 |
| `mixed` | `--users` `--duration` [`--rps`] [`--ramp-up`] [`--ws-ratio`] [`--quiet`] [`--output`] | 混合负载 |

### 通用参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--users` | int | 100 | 并发用户数 |
| `--rps` | float | 0 | 目标 RPS（0 = 不限） |
| `--duration` | float | 0 | 持续时间秒（0 = 跑完即止） |
| `--ramp-up` | float | 0 | 渐进加压秒数 |
| `--output` | str | — | JSON 报告输出路径 |
| `--quiet` | flag | — | 静默模式：拑制 WSClient 逓帧调试输出、setup-阶段 LoadGenerator 进度条/小计、websocket-client/urllib3 logger，只保留场景标题、步骤指示与最终报告。适用于所有场景。 |

### 场景特有参数

| 场景 | 参数 | 类型 | 默认值 | 说明 |
|------|------|------|--------|------|
| `ws-message` | `--messages-per-user` | int | 100 | 每用户消息数 |
| `mixed` | `--ws-ratio` | float | 0.7 | WS 请求占比 |
| `mixed` | `--duration` | float | 30 | 持续时间（秒） |

### 示例

```bash
# 快速容量验证
python benchmark.py register --users 10

# 渐进加压：500 用户、50 RPS、10s ramp-up
python benchmark.py register --users 500 --rps 50 --ramp-up 10

# 好友在线状态查询压测
python benchmark.py presence --users 50 --rps 100

# WS 消息压测 + 静默
python benchmark.py ws-message --users 50 --messages-per-user 500 --quiet

# 混合持续负载 + JSON 报告
python benchmark.py mixed --users 100 --duration 60 --rps 200 --output report.json

# 全力压测（不限速）
python benchmark.py ws-message --users 100 --messages-per-user 1000 --quiet
```

### 输出示例

```
  Scenario: Register -- 100 users, RPS=50.0

[==============================]  100%  req=100  qps=50.0  p50=105ms  p95=108ms

------------------------------------------------------------
  Benchmark Results
------------------------------------------------------------
  Duration:        2.0s
  Total Requests:  100
  Success:         100
  Errors:          0 (0.0%)
  Avg QPS:         50.0
  -- Latency (ms) --
  Min:  102.0ms   Avg:  105.5ms   Max:  110.0ms
  P50:  105.0ms   P90:  108.0ms   P95:  108.0ms   P99:  110.0ms
  -- Latency Histogram --
     < 1ms                        (0)
     1-5ms                        (0)
    5-10ms                        (0)
   10-25ms                        (0)
   25-50ms                        (0)
  50-100ms                        (0)
  100-250ms  ####################  (100)
  ...
------------------------------------------------------------
```

### 指标含义

| 指标 | 含义 |
|------|------|
| `Duration` | 压测实际耗时 |
| `Total Requests` | 总请求数（含失败） |
| `Avg QPS` | 平均每秒请求数 |
| `P50` | 50% 请求延迟低于此值（中位数） |
| `P95` | 95% 请求延迟低于此值 |
| `P99` | 99% 请求延迟低于此值（长尾） |
| `Error Types` | 错误细分：`api_40400`、`server_ack_40300`、`recv_timeout`、`ConnectionError` 等 |
