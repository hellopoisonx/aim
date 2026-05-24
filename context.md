# TUI 实现上下文总结

## 1. TUI 架构与边界

### 模块结构

```
app/tui/
├── main.go              # Bubble Tea 主循环、布局、键盘事件、状态结构体
├── commands.go          # 命令分发器（REST 调用 + WS 调用）
├── store.go             # SQLite 本地持久化（token/会话/消息/presence）
├── store_test.go        # 隔离性、并发测试
├── ws_handlers.go       # WS 帧回调（pushMessageToItem、handleServerAck、scheduleWSReconnect）
├── ws_handlers_test.go  # 推送帧解析测试
├── ui_auth.go           # 登录/注册认证页面（phaseAuth）
├── ui_overlay.go        # 创建群聊叠加层
├── ui_mention.go        # @ 提及选择器
├── ui_mention_test.go   # 提及测试
├── ui_picker.go         # 用户选择/标签缓存帮助方法
├── ui_receipt_typing.go # 已读回执 + 输入中状态渲染和处理
├── ui_receipt_typing_test.go # 已读回执测试
├── ui_picker_test.go    # 选择器测试
├── internal/
│   ├── client/
│   │   ├── client.go    # REST 客户端（所有 API 调用封装）
│   │   └── client_test.go
│   └── wsclient/
│       ├── wsclient.go  # WebSocket 客户端（Binary Protobuf 帧）
│       └── wsclient_test.go
└── .pi/agent/trust/lsp.json
```

### 边界

- **TUI 只和 gateway 通信**：REST → `internal/client`，WS → `internal/wsclient`。
- **不直接导入 auth/core/logic 的内部包**。
- **本地状态是客户端缓存**，非服务端权威数据。

## 2. 关键流程

### 2.1 启动参数

| 参数 | 环境变量 | 默认值 |
| --- | --- | --- |
| `--gateway` | `AIM_GATEWAY_HTTP` | `http://127.0.0.1:8888` |
| `--ws` | `AIM_GATEWAY_WS` | `ws://127.0.0.1:8888/ws` |
| `--email --password` | `AIM_TUI_EMAIL/PASSWORD` | 空（可同时提供自动登录） |
| `--instance` | `AIM_TUI_INSTANCE` | 随机生成（默认隔离） |
| `--db` | `AIM_TUI_DB` | `<UserCacheDir>/aim/tui/<instance_id>.db` |

### 2.2 认证流程

- 未登录 → `phaseAuth` 页面（Tab 切换 登录/注册，填写邮箱/密码/昵称）。
- 注册成功后自动调用登录。
- 登录成功后：`POST /api/auth/login` → 获取 `{access_token, refresh_token, expires_at, user_id}` → 写入 SQLite `tokens` 表 → 调用 `cmdWSConnect` → 调用 `cmdConversations` → 拉取历史。
- 启动时已有本地 token → 直接进入 `phaseMain`，调用 `cmdBootstrapSession`，内含刷新 token、WS 连接、拉会话/好友/申请/presence。
- 登出：`POST /api/auth/logout` → 服务端调用 `KickUser` 关闭 WS → TUI 清 token 回到 `phaseAuth`。

### 2.3 WebSocket 连接

- 升级协议：`GET /ws` 带 `Authorization: Bearer <access_token>`（**只支持 Header**，不支持 `?token=`）。
- 帧协议：Protobuf Binary，通过 `shared/proto/ws/ws.proto` 定义。
- 心跳：每 `~20s` 发送 `HEARTBEAT`（PresenceTTL 默认 45s）。
- 写入串行化：`writeMu sync.Mutex` 避免并发写 `Conn.Write` 卡死。
- 读循环：`readLoop` goroutine 持续读取，调用 `OnFrame` 回调。
- 帧回调 → `handleFrame` → 通过 `notifyUI` 将状态更新发到 `notifyCh` → 由 Bubble Tea 主 goroutine 执行，避免并发访问 `model.state`。
- 自动 ACK：`PUSH_MESSAGE`、`PUSH_READ_RECEIPT`、`PUSH_NOTIFICATION` 收到后自动发送 `CLIENT_ACK`。
- 重连：收到 `RECONNECT` 时延迟 `reconnect_delay_ms` 后断开并重新连接；收到 `TOKEN_EXPIRED` 时立即刷新 token 并重连。

### 2.4 本地 SQLite 存储

- **同进程**：`SetMaxOpenConns(1)` 避免并发写冲突。
- **跨进程**：`*.lock` 文件互斥（`flock.TryLock()`），第二个 TUI 启动失败提示换 `--instance`/`--db`。
- 表：`tokens`（PK `instance_id`）、`conversations`（PK `instance_id, conversation_id`）、`messages`（PK `instance_id, conversation_id, message_id, client_msg_id`）、`presence`（PK `instance_id, user_id`）。
- 乐观消息：发送时先保存 `client_msg_id` 到 SQLite → 收到 `SERVER_ACK ACCEPTED` 后用服务端 `message_id` 替换（`ReplacePendingMessage` → `DELETE` 旧 pending + `INSERT OR REPLACE`）。
- 限流 42900 REJECTED：移除本地对应 `client_msg_id` 的乐观消息。

### 2.5 多实例隔离

- 每个 `--instance` 对应独立 `device_id`（前缀 `tui-<instance_id>-` + random）。
- 所有 SQLite 表含 `instance_id` 分区键。
- 两个 TUI 用相同 `--db` 启动会被文件锁拒绝。
- 登录/注册时 `DeviceID` 必传，格式 `tui-<instance_id>-<short_uuid>`。

## 3. 已知约定与最近变更

### 协议约定

