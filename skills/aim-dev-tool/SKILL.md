---
name: aim-dev-tool
description: AIM 开发测试工具（dev-tool）。Python CLI，覆盖 gateway 全部 REST 端点与 WebSocket 帧协议，支持多 profile 多用户并行测试、并发压测。当需要调试 REST/WS 接口、运行集成测试、压力测试、或新增命令时使用。
---
# aim-dev-tool

## 工具概览

| 工具 | 文件 | 用途 |
|------|------|------|
| 功能测试 | `dev-tool/aim_test.py` | 单端点调试、冒烟测试、交互探索 |
| 压力测试 | `dev-tool/benchmark.py` | 并发压测、QPS/延迟分布、容量评估 |

两个工具共享 `RESTClient` / `WSClient` / `TokenManager` 和 Protobuf 编解码层。

## 使用顺序

- 先看 `references/setup.md`，确认 Python 环境和依赖就绪。
- 再看 `references/commands.md`，了解所有可用命令和交互模式。
- 改命令或新增端点时：改 `dev-tool/aim_test.py`，然后更新本 Skill 的 `references/commands.md`。
- 压测时：`python benchmark.py <scenario>`，详见下方 "压力测试" 段落。

## 压力测试

`dev-tool/benchmark.py` — 并发压测工具，复用 `aim_test.py` 的 REST/WS 客户端。

### 压测架构

```
dev-tool/benchmark.py
├── MetricsCollector    → 线程安全的指标采集（延迟、错误、状态码）
├── RateLimiter         → 令牌桶速率控制 + 渐进加压
├── LoadGenerator       → ThreadPoolExecutor 并发引擎
├── ReportPrinter       → 实时进度 + ASCII 延迟直方图
├── RegisterScenario    → 批量注册压测
├── LoginScenario       → 批量登录压测
├── FriendChainScenario → 好友链压测（注册→登录→加好友→接受）
├── WsMessageScenario   → WS 消息并发压测（端到端延迟：A→服务器→B）
├── MixedScenario       → REST + WS 混合负载
└── CLI (argparse)      → 子命令模式
```

### 压测场景

| 命令 | 说明 | 示例 |
|------|------|------|
| `register` | 批量注册用户 | `python benchmark.py register --users 100 --rps 50` |
| `login` | 批量登录 | `python benchmark.py login --users 100` |
| `friend-chain` | 好友链（注册→登录→加好友→接受） | `python benchmark.py friend-chain --users 50` |
| `ws-message` | WS 消息并发发送 | `python benchmark.py ws-message --users 20 --messages-per-user 100` |
| `presence` | 好友在线状态查询压测 | `python benchmark.py presence --users 50 --rps 100` |
| `mixed` | REST + WS 混合负载 | `python benchmark.py mixed --users 100 --duration 30 --rps 200` |

### 通用参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--users N` | 100 | 并发用户数 |
| `--rps N` | 0 (不限) | 目标每秒请求数 |
| `--duration N` | 0 (跑完即止) | 持续时间（秒） |
| `--ramp-up N` | 0 | 渐进加压时间（秒） |
| `--output PATH` | — | 输出 JSON 报告路径 |
| `--quiet` | — | 静默模式（屏蔽 WS 帧日志） |

### 指标说明

压测输出包含以下指标：

| 指标 | 说明 |
|------|------|
| **Total Requests** | 总请求数 |
| **Success / Errors** | 成功/失败数及比例 |
| **Avg QPS** | 平均每秒请求数 |
| **Min / Avg / Max** | 延迟最小值/均值/最大值 |
| **P50 / P90 / P95 / P99** | 延迟百分位分布 |
| **Error Types** | 错误类型细分（`api_40400`、`recv_timeout`、`ws_disconnected`、`orphaned`、`ConnectionError` 等） |
| **Latency Histogram** | ASCII 延迟分布直方图 |

> **ws-message 延迟语义**：端到端延迟，A 发送消息 → 服务器 → B 收到 PUSH_MESSAGE。通过 `client_msg_id` 关联发送与接收，超时 30 秒未收到推送则记录 `recv_timeout` 错误。
>
> **WS 断线重连**：`WSClient` 内置自动心跳（默认 30s 间隔）和自动重连（默认最多 3 次，指数退避）。压测 `send_one` 捕获 `RuntimeError("Not connected")` 后尝试重连并重试发送（最多 3 次）；重连仍失败则记录 `ws_disconnected` 错误。

### 典型用法

