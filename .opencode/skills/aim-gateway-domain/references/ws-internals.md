## 概览

本目录是 gateway 的 WebSocket 核心包，负责连接生命周期、身份上下文、Protobuf 帧编解码、GatewayService 推送、踢下线与 drain。它是有状态热路径，改动必须优先考虑并发与连接清理。

## 结构

| 文件 | 责任 |
| --- | --- |
| `manager.go` | `Identity{user_id,device_id}` 到 `Connection` 的并发安全映射 |
| `frame.go` | `WsFrame` 与各 payload 的 encode/decode/build |
| `gateway_server.go` | `PushMessage`、`PushPresence`、`PushFriendApplication`、`KickUser`、`DrainNotify` |
| `context.go` | 在 request context 中传递 WS identity |
| `auth/token.go` | Bearer token 提取和 JWT 验签 |

## 本地规则

- `Manager` 内部 map 只在锁内访问；不要把 map 或可变内部状态暴露给调用方。
- 每个 `user_id + device_id` 只能有一个活跃连接；重复注册必须显式失败或先踢旧连接，不能静默覆盖。
- 写 WebSocket 帧必须使用带超时的 context，避免慢连接阻塞推送链路。
- `DrainNotify` 流程：广播 `FRAME_TYPE_RECONNECT`，等待 `drain_timeout_ms`，再关闭连接；不要立即断开造成惊群重连。
- `KickUser` 的空 `device_id` 表示踢该用户所有设备；非空只踢指定设备。
- 新增帧类型时同步修改 `shared/proto/ws/ws.proto`、payload decode switch、handler/ack 映射和测试。
- WS 握手只接受 `Authorization: Bearer <token>`；不要支持 query token。

## 测试

```bash
go test ./app/gateway/api/internal/ws/...
go test -run 'Test.*WebSocket|Test.*Gateway|Test.*Manager' ./app/gateway/api/...
```

并发、timer、goroutine 相关修改必须保留 `goleak` 覆盖；新增 manager 行为优先加同包白盒测试。

## PushPresence

向目标用户推送在线状态变更（`FRAME_TYPE_PUSH_PRESENCE`）。

输入 `PushPresenceReq`：
- `user_id`：状态变更的用户 ID（用于 payload）
- `target_user_id`：接收推送的目标用户 ID（用于寻址连接；`== 0` 时回退到 `user_id`）
- `status`：online/offline
- `updated_at`：状态变更时间戳

关键逻辑：
1. 用 `target_user_id`（或回退的 `user_id`）通过 `manager.ForEachUser` 遍历该用户在本节点上的所有连接
2. 构造 `PushPresencePayload` 写入每个连接的 WebSocket 帧（`user_id` 仍是状态变更者，不是目标用户）

兼容性：旧版 caller 未设置 `target_user_id`（值为 0）时自动回退，保证升级期间行为不变。

修复记录：2026-05-22 修复前错误地使用 `req.UserId`（状态变更者）查找连接，导致推送不达。