| 项目 | 约定 |
| --- | --- |
| `conversation_type` | 全栈统一 `direct`/`group`；代码中 `normalizeConversationType` 将 `"single"` 转 `"direct"` |
| `mentions` | 字符串形式（如 `"42"`），TUI `SendMessagePayload.mentions` 和 `PushMessagePayload.mentions` 均如此 |
| 时间戳 | 全部 Unix 毫秒（`int64`） |
| REST 信封 | `{"code":0,"msg":"ok","body":{}}` |
| WS 帧 | Protobuf Binary，`WsFrame{type, seq, payload, timestamp}` |
| 已读回执 | 切换会话/拉历史后自动发送 `READ_RECEIPT`；当前会话新消息自动已读 |
| 输入中 | `TYPING` 帧发 debounce 约 2s；收到 `PUSH_TYPING` 显示约 4s 后消失 |
| 好友页 | Enter 搜索/加好友/私聊；`r` 拒绝申请 |

### 最近变更（2026-05-23）

- 修复双 TUI 同环境卡死：跨进程 DB 文件锁、`events` 通道不再阻塞 WS、WS 状态更新统一走 `notifyUI` 主 goroutine；登录时生成唯一 `device_id`。
- 已读/输入中全链路：历史 `read_states` 解析、`PUSH_READ_RECEIPT`/`PUSH_TYPING` 经 `notifyCh` 在主 goroutine 更新 UI。
- `gateway-api-reference.md` 对齐：`PUSH_READ_RECEIPT` 解码、推送自动 `CLIENT_ACK`、42900 限流 REJECTED 提示并移除乐观消息、`RECONNECT` 延迟重连、`PUSH_NOTIFICATION` 展示、`single`→`direct` 归一化。
- @ 提及全链路：输入 `@` 弹出成员选择，填充 `mentions`；对不可用昵称回退为用户 ID。
- 注册→登录→WS→会话/好友/发消息全链路 UI；认证页、session bootstrap、好友申请与加好友/建会话、WS 心跳；消息页三栏布局。
- 支持 `--email --password` 启动登录、后台 token 刷新、SQLite 保存 token/会话/消息/presence、多实例隔离。

## 4. 实现风险与待修复点

### 高优先级

1. **Token 明文存储**：`store.go` 用 SQLite 明文存 `access_token`/`refresh_token`，生产桌面应接入系统 keychain/credential vault。
2. **重连后未重新拉取 `read_states`**：`scheduleWSReconnectOnMain` 只做了 token 刷新 + WS 重连 + presence，**没有**重新拉取 `read_states`，可能导致已读状态丢失或显示错误。
3. **`notifyCh` 缓冲满风险**：当前容量 128，极端推送风暴可能导致消息丢失（select default 分支静默丢弃）。
4. **`events` 通道满静默丢弃**：`postEvent` 使用 select default，推送风暴时日志行被丢弃但无告警。

### 中优先级

5. **限流消息的本地乐观消息未从 SQLite 清除**：`handleServerAck` 的 REJECTED 分支调用了 `removeMessageByClientMsgID`（仅内存），**没有**调用 `store.ReplacePendingMessage` 或 `Delete` 来清理 SQLite 中的 pending 消息，可能导致重连后仍看到未发送的消息。
6. **`conversationReadStates` 在重连后丢失**：`cmdConversations` 从 REST 刷新会话列表时，`prevReadStates` 只保存了旧 read_states，但 REST 接口返回的 `ListConversationsResponse` 不含 `read_states`；read_states 只来自历史接口。如果进程重启或重连后未拉历史，read_states 为空。
7. **SQLite `messages` 表 `PRIMARY KEY` 包含 `client_msg_id`**：`INSERT OR REPLACE` 可能因 `client_msg_id` 不同产生重复行；`ReplacePendingMessage` 显式 `DELETE` 后 `INSERT`，但其他路径如 `SaveMessages` 没有做类似清理。
8. **多 profile 之间的 WS 连接冲突**：`profiles` map 支持多 profile 但 UI 切换 profile 时不会自动断开旧 WS 连接，可能出现多个活跃 WS 连接。

### 低优先级

9. **`unixMillisTime` 分支逻辑**：检测 `v < 1_000_000_000_000` 判断秒/毫秒，但 Unix 时间戳 1970-09-09 前的合法秒值会被误判为毫秒；建议统一协议强制毫秒。
10. **`history` 命令默认 limit=20**（代码中 `int32(20)`），但 gateway-api-reference 说默认 50，不一致。
11. **好友页 focusChain 不含 focusFriendSearch 以外的中间态**：当 `len(friendApplications) == 0` 时直接从 `focusFriendSearch` 跳到 `focusFriendList`，但 `focusFriendSearch` 焦点状态在搜索结果出现时不会自动切换到 `focusFriendList`，用户需多按一次 `→`。
12. **[2026-05-21 已知] `ui_receipt_typing.go` 的 `formatMessageReadDetail` 函数存在复制粘贴未更新字段名问题**：该函数已从 `messageReadSuffix` 重构而来，但检查发现 `ReadDetails` 字段解析时 `rd.UserID` 与 `rd.IsRead` 在由历史 API 返回的 `MessageReadDetailItem` 结构中引用，需核实后端是否实际返回 `read_details` 字段。

## 5. 快速入口

- **`main.go`**：Bubble Tea `model` 状态结构体、`Update`/`View`、`Init`、`parseConfig` — TUI 顶级调度逻辑。
- **`commands.go`**：命令分发器、登录/消息/会话/WS 等异步操作实现。
- **`ws_handlers.go`**：所有 WS 帧的回调处理方法。
- **`store.go`**：SQLite 持久化层，含跨进程文件锁。
- **`.pi/skills/aim-tui-domain/references/gateway-api-reference.md`**：Gateway 协议详细参考（必读）。