```bash
cd dev-tool

# 快速验证：小规模注册
python benchmark.py register --users 10

# 容量评估：500 用户注册 + 50 RPS 限速 + 10s 渐进加压
python benchmark.py register --users 500 --rps 50 --ramp-up 10

# WS 消息压测：50 用户每用户发 500 条消息
python benchmark.py ws-message --users 50 --messages-per-user 500 --quiet

# 混合负载：100 用户持续 60 秒，限速 200 RPS，输出 JSON 报告
python benchmark.py mixed --users 100 --duration 60 --rps 200 --output report.json
```

### 输入/输出格式

**JSON 报告格式** (`--output`):

```json
{
  "title": "mixed",
  "duration_s": 60.0,
  "total_requests": 12000,
  "success": 11950,
  "errors": 50,
  "error_rate": 0.004,
  "avg_qps": 200.0,
  "latency_ms": {
    "min": 0.5, "avg": 5.2, "max": 150.3,
    "p50": 3.1, "p90": 8.5, "p95": 12.2, "p99": 25.8
  },
  "error_types": {
    "api_50000": 45,
    "ConnectionError": 5
  }
}
```

## 架构

```
dev-tool/aim_test.py
├── RESTClient     → gateway HTTP API（POST/GET + Bearer token）
├── WSClient       → gateway WebSocket（Protobuf 帧 encode/decode）
├── TokenManager   → 多 profile 的 access/refresh token 持久化
├── CLI (argparse) → 子命令模式
└── Interactive    → REPL 交互模式
```

依赖：
- `ws_pb2.py` / `gateway_pb2.py`：编译自 `shared/proto/ws/ws.proto` / `shared/proto/gateway/gateway.proto`
- `requests`, `websocket-client`, `protobuf`

## 约定

- 压测环境与本地开发环境通过端口 +10000 偏移隔离，互不影响；压测环境服务名统一 `bench-` 前缀。
- Gateway 容器需设置 `AIM_GATEWAY_NODE_ID` 环境变量（Snowflake 机器 ID），主开发环境为 `0`，压测环境也为 `0`（因两者不会同时运行于同一数据库）。
- Fixture 数据（`user.json`、`msg.txt`）由 `generate_fixtures.py` 生成，不要手动编辑；如需自定义数量用 `--count` 参数。

## 故障排查

### Windows 编码错误

`run-all` 使用 Unicode 字符（如 `✓`），Windows 终端默认 GBK 编码会报 `UnicodeEncodeError`。
所有 `python aim_test.py` 命令前加 `PYTHONIOENCODING=utf-8`：

```bash
PYTHONIOENCODING=utf-8 python aim_test.py run-all
```

### 注册后立即好友请求失败（`user not found`）

`run-all` 注册 Alice/Bob 后立即调 `add_friend` 可能返回 `[40400] user not found`。根因：

1. 注册 → auth 服务写入 auth 库 → Kafka `UserCreated` 事件
2. logic 服务消费 Kafka 事件 → 写入 logic 库用户信息表
3. `add_friend` 走 logic 查询用户 → 步骤 2 未完成则失败

延迟约 2-3 秒。`run-all` 已在注册完成后 `time.sleep(5)` 等待 Kafka 事件消费。
手动测试时需自行加延迟或重试。

### 接口 404（路由不存在）

如 `/api/friends/me` 返回 404 但 API 定义和 routes.go 中已存在路由，说明 Docker 容器运行的是旧镜像。
需重建并重启：

```bash
docker compose build --no-cache
docker compose up -d --force-recreate aim-auth aim-core aim-gateway aim-logic
```

### Gateway 日志大量 Nacos resolver 报错

如果 gateway/core 持续重复：`nacos initial SelectInstances for "9848": instance list is empty!`，说明运行中的镜像仍使用旧版 `nacos` resolver scheme，抢占了 Nacos SDK 内部 `nacos:9848` 直连目标。
需重建并重启相关服务；新版 AIM 自定义 resolver 使用 `aimnacos:///<service>`，真实启动期空实例日志应显示业务服务名（如 `auth.rpc`、`logic.rpc`），而不是 `9848`。
判定 gateway HTTP 是否正常：`curl http://127.0.0.1:8888/api/auth/register` 返回 405（非连接拒绝）即 gateway HTTP 正常。

### 接口层测试编译失败（fake/querier 不满足接口）

当 `model.Querier` 新增方法后，使用 `fakeQuerier` 的测试文件需同步补充对应空实现。
例如新增 `ListFriends` → 在 `database_permission_checker_test.go` 加：

```go
func (f *fakeQuerier) ListFriends(ctx context.Context, userID int64) ([]model.Friendship, error) {
    return nil, nil
}
```

## 文档结构

