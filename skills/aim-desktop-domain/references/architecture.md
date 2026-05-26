# Desktop 架构

## 定位

`app/desktop` 是 AIM 当前桌面客户端，基于 Wails v2 + Vue 3 + Arco Design。它负责用户交互、本地缓存、WebSocket 长连接与 Gateway REST API 调用，不承担服务端业务规则。

## 边界

```text
Desktop Frontend (Vue/Arco)
        │ Wails generated bindings / runtime events
        ▼
Desktop App (Go, app/desktop/app.go)
        ├── internal/api  → Gateway REST
        ├── internal/ws   → Gateway WebSocket Protobuf
        └── internal/store→ config.json + per-account SQLite
        │
        ▼
aim-gateway（唯一服务端入口）
```

规则：

- Desktop 只依赖 gateway 的公开 REST/WS 协议。
- Desktop Go 侧可以依赖 `shared/proto/ws/pb`，用于 WS 二进制帧。
- Desktop 不导入 `app/auth`、`app/core`、`app/logic`、`app/gateway` 的内部包。
- 共享代码进入 `app/shared` 前必须满足横切能力标准；纯桌面能力留在 `app/desktop/internal`。

## 目录

| 路径 | 职责 |
|---|---|
| `main.go` | Wails 启动入口，嵌入前端产物并绑定 `App`。 |
| `app.go` | 应用服务聚合层：配置、账号、Token、WS、SQLite、事件。 |
| `views.go` | Go → TS 绑定 DTO，Snowflake ID 转字符串，展示名兜底。 |
| `internal/api/client.go` | HTTP client，统一 Envelope 解包、Bearer Token、端点封装。 |
| `internal/api/types.go` | REST DTO。 |
| `internal/ws/client.go` | WS client，Protobuf encode/decode，心跳、ACK、发消息、输入中、已读。 |
| `internal/store/config.go` | `config.json`，多账号与旧配置迁移。 |
| `internal/store/sqlite.go` | 账号级 SQLite cache schema 与 upsert/list 方法。 |
| `internal/api/attachments.go` | Gateway REST 附件 client，init/complete/get/download。 |
| `frontend/src/App.vue` | 单页桌面 UI。 |
| `frontend/wailsjs` | Wails 生成的 Go 绑定与 runtime helper。 |

## 运行时对象

`App` 持有以下关键状态：

- `ctx/cancel`：Wails 生命周期上下文。
- `rootDir`：系统用户配置目录下的 `aim-desktop`。
- `cfg/cfgStore`：Gateway 配置、账号列表、活跃账号。
- `api`：Gateway REST client，随配置更新 base URL。
- `db`：当前活跃账号 SQLite 连接。
- `ws`：当前活跃账号 WebSocket 连接。
- `mu`：保护账号切换、Token 刷新、WS/DB 重置等共享状态。

## 事件流

服务端推送通过 WS 进入 Go 侧，再通过 Wails runtime event 通知前端：

| WS 帧 | Go 侧处理 | 前端事件 |
|---|---|---|
| `PUSH_MESSAGE` | upsert 当前账号 DB，发送 `CLIENT_ACK` | `ws:message` |
| `PUSH_PRESENCE` | upsert presence，发送 `CLIENT_ACK` | `ws:presence` |
| `PUSH_TYPING` | 转换为 `TypingView`，发送 `CLIENT_ACK` | `ws:typing` |
| `PUSH_READ_RECEIPT` | 转换为 `ReadReceiptView`，发送 `CLIENT_ACK` | `ws:read-receipt` |
| `PUSH_FRIEND_APPLICATION` | 补展示名，发送 `CLIENT_ACK` | `ws:friend-application` |
| `SERVER_ACK` | 转换发送状态/服务端 message_id | `ws:server-ack` |
| `TOKEN_EXPIRED` | 通知前端触发刷新 | `ws:token-expired` |

WS 建连/断连通过 `ws:connection` 事件广播。

## 多账号隔离

- `prepareWSLocked()` 创建 WS 时捕获账号 `userID`。
- `handleFrameFor(userID, frame)` 先检查 `cfg.ActiveUserID == userID`。
- 切换账号后，旧连接迟到帧会被丢弃，不写入新账号 DB。
- `resetRuntimeLocked()` 总是先断开 WS、发出离线事件，再重新打开目标账号 DB。

## 反模式

- 不要让前端直接拼接服务端 Token 调用非 Gateway 地址。
- 不要把当前账号之外的事件写入当前 DB。
- 不要在一个 `App` 实例中长期同时打开多个账号 DB。
- 不要手改 `frontend/wailsjs`；应修改 Go 绑定后重新生成。
