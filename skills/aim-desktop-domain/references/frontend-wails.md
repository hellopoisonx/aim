# Desktop 前端与 Wails 绑定

## 技术栈

- Wails v2：Go 后端 + WebView 桌面壳。
- Vue 3：`<script setup>` 单页应用。
- Arco Design Vue：布局、表单、列表、弹窗、抽屉、消息提示。
- TypeScript：前端类型检查。
- Vite：前端构建。

## 目录

```text
app/desktop/frontend/
├── src/App.vue          # 主界面与状态
├── src/main.ts          # Vue bootstrap
├── src/style.css        # 全局样式
├── wailsjs/             # Wails 生成绑定
├── package.json         # pnpm scripts
└── vite.config.ts
```

## 前端状态模型

`App.vue` 当前维护：

- 认证：`authMode`、`authForm`、`session`、`accounts`、`addingAccount`。
- 连接：`connected`。
- 会话：`conversations`、`activeConversation`、`messages`、`draft`。
- 好友：`friends`、`searchName`、`searchResults`。
- 群管理：`members`、创建群/添加成员/编辑群资料表单。
- 设置：`config`、`showSettings`。

## Wails Go 绑定

前端从 `../wailsjs/go/main/App` 调用 Go 方法：

| 方法 | 用途 |
|---|---|
| `GetConfig()` / `SaveConfig(input)` | 读写 Gateway 地址。 |
| `ListAccounts()` / `SwitchAccount(userID)` | 多账号列表与切换。 |
| `Register(input)` / `Login(input)` / `AutoLogin()` / `RefreshToken()` / `Logout()` | 认证与 Token 生命周期。 |
| `ListConversations()` / `GetCachedConversations()` | 会话列表。 |
| `GetConversationHistory()` / `GetCachedMessages()` | 历史消息与本地缓存。 |
| `SendMessage()` / `SendTyping()` / `SendReadReceipt()` | WS 消息、输入中、已读。 |
| `SearchUsers()` / `AddFriend()` / `ListFriends()` / `GetCachedFriends()` | 用户搜索与好友。 |
| `CreateConversation()` / `CreateGroup()` | 创建会话/群聊。 |
| `GetConversationMembers()` / `GetCachedConversationMembers()` | 成员列表。 |
| `AddGroupMembers()` / `RemoveGroupMember()` / `LeaveGroup()` / `DismissGroup()` / `UpdateGroupInfo()` | 群管理。 |

Go 方法签名、DTO 或包名变化后，必须重新生成绑定：

```bash
cd app/desktop
wails generate module
```

生成文件位于 `frontend/wailsjs`。不要手改这些文件。

## Runtime Events

前端从 `../wailsjs/runtime/runtime` 使用 `EventsOn` 订阅 Go 侧事件：

| 事件 | Payload | 前端行为 |
|---|---|---|
| `ws:connection` | `{ connected, error? }` | 更新连接状态。 |
| `ws:message` | `MessageView` | 合并/追加消息。 |
| `ws:server-ack` | `ServerAckView` | 按 `client_msg_id` 更新发送状态和 `message_id`。 |
| `ws:presence` | `PresenceView` | 更新在线状态展示。 |
| `ws:typing` | `TypingView` | 展示输入中提示（如实现）。 |
| `ws:read-receipt` | `ReadReceiptView` | 更新已读展示（如实现）。 |
| `ws:friend-application` | `FriendApplicationView` | 刷新好友申请（如实现）。 |
| `ws:token-expired` | `{ at }` | 调用 `RefreshToken()`。 |

## UI 流程

### 启动

`onMounted()`：

1. `GetConfig()`。
2. `ListAccounts()`。
3. `AutoLogin()`。
4. 若自动登录成功，先加载缓存，再刷新会话和好友。
5. 注册所有 WS runtime event 监听器。

### 多账号

- Header 使用 `a-select` 展示本机账号。
- `SwitchAccount(userID)` 成功后清空当前会话/消息状态，再加载目标账号缓存与服务端数据。
- “添加账号”打开认证面板，不删除已有账号。
- “退出当前账号”只清 Token，保留账号记录。

### 会话和消息

- 选择会话先调用 `GetCachedMessages()` 快速展示，再调用 `GetConversationHistory()` 拉取云端历史。
- 发送消息调用 `SendMessage()`；Go 侧先写 pending，前端收到返回值立即合并。
- `ws:server-ack` 按 `client_msg_id` 合并发送结果。
- `ws:message` 按 `message_id` / `client_msg_id` 合并，避免重复展示。

## 构建与检查

```bash
cd app/desktop/frontend
pnpm install
pnpm build
```

`pnpm build` 会先运行 `vue-tsc --noEmit`，再执行 `vite build`。