- `references/setup.md`：环境与配置（aim_test + benchmark 通用）
- `references/commands.md`：aim_test 命令参考 + benchmark 命令参考

### 压测环境（独立 Docker Compose）

`dev-tool/docker-compose.yaml` 提供独立于本地开发的压测环境，所有端口 +10000 偏移避免冲突：

| 服务 | 压测端口 | 开发端口 | 偏移 |
|------|---------|---------|------|
| gateway REST | 18888 | 8888 | +10000 |
| gateway gRPC | 19091 | 9091 | +10000 |
| auth gRPC | 18989 | 8989 | +10000 |
| core gRPC | 18081 | 8080 | +10000 |
| PostgreSQL | 15432 | 5432 | +10000 |
| Redis | 16379 | 6379 | +10000 |
| Kafka | 19092 | 9092 | +10000 |
| Nacos | 18848 | 8848 | +10000 |
| Tempo HTTP API | 13200 | 3200 | +10000 |

压测环境配置文件在 `dev-tool/etc/`，服务名均以 `bench-` 前缀，容器间通过 `bench-network` 通信。

启动压测环境：

```bash
cd dev-tool
docker compose up -d                    # 启动压测环境
python benchmark.py register --users 100
python benchmark.py ws-message --users 20 --messages-per-user 100
docker compose down -v                  # 停止并清理数据
```

压测时需指向压测端口：

```bash
export AIM_GATEWAY_HTTP=http://127.0.0.1:18888
export AIM_GATEWAY_WS=ws://127.0.0.1:18888/ws
```

### Fixture 生成

`dev-tool/generate_fixtures.py` 生成压测所需的测试数据：

```bash
cd dev-tool
python generate_fixtures.py                # 默认 1000 条
python generate_fixtures.py --count 5000   # 自定义数量
```

输出：
- `user.json`：批量注册用户数据（email, password, username）
- `msg.txt`：随机消息字符串（每行一条，混合中文/英文/数字）

## 最近变更

