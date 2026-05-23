# TUI 运行时设计

## 启动参数

- `--gateway`：gateway REST base URL，默认 `AIM_GATEWAY_HTTP` 或 `http://127.0.0.1:8888`。
- `--ws`：gateway WebSocket URL，默认 `AIM_GATEWAY_WS` 或 `ws://127.0.0.1:8888/ws`。
- `--email --password`：启动后自动登录；二者必须同时提供。
- `--instance`：本地实例 ID。为空时生成随机 ID，因此同一电脑同时启动多个 TUI 默认互不干扰。
- `--db`：SQLite 文件路径。为空时使用用户 cache 目录下按实例 ID 命名的数据库文件。

## 认证界面

未登录时进入 `phaseAuth`：

- 支持登录 / 注册（Tab 切换模式与字段）。
- 注册成功后自动登录并进入主界面。
- 本地已有 token 时启动直接进入 `phaseMain` 并执行 session bootstrap（刷新 token、WS 连接、拉取会话/好友/申请/presence）。

也可使用 `--email --password` 或 `/` 命令模式执行 `login` / `register`。

## 布局

Bubble Tea 主界面：

- **消息页**（三栏）：菜单 | 会话列表 | 聊天区（消息 + 输入框）。
- **好友页**（两栏）：菜单 | 搜索 + 好友申请 + 好友列表。

导航：

- `←/→` 在菜单、列表、输入框等区域间移动焦点。
- `↑/↓` 在菜单项、会话、好友、申请间移动选择。
- `Enter`：提交输入、拉取历史、加好友、接受申请、发起私聊等。

发送消息：在消息页输入框输入后 `Enter` 发送（底层走 WS `SEND_MESSAGE`）。

输入状态：在消息输入框打字时 debounce（约 2s）发送 `TYPING`；收到 `PUSH_TYPING` 在会话标题下显示「正在输入」；约 4s 自动消失。

已读回执：切换会话或拉历史后自动 `READ_RECEIPT`；浏览当前会话收到新消息时自动已读；历史 `read_states` 与 `PUSH_READ_RECEIPT` 在聊天区展示「已读: #uid→#msg」。

菜单动作：**创建群聊**（弹窗填群名 + 从好友/最近搜索结果多选成员，Space 勾选）、**退出登录**。

好友页：搜索用户 / 好友列表 / 好友申请均为列表点选（Enter 添加、私聊、接受；`r` 拒绝申请），无需手输 user id。

命令模式：按 `/` 输入 `help` 所列命令（与 dev-tool 对齐的 REST/WS 调试面）。

## 在线状态颜色

TUI 根据 presence 快照和 WS presence 推送维护用户状态：

- 绿点 `●`：会话中除自己以外至少一个成员 online。
- 红点 `●`：无在线成员或状态未知，视为 offline。

Presence 来源：

- 后台周期调用 `GET /api/presence/friends`。
- WebSocket `PUSH_PRESENCE` 实时更新。

## Token 无感刷新

- TUI 启动登录或加载本地 token 后，会周期检查 `expires_at`。
- 距离过期不足 60 秒时调用 `POST /api/auth/refresh`。
- 刷新成功后写入 SQLite，并重建 WebSocket 连接使新 access token 生效。
- 收到 WS `TOKEN_EXPIRED` 帧时立即触发刷新。

## 多实例隔离

- 默认每次启动生成随机 `--instance`，对应独立 SQLite 文件和独立 device_id。
- 也可显式传入不同 `--instance` 或不同 `--db`。
- 同一 DB 文件内所有表均包含 `instance_id` 分区键，避免 token、会话、消息、presence 串扰。
- **不要**在两个终端里用相同的 `--db`（或固定 `AIM_TUI_INSTANCE` 导致同库）同时跑 TUI：会争用 refresh token/device_id，并触发数据库文件锁。
- 同时开两个 TUI 时，推荐各自默认启动（随机 instance），或显式 `--instance a` / `--instance b`。
