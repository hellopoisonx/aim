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
| `register` | `--email` `--password` [`--username`] [`--avatar`] | 无 | `POST /api/auth/register` |
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

### 会话

| 命令 | 参数 | 鉴权 | 对应 API |
|------|------|------|----------|
| `conv-create` | `--member-id` 或 `--member-ids` | Bearer | `POST /api/conversations` |
| `history` | `--conversation-id` [`--limit`] [`--cursor-created-at`] [`--cursor-id`] | Bearer | `GET /api/conversations/history/:id` |

### WebSocket

| 命令 | 参数 | 鉴权 | 帧类型 |
|------|------|------|--------|
| `ws-connect` | [`--profile`] | Bearer (header) | — |
| `ws-send` | `--conversation-id` `--content` [`--message-type`] [`--profile`] | Bearer | `SEND_MESSAGE` |
| `ws-heartbeat` | [`--profile`] | Bearer | `HEARTBEAT` |
| `ws-typing` | `--conversation-id` [`--profile`] | Bearer | `TYPING` |

### 元命令

| 命令 | 说明 |
|------|------|
| `interactive` | 进入 REPL 交互模式 |
| `run-all` | 运行全流程集成测试（注册→登录→好友→会话→WS→注销） |

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
│  register <email> <password> [username]           │
│  login <email> <password> [--profile NAME]        │
│  refresh | logout                                 │
├─ Users & Friends ────────────────────────────────┤
│  search <name>          user <id>                 │
│  friend-add <id>        friend-apps               │
│  friend-accept <id>     friend-reject <id>        │
│  friend-list                                       │
├─ Conversations ──────────────────────────────────┤
│  conv-create <member_id>  (or comma-sep for group)│
│  history <conversation_id> [limit]                │
├─ WebSocket ───────────────────────────────────────┤
│  ws-connect [--profile NAME]                      │
│  ws-send <conv_id> <text> [--profile NAME]        │
│  ws-heartbeat [--profile NAME]                    │
│  ws-typing <id> [--profile NAME]                  │
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
aim [default] [#1] ·> conv-create 2,3,4
✓ Conversation #5 created (group)
  Members: [2, 3, 4]
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
