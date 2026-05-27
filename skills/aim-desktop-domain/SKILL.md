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
- 桌面端聊天附件选择、SeaweedFS 直传、附件消息发送与附件卡片渲染。

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
| `app/desktop/internal/api` | Gateway REST client 与响应 DTO；包含附件 init/complete/download client（`attachments.go`）。 |
| `app/desktop/internal/ws` | Gateway WebSocket Protobuf client、心跳、ACK、发送消息/输入中/已读。 |
| `app/desktop/internal/store` | `config.json` 与账号级 SQLite 缓存。 |
| `app/desktop/frontend/src/App.vue` | Vue 主界面：登录/注册、多账号、会话、好友、群管理、消息。 |
| `app/desktop/frontend/wailsjs` | Wails 生成绑定；Go 绑定变更后用 `wails generate module` 重建。 |
| `app/desktop/docker/gui` | Desktop 图形化 Docker 运行环境：Debian Slim + Xvfb + Openbox + x11vnc + noVNC。 |
| `docker-compose.desktop-gui.yaml` | 通过浏览器/VNC 访问 Linux Desktop 容器的 Compose 覆盖文件。 |
| `app/desktop/PLAN-desktop-missing-features.md` | Desktop 功能缺口规划（临时文档，实施后清理）。 |

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

# Docker 图形化运行（浏览器访问 http://127.0.0.1:6080/vnc.html）
cd ../..
docker compose -f docker-compose.yaml -f docker-compose.desktop-gui.yaml up --build aim-desktop-gui
```

## 最近变更

- 2026-05-27: Desktop 附件选择器支持任意普通文件；非 `image`/`video`/`audio` MIME 自动按 `file` 附件发送，普通文件只展示通用附件卡片与下载/打开入口，不进入 data_parsing。
- 2026-05-25: Desktop 附件卡片支持图片/视频缩略图渲染，点击附件通过独立预览窗口展示原始媒体，并新增 `GetAttachmentDownload` Wails 绑定获取 Gateway 授权下载 URL。
- 2026-05-25: Desktop 附件卡片新增按 `file_id` 查询 Gateway `/api/attachments/{id}` 的当前附件状态缓存/轮询，用权威附件状态覆盖消息内容中的发送时快照，避免解析完成后仍显示 `pending`。
- 2026-05-25: `aim-desktop-domain` skill 同步更新：合并删除了的 `app/desktop/*/README.md` 内容，附件链路文档由 HTTP 代理更新为 `AttachmentService` gRPC。
- 2026-05-25: Desktop 新增聊天附件发送入口：通过 Wails 文件选择器选取图片/视频/音频/普通文件，调用 Gateway `/api/attachments`（gateway 侧以 gRPC 转发 `attachment.rpc`）完成上传初始化、SeaweedFS 直传、完成确认，再发送 `aim.attachment.v1` 附件消息；前端新增附件卡片基础渲染。
- 2026-05-24: 新增 Desktop 图形化 Docker 运行环境，使用 Debian Slim、Xvfb、Openbox、x11vnc、noVNC 启动 Linux 版 Wails Desktop，可通过 `docker-compose.desktop-gui.yaml` 在浏览器访问。
- 2026-05-24: 修复 Desktop WS 状态竞态：前端启动时先注册 Wails 运行时事件再 AutoLogin，Go 侧仅允许当前活跃 WS client 发出连接状态事件，避免旧连接断开覆盖新连接；WS client 断连回调改为单次触发，`SendTyping`/`SendReadReceipt` 在连接对象存在但已断开时也会重连。
- 2026-05-24: Desktop 支持同设备多账号：`config.json` 使用 `accounts[]` 保存账号级 `device_id`/Token，SQLite 缓存迁移到 `accounts/{user_id}/cache.db`，切换账号时重置 WS 与本地缓存句柄。
