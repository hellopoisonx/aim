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

## 服务端 seq 分配

- `WsFrame.seq` 按每条 WebSocket 连接独立递增；`ClientAck.ack_seq` 也按连接推进 `LastAckedSeq`。
- 服务端业务代码可构造 `seq=0` 的帧作为“待分配”哨兵；实际写出前必须由 `Connection` 写队列补齐为正数。
- 每个 `Connection` 维护 bounded outbound channel 和单独 `writerLoop`；所有 GatewayService 推送、handler ACK、TOKEN_EXPIRED 等写入都通过该队列串行化。
- `writerLoop` 是同一连接唯一 WebSocket writer，按出队顺序执行“分配 seq → 编码 → 写 socket → 返回结果”，避免多个 goroutine 抢同一把锁，也保证写出顺序与 seq 顺序一致。
- 连接注销、context 取消或 token 过期关闭时必须停止 writer 并让后续写入返回错误，避免 goroutine 泄漏。

## 测试

```bash
go test ./app/gateway/api/internal/ws/...
go test -run 'Test.*WebSocket|Test.*Gateway|Test.*Manager' ./app/gateway/api/...
```

并发、timer、goroutine 相关修改必须保留 `goleak` 覆盖；新增 manager 行为优先加同包白盒测试。

## PushMessage

向目标用户推送聊天消息（`FRAME_TYPE_PUSH_MESSAGE`）。

输入 `PushMessageReq`：
- `target_user_id`：接收推送的目标用户 ID（用于寻址连接）
- `sender_id`：发送者用户 ID（用于判断是否为发送者多端同步）
- `source_device_id`：普通消息的原始发送设备 ID；仅用于过滤，不写入 WS payload
- 其他消息字段：构造 `PushMessagePayload` 后透传给客户端

关键逻辑：
1. 用 `target_user_id` 通过 `manager.GetByUserID` 获取该用户在本节点上的所有连接
2. 普通消息多端同步时，core 会将发送者用户也作为 target；若 `target_user_id == sender_id` 且连接 `device_id == source_device_id`，跳过原始发送设备，避免发送端收到 ACK 后又收到同一条 PUSH_MESSAGE
3. 发送者其他设备、其他会话成员设备继续接收 `FRAME_TYPE_PUSH_MESSAGE`


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

## 轻量 pending 帧分类

客户端或未来服务端 ACK 补发实现只应对白名单帧做短 TTL、有限容量 pending：

- `FRAME_TYPE_PUSH_MESSAGE`：按 `message_id` 去重；发送方可用 `client_msg_id` 合并本地 sending，最终以 history 为准。
- `FRAME_TYPE_PUSH_NOTIFICATION`：低容量短 TTL；有通知中心/配置快照时以 REST 快照为准。
- `FRAME_TYPE_PUSH_FRIEND_APPLICATION`：按好友双方与 `updated_at` 合并，最终刷新好友申请列表或好友列表。
- `FRAME_TYPE_PUSH_READ_RECEIPT`：按 `(conversation_id, user_id)` 合并最大 `last_read_message_id`，最终以 history 返回的 `read_states` / `read_details` 校准。

不要 pending：`FRAME_TYPE_PUSH_TYPING`、`FRAME_TYPE_PUSH_PRESENCE`、`FRAME_TYPE_RECONNECT`、`FRAME_TYPE_TOKEN_EXPIRED`、`FRAME_TYPE_SERVER_ACK`。其中 presence 重连后直接拉 `GET /api/presence/friends` 快照；连接控制帧和 ACK 不做重放。
