# TUI 运行时设计

## 启动参数

- `--gateway`：gateway REST base URL，默认 `AIM_GATEWAY_HTTP` 或 `http://127.0.0.1:8888`。
- `--ws`：gateway WebSocket URL，默认 `AIM_GATEWAY_WS` 或 `ws://127.0.0.1:8888/ws`。
- `--email --password`：启动后自动登录；二者必须同时提供。
- `--instance`：本地实例 ID。为空时生成随机 ID，因此同一电脑同时启动多个 TUI 默认互不干扰。
- `--db`：SQLite 文件路径。为空时使用用户 cache 目录下按实例 ID 命名的数据库文件。

## 布局

Bubble Tea 主界面是两栏：

- 左栏：对话概览和选择。`↑/↓` 移动选择，`open <conversation_id>` 直接选择。
- 右栏：当前会话窗口，显示本地缓存消息和最近日志。

发送消息：

- `send <text>` 向当前选中会话发送消息。
- `ws-send <conversation_id> <text> [type]` 保留为底层调试命令。

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
