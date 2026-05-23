---
name: aim-tui-domain
description: aim 的 TUI 客户端域。对应 `app/tui` 模块。
---
# aim-tui-domain

## 使用范围

当需求涉及 `app/tui` 终端客户端、CLI 参数、Bubble Tea 布局、本地 SQLite 缓存、TUI WebSocket/REST 客户端、token 自动刷新、本机多实例隔离时使用本 Skill。

## 模块边界

- TUI 只和 gateway 通信：REST 走 `app/tui/internal/client`，WS 走 `app/tui/internal/wsclient`。
- TUI 不直接导入 auth/core/logic 的内部包。
- TUI 本地状态只作为客户端缓存，不作为服务端权威状态。

## 参考资料

- `references/runtime-design.md`
- `references/local-store.md`

## 最近变更

- 2026-05-23: TUI 改为两栏聊天布局；支持 `--email --password` 启动登录、后台 token 刷新、SQLite 保存 token/会话/消息/presence、本机多实例隔离（`--instance`/`--db`）。
