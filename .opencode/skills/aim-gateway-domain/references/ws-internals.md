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