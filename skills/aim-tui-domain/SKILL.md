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

- `references/gateway-api-reference.md` — Gateway REST + WebSocket 帧协议（TUI 对接必读）
- `references/runtime-design.md`
- `references/local-store.md`

## 最近变更

- 2026-05-23: 修复双 TUI 同环境卡死：跨进程 DB 文件锁、`events` 通道不再阻塞 WS、WS 状态更新统一走 `notifyUI` 主 goroutine；登录时生成唯一 `device_id`。
- 2026-05-23: 打通已读/输入中全链路：登录与 bootstrap 自动拉历史+`read_states`+`READ_RECEIPT`；`PUSH_TYPING`/`PUSH_READ_RECEIPT` 经 `notifyCh` 在主 goroutine 更新 UI；会话头展示 `已读: user→#msg`；己方消息展示 `✓ 已读`。
- 2026-05-23: TUI 对接 `gateway-api-reference.md`：`PUSH_READ_RECEIPT` 解码、推送自动 `CLIENT_ACK`、42900 限流 REJECTED 提示并移除乐观消息、`RECONNECT` 延迟重连、`PUSH_NOTIFICATION` 展示、历史/推送 `sender_info` 与系统消息渲染、`single`→`direct` 归一化。
- 2026-05-23: @ 提及全链路：TUI 输入 `@` 弹出成员选择并填充 `mentions`；历史/推送返回 `mentions`（`logic.MessageItem` / `PushMessagePayload` 字段号 9/11）。展示时对不可用昵称回退为用户 ID，避免把空白/带空格/含 `@` 的标签写入正文。
- 2026-05-23: 同步 `gateway-api-reference.md` 与后端 proto 实现对齐：CLIENT_ACK 已处理、登出 KickUser、Transfer 限流 42900、PUSH_NOTIFICATION 生产者、RECONNECT/drain、`conversation_type` 统一为 direct/group、mentions 字符串类型。
- 2026-05-23: 补全 TYPING/已读状态 UI；菜单增加创建群聊、退出登录；历史 `read_states` 解析与实时已读展示。
- 2026-05-23: 完成注册→登录→WS→会话/好友/发消息全链路 UI；认证页、session bootstrap、好友申请与加好友/建会话、WS 心跳；消息页三栏（菜单+会话+聊天）。
- 2026-05-23: 新增 `references/gateway-api-reference.md`，汇总当前全部 REST 端点与 WS 帧协议，供 TUI 实现参考。
- 2026-05-23: 支持 `--email --password` 启动登录、后台 token 刷新、SQLite 保存 token/会话/消息/presence、本机多实例隔离（`--instance`/`--db`）。
