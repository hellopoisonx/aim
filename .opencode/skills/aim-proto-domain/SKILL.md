---
name: aim-proto-domain
description: aim 的 Protobuf 协议域。定义跨端/跨服务线缆协议。对应 `shared/proto` 目录。
---
# aim-proto-domain

## 如何使用

- 涉及协议规则与帧编号约定 -> `references/protocol-rules.md`
- 涉及 proto 代码生成 -> `references/protocol-rules.md`（生成命令在文末）

## 参考资料

- `references/protocol-rules.md`

## 最近变更

- 2026-05-22: `gateway.proto` 新增 `PushTyping` RPC（`PushTypingReq`/`PushTypingResp`），用于 core `TypingConsumer` 向目标用户网关推送输入状态；字段号 1-4 对应 target_user_id/from_user_id/conversation_id/timestamp。
- 2026-05-22: `PushPresenceReq` 新增 `target_user_id`（字段号 4），解决在线状态/输入中推送无法到达客户端的问题。`PushPresence` 兼容 `target_user_id == 0` 时回退到 `user_id` 寻址。参见 `references/protocol-rules.md` §向后兼容规则。
- 2026-05-20: 从 shared/proto/AGENTS.md 迁移