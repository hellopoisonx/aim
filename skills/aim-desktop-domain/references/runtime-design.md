# Desktop 运行时设计

## 启动与生命周期

Wails 入口位于 `app/desktop/main.go`：

1. `main()` 创建 `NewApp()`。
2. `wails.Run` 嵌入 `frontend/dist`，绑定 `App` 到前端。
3. `OnStartup` 调用 `app.startup(ctx)`：加载配置、初始化 REST client、打开活跃账号 DB。
4. `OnShutdown` 调用 `app.shutdown(ctx)`：取消上下文、断开 WS、关闭 DB。

默认窗口：`1280x820`，最小尺寸 `1080x680`。

## 配置

默认配置：

| 项 | 默认值 | 存储位置 |
|---|---|---|
| Gateway REST | `http://localhost:8888` | `config.json.gateway_url` |
| Gateway WS | `ws://localhost:8888/ws` | `config.json.ws_url` |
| 配置根目录 | 系统用户配置目录下 `aim-desktop` | `appDir()` |

前端通过 `GetConfig()` / `SaveConfig()` 读取和保存 Gateway 地址。保存后 Go 侧会更新 REST client base URL。

## 认证流程

### 注册

`Register(input)`：

1. 查找同邮箱账号，若存在复用原 `device_id`，否则生成新 UUID。
2. 调用 `POST /api/auth/register`。
3. 返回注册出的 `user_id`；当前前端注册后再执行登录。

### 登录

`Login(input)`：

1. 查找同邮箱账号，复用其 `device_id`，否则生成新 UUID。
2. 调用 `POST /api/auth/login` 获取 Token。
3. 构造/更新 `AccountProfile`，写入 `accounts[]`。
4. 设置 `active_user_id`。
5. `resetRuntimeLocked()` 切换 DB/WS 上下文。
6. 异步建立 WS。
7. 返回 `SessionInfo` 给前端。

### 自动登录

`AutoLogin()`：

- 无活跃账号或无 Token 时返回空 Session。
- 若 access token 即将过期（小于约 1 分钟），优先刷新 Token。
- 有可用 access token 后异步连接 WS。

### 登出

`Logout()`：

- 若当前账号有 access token，best-effort 调用 `POST /api/auth/logout`。
- 断开当前 WS。
- 清空当前账号 Token，但保留账号记录、用户资料与本地缓存。
- 发出 `ws:connection { connected: false }`。

## Token 刷新

`token()` 和 `AutoLogin()` 都会在 access token 即将过期时调用 `refreshTokenLocked()`：

1. 使用 refresh token 调 `POST /api/auth/refresh`。
2. 更新账号 access/refresh/expires_at。
3. 保存 `config.json`。
4. 重建 WS，让新 access token 生效。

收到 WS `TOKEN_EXPIRED` 事件时，前端调用 `RefreshToken()`。

## WebSocket

Go 侧 WS client 位于 `internal/ws/client.go`：

- 使用 Bearer Token 建连。
- 使用 Protobuf binary frame。
- 每 20 秒发送一次 `HEARTBEAT(last_seq)`。
- 所有写操作由 `writeMu` 串行化。
- 推送类事件处理后发送 `CLIENT_ACK(ack_seq)`。

当前 `App` 绑定方法：

| 方法 | WS 帧 |
|---|---|
| `SendMessage(cid, typ, content, mentions)` | `SEND_MESSAGE` |
| `SendTyping(cid)` | `TYPING` |
| `SendReadReceipt(cid, lastMsgID)` | `READ_RECEIPT` |

## 数据同步

启动或切换账号后，前端流程：

1. `GetConfig()` / `ListAccounts()`。
2. `AutoLogin()`。
3. 若已有登录态，先加载本地缓存：`GetCachedConversations()`、选中会话后 `GetCachedMessages()`、`GetCachedFriends()`。
4. 再拉服务端：`ListConversations()`、`ListFriends()`、`GetFriendsPresence()`。
5. 选中会话后调用 `GetConversationHistory()` 分页同步历史。
6. 新消息通过 WS `ws:message` 增量进入前端和本地缓存。

发送消息流程：

1. 生成稳定 `client_msg_id`。
2. 写入本地 pending 消息。
3. 如 WS 未连接则尝试连接。
4. 发送 `SEND_MESSAGE`。
5. 收到 `SERVER_ACK` 后按 `client_msg_id` 回填状态和服务端 `message_id`。

## 群管理

前端通过 Wails 绑定调用：

- `CreateGroup(req)`：创建群聊。
- `GetConversationMembers(cid)` / `GetCachedConversationMembers(cid)`：成员列表。
- `AddGroupMembers(cid, ids)`：添加成员。
- `RemoveGroupMember(cid, uid)`：移除成员。
- `LeaveGroup(cid)`：退出群聊。
- `DismissGroup(cid)`：解散群聊。
- `UpdateGroupInfo(cid, req)`：更新群资料。

## 修改检查

```bash
cd app/desktop
go test ./...

cd frontend
pnpm build
```

Go 绑定方法或 DTO 修改后：

```bash
cd app/desktop
wails generate module
```
