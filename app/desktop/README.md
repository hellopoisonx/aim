# AIM Desktop

Wails v2 + Vue 3 + Arco Design 桌面客户端。

## 默认配置

- REST Gateway：`http://localhost:8888`
- WebSocket：`ws://localhost:8888/ws`
- Gateway 配置保存到系统用户配置目录下的 `aim-desktop/config.json`
- 支持同一台设备保存多个账号：每个账号在 `config.json` 的 `accounts[]` 中维护独立 `device_id`、token 与用户资料
- 本地缓存按账号隔离，SQLite 路径为 `aim-desktop/accounts/{user_id}/cache.db`

## 本地缓存与同步

桌面端使用 SQLite 缓存会话、消息、好友、群成员、已读状态和在线状态。缓存目录按 `user_id` 分开，切换账号时会断开当前 WS、关闭当前账号 DB，再打开目标账号 DB，避免不同账号的会话/消息/好友缓存混用。首次升级会将旧版单账号 `aim-desktop/cache.db` 迁移到唯一已知账号目录。

消息同步遵循：

1. REST 历史消息、WS 推送消息、发送 ACK 回填统一走 upsert 入库路径。
2. `messages.message_id` 唯一，`(conversation_id, client_msg_id)` 唯一，重复分页/重复推送只更新同一行。
3. `conversation_sync` 保存每个会话的本地游标：`min_message_id`、`max_message_id`、`max_created_at`、`last_synced_at`、`has_more_before`。
4. 启动/重连/切换会话时先展示 SQLite 缓存，再通过历史接口分页补齐云端增量。
5. 本机发送先插入 `pending` 消息，服务端 ACK 后用 `client_msg_id` 回填 `message_id/status`，后续 WS 推送同一消息不会重复插入。

## 开发依赖

- Go 1.26+
- Wails CLI v2：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Node.js
- pnpm：`npm install -g pnpm`

## 安装与构建

```bash
cd app/desktop/frontend
pnpm install
pnpm build
```

```bash
cd app/desktop
go test ./...
wails generate module
wails dev
```

生产构建：

```bash
cd app/desktop
wails build
```

## 重新生成 Wails 绑定

Go 绑定方法修改后执行：

```bash
cd app/desktop
wails generate module
```

生成结果位于 `frontend/wailsjs`，前端通过：

```ts
import * as api from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
```

调用 Go 方法和接收 WS 事件。

## 多账号管理 API

桌面端通过以下 Go 绑定方法管理多账号（前端通过 `../wailsjs/go/main/App` 调用）：

| 前端方法 | 说明 |
|---|---|
| `ListAccounts()` | 返回所有已保存账号的列表，含活跃态与登录态 |
| `SwitchAccount(userID)` | 切换到指定账号（断开旧 WS、切换 SQLite 缓存、建立新 WS） |
| `Login(input)` | 登录后自动 upsert 到账号列表并设为活跃 |
| `Logout()` | 清除当前账号 token，保留账号记录 |

账号视图类型 `AccountView` 包含 `user_id` / `email` / `nickname` / `active` / `logged_in` / `display_name` 字段。
