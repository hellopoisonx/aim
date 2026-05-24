# AIM Desktop

Wails v2 + Vue 3 + Arco Design 桌面客户端。

## 默认配置

- REST Gateway：`http://localhost:8888`
- WebSocket：`ws://localhost:8888/ws`
- 配置与登录态保存到系统用户配置目录下的 `aim-desktop/config.json`
- 本地缓存使用 SQLite：`aim-desktop/cache.db`

## 本地缓存与同步

桌面端使用 SQLite 缓存会话、消息、好友、群成员、已读状态和在线状态。消息同步遵循：

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
