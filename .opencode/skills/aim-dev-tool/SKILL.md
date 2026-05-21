---
name: aim-dev-tool
description: AIM 开发测试工具（dev-tool）。Python CLI，覆盖 gateway 全部 REST 端点与 WebSocket 帧协议，支持多 profile 多用户并行测试。当需要调试 REST/WS 接口、运行集成测试、或新增命令时使用。
---
# aim-dev-tool

`dev-tool/aim_test.py` — AIM 的后端接口交互式测试工具。通过 gateway HTTP/WS 与后端服务通信，
无需前端客户端即可完成注册、登录、好友、会话、消息的全流程验证。

## 使用顺序

- 先看 `references/setup.md`，确认 Python 环境和依赖就绪。
- 再看 `references/commands.md`，了解所有可用命令和交互模式。
- 改命令或新增端点时：改 `dev-tool/aim_test.py`，然后更新本 Skill 的 `references/commands.md`。

## 适用场景

| 场景 | 操作 |
|------|------|
| 冒烟测试 | `python aim_test.py run-all` |
| 单端点调试 | `python aim_test.py <command> --<args>` |
| 多用户流程 | `--profile alice` / `--profile bob` |
| 交互探索 | `python aim_test.py interactive` |
| 新增 REST 端点 | 在 `RESTClient` 加方法 → 加 CLI handler → 加 parser → 更新 help/interactive/run-all → 更新本文档 |

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

gateway 启动后常见日志：`nacos initial SelectInstances for "9848": instance list is empty!`
这是 resolver 在 Nacos 服务注册完成前的非致命错误，**不影响主链路通信**。
判定方法：`curl http://127.0.0.1:8888/api/auth/register` 返回 405（非连接拒绝）即 gateway HTTP 正常。
详见 `aim-shared-domain` 中 Nacos resolver 规则。

### 接口层测试编译失败（fake/querier 不满足接口）

当 `model.Querier` 新增方法后，使用 `fakeQuerier` 的测试文件需同步补充对应空实现。
例如新增 `ListFriends` → 在 `database_permission_checker_test.go` 加：

```go
func (f *fakeQuerier) ListFriends(ctx context.Context, userID int64) ([]model.Friendship, error) {
    return nil, nil
}
```

## 参考资料

- `references/setup.md`
- `references/commands.md`
