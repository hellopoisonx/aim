# Code Context

## 文件索引

### 核心文件
1. `app/gateway/api/gateway.api` — REST API 协议定义（22 个 endpoint）
2. `app/gateway/api/internal/handler/routes.go` — goctl 生成的路由注册（不编辑）
3. `app/gateway/api/internal/handler/ws/ws_handler.go` — WebSocket 帧分发处理
4. `app/gateway/api/internal/ws/gateway_server.go` — gRPC GatewayService 实现（推送帧出口）
5. `app/gateway/api/internal/ws/frame.go` — 帧编解码 + 所有 13 种帧类型的 DecodePayload 分支
6. `shared/proto/ws/ws.proto` — WebSocket 帧协议定义

### 测试/压测文件
7. `dev-tool/aim_test.py` — REST + WS 测试套件（28 个子命令 + `run-all` 集成测试）
8. `dev-tool/benchmark.py` — 压测工具（5 个场景：register/login/friend-chain/ws-message/mixed）
9. `dev-tool/docker-compose.yaml` — 独立压测环境（端口 +10000 偏移）
10. `dev-tool/run_ws_message_benchmark.ps1` — WS 消息压测快捷脚本

---

## REST API 端点总览（22 个）

| # | 方法 | 路径 | 中间件 | 测试覆盖 | 压测覆盖 |
|---|------|------|--------|---------|---------|
| 1 | POST | `/api/auth/register` | 无 | ✅ CLI + run-all | ✅ register 场景 |
| 2 | POST | `/api/auth/login` | 无 | ✅ CLI + run-all | ✅ login 场景 |
| 3 | POST | `/api/auth/refresh` | 无 | ✅ CLI + run-all | ❌ 未单独压测 |
| 4 | POST | `/api/auth/logout` | 无 | ✅ CLI + run-all | ❌ |
| 5 | GET | `/api/users/by-name/:name` | Auth | ✅ CLI + run-all | ❌ |
| 6 | GET | `/api/users/by-id/:id` | Auth | ✅ CLI | ❌ |
| 7 | POST | `/api/users/friends/:id` | Auth | ✅ CLI + run-all | ✅ friend-chain |
| 8 | POST | `/api/conversations` | Auth | ✅ CLI + run-all | ✅ ws-message/mixed |
| 9 | POST | `/api/conversations/group` | Auth | ✅ CLI | ❌ |
| 10 | GET | `/api/conversations/history/:id` | Auth | ✅ CLI + run-all | ✅ mixed (get_history) |
| 11 | GET | `/api/conversations` | Auth | ✅ CLI | ❌ |
| 12 | GET | `/api/conversations/:id/members` | Auth | ✅ CLI | ❌ |
| 13 | POST | `/api/conversations/:id/members` | Auth | ✅ CLI | ❌ |
| 14 | DELETE | `/api/conversations/:id/members/:uid` | Auth | ✅ CLI | ❌ |
| 15 | POST | `/api/conversations/:id/leave` | Auth | ✅ CLI | ❌ |
| 16 | DELETE | `/api/conversations/:id` | Auth | ✅ CLI | ❌ |
| 17 | PUT | `/api/conversations/:id` | Auth | ✅ CLI | ❌ |
| 18 | GET | `/api/friends/applications` | Auth | ✅ CLI + run-all | ❌ |
| 19 | GET | `/api/friends/me` | Auth | ✅ CLI + run-all | ❌ |
| 20 | POST | `/api/friends/accept/:id` | Auth | ✅ CLI + run-all | ✅ friend-chain |
| 21 | POST | `/api/friends/reject/:id` | Auth | ✅ CLI | ❌ |
| 22 | GET | `/api/presence/friends` | Auth | ❌ 完全未覆盖 | ❌ |

---

## WebSocket 帧协议覆盖

### 客户端 → 网关（5 种发送帧）