- 2026-05-30: 用户侧 Bot 管理命令。新增 `RESTClient` 16 个方法（`create_user_bot`/`list_user_bots`/`get_user_bot`/`update_user_bot`/`enable_user_bot`/`disable_user_bot`/`delete_user_bot`/`create_bot_token`/`list_bot_tokens`/`update_bot_token`/`rotate_bot_token`/`revoke_bot_token`/`add_bot_to_conversation`/`create_bot_direct_conversation`/`list_bot_actions`/`list_bot_events`）；CLI 新增 `user-bot-create`/`user-bot-list`/`user-bot-get`/`user-bot-update`/`user-bot-enable`/`user-bot-disable`/`user-bot-delete`/`user-bot-token-create`/`user-bot-token-list`/`user-bot-token-update`/`user-bot-token-rotate`/`user-bot-token-revoke`/`user-bot-add-conv`/`user-bot-direct-conv`/`bot-actions`/`bot-events` 子命令；交互模式新增 `bot-create`/`bot-list`/`bot-get`/`bot-update`/`bot-enable`/`bot-disable`/`bot-delete`/`bot-token-create`/`bot-token-list`/`bot-token-revoke`/`bot-token-rotate`/`bot-add-conv`/`bot-direct-conv`/`bot-actions`/`bot-events` 命令。
- 2026-05-30: `WSClient` 自动跟踪已连续处理的白名单 pending 推送 `seq`，自动心跳携带 `HeartbeatPayload.last_seq` 以触发服务端 L1 pending 补发；`ws-heartbeat` 新增 `--last-seq` 覆盖参数，交互模式支持 `ws-heartbeat N`。
- 2026-05-28: 压测配置统一 GatewayRpc 监听 `9091`（宿主机 `19091`），修复 `dev-tool/etc/core.yaml` 中重复 `Consumers` 与误缩进的 `Presence.TTLSeconds`；压测 Compose 迁移/Kafka topic 初始化改用 `deploy/scripts/`，避免遗漏新 migration 或 topic。
- 2026-05-28: 压测 compose 的 `bench-tempo` 镜像固定为 `grafana/tempo:2.8.1`，避免 `latest` 拉到 Tempo v3 RC 后无法解析仓库共用的 `deploy/tempo/tempo.yaml`；压测 `etc/logic.yaml`/`etc/core.yaml` 已启用 `aim.conversation.events` 群管理系统消息链路。
- 2026-05-24: 重新生成 `ws_pb2.py`/`gateway_pb2.py`，同步 `PUSH_READ_RECEIPT`、`PushReadReceiptPayload`、`PushMessagePayload.sender_info/is_system/mentions`、`GatewayService.PushReadReceipt` 等协议字段；`aim_test.py` 的帧名称表和解码表新增 `PUSH_READ_RECEIPT`，`ws-send` 支持 `--mentions`，会话创建未传 `name` 时自动生成默认名；`benchmark.py` 修复并发建会话索引竞争，并将非 ACCEPTED 的 `SERVER_ACK` 直接计入错误；压测 compose 的 tracing 后端已改为 Grafana Tempo，HTTP API 暴露在 `13200`；压测 `logic.yaml` 将 `TemporaryConversationMessageLimit` 改为 `-1`，避免 `0` 被服务上下文归一化为默认 10。
- 2026-05-23: 交互模式改用 `prompt_toolkit.patch_stdout` 包裹输入循环，后台 WS 推送/接收打印会显示在 prompt 上方并保留当前输入；新增 `presence-friends` REST 命令、`ws-read-receipt`/`ws-ack` WS 命令；`run-all` 覆盖好友在线状态接口；`benchmark.py` 新增 `presence` 场景，`mixed` REST 负载补充 presence 查询。
- 2026-05-22: 新增 `group-create` CLI 命令和交互命令，调用 `POST /api/conversations/group` 专用创建群聊端点。`RESTClient` 新增 `create_group()` 方法（支持 `name`/`avatar` 可选参数）。详见 `references/commands.md`。
- 2026-05-22: 新增群管理 REST 命令：`conv-members`（获取成员详情）、`conv-add-members`（添加成员）、`conv-remove-member`（移除成员）、`conv-leave`（退出群聊）、`conv-dismiss`（解散群聊）、`conv-update`（更新群信息）；`conv-create` 新增 `--name` 参数；`RESTClient` 新增 `_delete`/`_put` HTTP 方法和 6 个群管理方法；重新生成 `ws_pb2.py`/`gateway_pb2.py`（新增 `is_system` 字段）。
- 2026-05-22: `WSClient` 新增 `rest_client` 参数和 `_try_refresh_token()` 方法：`reconnect()` 重连前自动检测 Token 过期并通过 REST API 刷新。benchmark 数据流扩展：`user_creds`/`conv_pairs` 元组增加 `refresh_token`/`expires_at`/`device_id`，`WsMessageScenario` 和 `MixedScenario` 为每个 `WSClient` 注入对应的 `RESTClient`。压测配置 `dev-tool/etc/auth.yaml` 的 `AccessTTL` 从 `5m` 增大到 `30m`。详见 `aim-ws-token-management` skill。
- 2026-05-22: `WSClient` 新增自动心跳（默认 30s 间隔）、自动重连（默认最多 3 次，指数退避 1/2/4s）、`ensure_connected()` 主动重连方法。`disconnect()` 标记 `_intentional_close` 以区分主动/被动断线；`_on_close` 被动断线时自动触发重连。压测 `ws-message` 的 `send_one` 捕获 `RuntimeError("Not connected")` 后尝试重连重试（最多 3 次），重连仍失败记录 `ws_disconnected` 错误（替代原来笼统的 `RuntimeError`）。`mixed` 场景同步增加断线重连逻辑。
- 2026-05-22: `ws-message` 延迟语义改为端到端（A 发送 → 服务器 → B 收到 PUSH_MESSAGE）。发送方和接收方均建立 WS 连接，通过 `client_msg_id` 关联发送与接收，30s 超时未收到则记录 `recv_timeout` 错误。步骤从 4 步扩展为 5 步（Step 3: 接收方连接，Step 4: 发送方连接，Step 5: 发送消息+等待回执）。`WSClient.send_message()` 现在返回 `client_msg_id`。`LoadGenerator._worker_loop` 支持 `_SKIP_METRICS` 哨兵值，允许 task_fn 自行管理指标记录。
- 2026-05-22: 新增独立压测 Docker Compose 环境（`dev-tool/docker-compose.yaml`，端口 +10000 偏移）、压测专属配置（`dev-tool/etc/`）、Fixture 生成器（`dev-tool/generate_fixtures.py`）。主 `docker-compose.yaml` 为 gateway 添加 `AIM_GATEWAY_NODE_ID: 0` 环境变量。
- 2026-05-22: `--quiet` 彻底静默。现在会同时（1）设 `aim_test.VERBOSE=False` 拑制 WSClient 调试输出；（2）将 `websocket`/`urllib3`/`requests` logger 降为 CRITICAL；（3）为 setup-阶段 LoadGenerator 传 `verbose=False` 以去掉注册/创建会话阶段的进度条与中间汇总。CLI 所有场景（含 register/login/friend-chain）都支持 `--quiet`。
