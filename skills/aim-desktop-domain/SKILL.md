---
name: aim-desktop-domain
description: AIM 的 Wails 桌面客户端域。涉及 `app/desktop`、Vue/Arco 前端、Wails Go 绑定、REST/WS Gateway 客户端、本地 SQLite 缓存、Token 自动刷新、多账号与缓存隔离时使用。
---
# aim-desktop-domain

## 使用范围

当需求涉及以下内容时使用本 Skill：

- `app/desktop` Wails 桌面客户端、Go 绑定方法或生命周期。
- `app/desktop/frontend` Vue 3 + Arco Design 前端页面、状态、交互。
- 桌面端 Gateway REST / WebSocket 对接、Protobuf WS 帧编解码。
- 桌面端本地配置、账号列表、Token 自动刷新、同设备多账号切换。
- 桌面端 SQLite 缓存、离线展示、历史消息分页、会话/好友/成员/在线状态同步。
- Wails 绑定生成、前后端构建、桌面端测试。

## 模块边界

- Desktop 只和 gateway 通信：REST 走 `app/desktop/internal/api`，WebSocket 走 `app/desktop/internal/ws`。
- Desktop 不直接导入 auth/core/logic 的内部包；跨端协议只依赖 `shared/proto/ws/pb`。
- 本地 SQLite 仅作为客户端缓存与离线展示，不是服务端权威状态。
- 同设备多账号必须隔离：每个账号拥有独立 `device_id`、Token 与 `accounts/{user_id}/cache.db`。
- 切换账号必须先断开旧 WebSocket、关闭旧 DB，再打开目标账号 DB 并建立新连接。
- 前端不得绕过 Wails 绑定直接访问本地文件或服务端内部接口。

## 代码地图

| 路径 | 说明 |
|---|---|
| `app/desktop/main.go` | Wails 入口，嵌入 `frontend/dist`，绑定 `App`。 |
| `app/desktop/app.go` | 桌面端应用服务：配置、账号、REST/WS、缓存、事件分发。 |
| `app/desktop/views.go` | 暴露给前端的 DTO、ID 字符串化、展示名归一化。 |
| `app/desktop/internal/api` | Gateway REST client 与响应 DTO。 |
| `app/desktop/internal/ws` | Gateway WebSocket Protobuf client、心跳、ACK、发送消息/输入中/已读。 |
| `app/desktop/internal/store` | `config.json` 与账号级 SQLite 缓存。 |
| `app/desktop/frontend/src/App.vue` | Vue 主界面：登录/注册、多账号、会话、好友、群管理、消息。 |
| `app/desktop/frontend/wailsjs` | Wails 生成绑定；Go 绑定变更后用 `wails generate module` 重建。 |

## 参考资料

- `references/architecture.md` — Desktop 模块边界、运行架构与事件流。
- `references/runtime-design.md` — Wails 生命周期、认证、WS、Token 刷新与同步流程。
- `references/local-store.md` — `config.json` 与 SQLite 缓存表、幂等策略、多账号隔离。
- `references/gateway-api-reference.md` — Desktop 当前使用的 Gateway REST 与 WS 帧协议。
- `references/frontend-wails.md` — Vue/Arco 前端与 Wails 绑定约定。

## 常用命令

```bash
# 前端依赖与构建
cd app/desktop/frontend
pnpm install
pnpm build

# Go 侧测试与绑定生成
cd app/desktop
go test ./...
wails generate module
wails dev

# 生产构建
cd app/desktop
wails build
```

## 最近变更

- 2026-05-24: 删除旧客户端域，重建 `aim-desktop-domain`；Desktop 成为唯一客户端实现文档入口。
- 2026-05-24: Desktop 支持同设备多账号：`config.json` 使用 `accounts[]` 保存账号级 `device_id`/Token，SQLite 缓存迁移到 `accounts/{user_id}/cache.db`，切换账号时重置 WS 与本地缓存句柄。