| 帧类型 | 协议定义 | 服务端处理 | aim_test.py 发送 | aim_test.py 接收解码 |
|--------|---------|-----------|-----------------|-------------------|
| FRAME_TYPE_SEND_MESSAGE (1) | ✅ | ✅ handleSendMessage → core.Transfer | ✅ send_message() | ✅ 解码 |
| FRAME_TYPE_HEARTBEAT (2) | ✅ | ✅ handleHeartbeat → SERVER_ACK | ✅ send_heartbeat() | ✅ 解码 |
| FRAME_TYPE_TYPING (3) | ✅ | ✅ handleTyping → Kafka | ✅ send_typing() | ✅ 解码 |
| FRAME_TYPE_READ_RECEIPT (4) | ✅ | ❌ **无 case 分支**（frame.go 可解码，handler switch 未处理） | ✅ send_read_receipt() 定义 | ✅ 解码 |
| FRAME_TYPE_ACK (5) | ✅ | ❌ **无 case 分支**（frame.go 可解码，handler switch 未处理） | ❌ **无发送方法** | ✅ 解码 |

### 网关 → 客户端（8 种推送帧）

| 帧类型 | 协议定义 | GatewayServer 实现 | aim_test.py 接收解码 |
|--------|---------|-------------------|-------------------|
| FRAME_TYPE_PUSH_MESSAGE (101) | ✅ | ✅ PushMessage | ✅ on_frame 回调（用于 e2e 延迟测量） |
| FRAME_TYPE_PUSH_PRESENCE (102) | ✅ | ✅ PushPresence | ✅ 解码 |
| FRAME_TYPE_PUSH_NOTIFICATION (103) | ✅ | ❌ **无推送方法** | ✅ 解码（无特定处理） |
| FRAME_TYPE_PUSH_TYPING (104) | ✅ | ✅ PushTyping | ✅ 解码 |
| FRAME_TYPE_RECONNECT (105) | ✅ | ✅ DrainNotify | ✅ 解码 |
| FRAME_TYPE_SERVER_ACK (106) | ✅ | ✅ heartbeat + sendMessage 响应 | ✅ 解码 |
| FRAME_TYPE_TOKEN_EXPIRED (107) | ✅ | ✅ sendTokenExpired | ✅ 解码 |
| FRAME_TYPE_PUSH_FRIEND_APPLICATION (108) | ✅ | ✅ PushFriendApplication | ✅ 解码 |

---

## 已覆盖 vs 缺口分析

### ✅ 已覆盖良好的链路
- **注册→登录→Token 生命周期**：CLI + `run-all` + benchmark（register/login 场景）全覆盖
- **好友请求→接受→列表**：CLI + `run-all` + benchmark（friend-chain 场景）全覆盖
- **创建对话→发送消息→接收推送**：CLI + `run-all` + benchmark（ws-message 场景）全覆盖（含 e2e 延迟测量）
- **Token 过期→TOKEN_EXPIRED 推送**：ws_handler 实现完整
- **心跳→SERVER_ACK**：实现 + 测试覆盖
- **Typing→Kafka 发布**：实现 + CLI 覆盖

### ⚠️ REST 未覆盖
1. **`GET /api/presence/friends`** — RESTClient 无方法，CLI 无命令，run-all 未调用，benchmark 未压测

### ⚠️ WS 帧实现缺口
1. **FRAME_TYPE_READ_RECEIPT（4）** — ws_handler.go handleFrame switch 缺少 case（frame.go可解码→无分发→无效操作）；但客户端 send_read_receipt() 已定义
2. **FRAME_TYPE_ACK（5，ClientAckPayload）** — 同上：可解码无分发；WSClient 无任何 ack 发送方法
3. **FRAME_TYPE_PUSH_NOTIFICATION（103）** — GatewayServer 无推送方法；proto 定义、编解码路径完整但无触发来源

### ⚠️ 集成测试缺口（run-all）
`run-all` 只覆盖了最基本的好友→对话→WS 消息路径。以下完全未测试：
- 群组创建/加人/踢人/离开/解散/更新信息
- 查看对话成员详情
- 拒绝好友请求
- Presence endpoint
- 翻页查询历史（cursor）
- 断线重连 (RECONNECT 帧)
- 已读回执流程

### ⚠️ 压测缺口（benchmark.py）
benchmark 已有 5 个场景，缺口包括：
- 群组操作压测（create_group → add_members → history）
- 已读回执 / typing 并发压测
- 断线重连压测
- Auth refresh/logout 压测
- 窗口翻页历史查询压测
- Presence 查询压测

---

## 架构要点

