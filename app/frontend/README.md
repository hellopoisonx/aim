# AIM Desktop Client

This is the AIM desktop client, built with Wails v2 (Go) + Vue 3 + TypeScript + Element Plus.

## About

This project was scaffolded from the official Wails Vue-TS template.

You can configure the project by editing `wails.json`. More information about the project settings can be found
here: https://wails.io/docs/reference/project-config

## Live Development

To run in live development mode, run `wails dev` in the project directory. This will run a Vite development
server that will provide very fast hot reload of your frontend changes. If you want to develop in a browser
and have access to your Go methods, there is also a dev server that runs on http://localhost:34115. Connect
to this in your browser, and you can call your Go code from devtools.

## Architecture

```
Vue (frontend/) ──(Wails Bindings)──→ App (Go) ──→ RESTClient ──HTTP──→ Gateway
                                         ├── wsclient ──WS──→ Gateway
                                         └── device
```

- **Vue 层** 不直接调用 REST/WS；通过 Wails 生成的 `../wailsjs/go/main/App` 绑定调用 `App` 公开方法。
- **Go 层** `app/frontend/app.go` 持有 `RESTClient`（HTTP）和 `wsclient.Client`（WebSocket），封装所有网关通信。
- **Protocol**：REST 使用 `Envelope` JSON 包裹格式；WebSocket 使用 protobuf 二进制帧（`WsFrame`）。

> 领域知识详见 `.opencode/skills/aim-frontend-domain/`。

## 当前覆盖的 REST 端点（共 15 个）

| Method | Path |
|---|---|
| POST | `/api/auth/register` |
| POST | `/api/auth/login` |
| POST | `/api/auth/refresh` |
| POST | `/api/auth/logout` |
| GET | `/api/users/by-name/{name}` |
| GET | `/api/users/by-id/{id}` |
| POST | `/api/conversations` |
| GET | `/api/conversations` |
| GET | `/api/conversations/history/{id}` |
| POST | `/api/users/friends/{id}` |
| POST | `/api/friends/accept/{id}` |
| POST | `/api/friends/reject/{id}` |
| GET | `/api/friends/me` |
| GET | `/api/friends/applications` |
| GET | `/ws` |

## 登录后数据流

1. `Login()` → 设置 `currentUserId` / `currentUserLabel`
2. `ConnectWS()` → 建立 WebSocket 连接
3. `ListConversations()` → 从服务端加载会话列表
4. 对每个会话：通过 `GetUserById()` 解析成员显示名，`loadConversationHistory()` 预拉最新消息

## Building

To build a redistributable, production mode package, use `wails build`.
