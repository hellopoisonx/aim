# 协议规则

## 概览

`shared/proto` 定义 AIM 的跨端/跨服务线缆协议。这里不是业务实现目录，只放 `.proto` 和生成的 Go 代码；生成文件不要手改。

## 结构

```text
shared/proto/
├── ws/ws.proto              # WebSocket 二进制帧协议，gateway 与 frontend 共用
└── gateway/gateway.proto    # Core 调 GatewayService 的推送/踢下线/drain RPC
```

## 协议规则

### 字段约定

- `PushPresenceReq` 字段号：
  - `1` = `user_id`（状态变更者）
  - `2` = `status`（online/offline）
  - `3` = `updated_at`
  - `4` = `target_user_id`（接收推送的目标用户；未设置时网关回退用 `user_id` 寻址）
- `PushTypingReq` 字段号：
  - `1` = `target_user_id`
  - `2` = `from_user_id`
  - `3` = `conversation_id`
  - `4` = `timestamp`

### 向后兼容规则

- 服务端 `PushPresence` 收到 `target_user_id == 0` 时，自动回退到 `user_id` 寻址连接，保证旧版 caller（未设置 `target_user_id` 的 core 部署）行为不变。

## 一般规则

- WebSocket 只使用 Protobuf binary frame；不要新增 JSON/text frame 协议。
- `WsFrame.seq` 用于请求/ACK 关联；客户端发消息必须带稳定 `client_msg_id` 以支持重试和幂等。
- `FrameType` 编号保持区间语义：客户端到网关使用低编号，网关到客户端推送/ACK 使用高编号；新增值只追加，不复用旧编号。
- Proto 字段号一旦发布不得改含义；废弃字段保留编号和注释，不删除后复用。
- `gateway.proto` 的 `GatewayService` 由 gateway 实现，core 调用；不要让 logic 反向依赖 core/gateway 内部包。

## 生成

```bash
# WS/Gateway 共享 proto：在仓库根执行
protoc --go_out=. shared/proto/ws/ws.proto
protoc --go_out=. --go-grpc_out=. shared/proto/gateway/gateway.proto
```

若生成路径出现 `shared/proto/gateway` 与 `shared/proto/gateway/pb` 双份输出，先检查 `go_package`，不要手工改生成文件内容来修。

## 修改检查

```bash
go test ./app/gateway/api/internal/ws/... ./app/frontend/... ./app/core/...
go build ./...
```