1. **Gateway 是唯一入口**：前端/客户端只连 gateway（REST + WS），gateway 通过 gRPC 调 core/auth/logic
2. **WS 消息流**：ws_handler.handleSendMessage → corepb.TransferReq → core.Transfer (gRPC) → core 处理消息+Kafka→ logic 消费→ core PushMessage (gRPC) → gateway_server.PushMessage → 对端 WS 连接
3. **Presence 流**：Manager.Register/Unregister/RecordHeartbeat ↔ Redis ↔ PresencePub (Kafka) ↔ 其他节点
4. **好友申请推送**：logic 端处理后 → PushFriendApplication (gRPC) → gateway_server.PushFriendApplication
5. **Read Receipt 尚未接入**：协议层面完整（proto + 编解码 + WSClient 发送方法），但 handler switch 无处理，logic/core 无消费逻辑

---

## 执行建议（Worker 可操作）

### 优先级 P0：堵住已知覆盖缺口

**任务 1：补充 `/api/presence/friends` 测试覆盖**
- 编辑 `dev-tool/aim_test.py`：RESTClient 增加 `get_friends_presence()` 方法，CLI 增加 `presence-friends` 命令，interactive 模式增加处理
- 验证：`python aim_test.py presence-friends`

**任务 2：补充 `run-all` 群组操作测试**
- 编辑 `dev-tool/aim_test.py` cmd_run_all：注册第三用户 charlie，创建群组对话，加人，查看成员，更新群名，查看历史

**任务 3：补充 benchmark 群组场景**
- 编辑 `dev-tool/benchmark.py`：新增 `GroupScenario`（register → login → create_group → add_members → list_members → history → leave），注册到 CLI

### 优先级 P1：WS 帧 handler 实现

**任务 4：实现 FRAME_TYPE_READ_RECEIPT 服务端处理**
- 文件：`app/gateway/api/internal/handler/ws/ws_handler.go`
- handleFrame switch 增加 `case pb.FrameType_FRAME_TYPE_READ_RECEIPT` → handleReadReceipt
- handleReadReceipt 发布到 Kafka（类似 handleTyping），topic 待确认

**任务 5：实现 FRAME_TYPE_ACK（ClientAckPayload）服务端处理**
- 文件：`app/gateway/api/internal/handler/ws/ws_handler.go`
- handleFrame switch 增加 `case pb.FrameType_FRAME_TYPE_ACK`
- 当前可简单地记录日志或更新 last_ack_seq

### 优先级 P2：测试/压测增强

**任务 6：aim_test.py 增加 ws-read-receipt CLI 命令**
- 文件：`dev-tool/aim_test.py`
- send_read_receipt 已定义，增加 CLI 子命令 + interactive 处理

**任务 7：benchmark.py 增加消息类型多样性**
- 在 WsMessageScenario 中交替发送 read_receipt 和 typing 帧（当前只发 SEND_MESSAGE）

### 修复建议

**建议 1**：`aim_test.py` send_read_receipt 的第二个参数是 `last_msg_id`（int），但 CLI 无对应命令；建议增加 `--last-msg-id` 参数。

**建议 2**：ws_handler.go handleFrame 的 `default` 分支（第 163 行）对未知帧静默忽略，建议至少打印 debug 日志以便调试。

**建议 3**：benchmark.py MixedScenario 中 REST 调用（第 1156-1170 行）混合了 list_friends / list_conversations / get_history，但缺少 presence 调用补充（P0 任务 1 完成后）。

---

## Start Here

**Worker 应先打开 `app/gateway/api/internal/handler/ws/ws_handler.go`**，查看 handleFrame switch（约第 150-165 行），这是 WS 帧处理的核心分发点，所有客户端帧类型在此路由。然后对照 `shared/proto/ws/ws.proto` 确认帧枚举值，最后编辑 `dev-tool/aim_test.py` 补齐测试。

## 快速验证命令

```bash
# 1. 验证当前 REST 服务器启动
cd app/gateway && go build ./...

# 2. 开发环境启动
cd dev-tool && docker compose -f docker-compose.yaml up -d

# 3. 运行 run-all 集成测试
cd dev-tool && python aim_test.py run-all

# 4. 压测 WS 消息（3000 用户，各 100 条）
cd dev-tool && python generate_fixtures.py --count 3000
cd dev-tool && python benchmark.py ws-message --gateway http://127.0.0.1:18888 --users 3000 --messages-per-user 100 --quiet --output result.json

# 5. 运行单一 API 测试
cd dev-tool && python aim_test.py friend-list
cd dev-tool && python aim_test.py conv-list
```