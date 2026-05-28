# Dev Tool 环境与配置

## 前置条件

- Python 3.9+
- 已启动的 AIM 后端服务（`docker compose up -d`）
- gateway 可访问：默认 `http://127.0.0.1:8888`（REST）和 `ws://127.0.0.1:8888/ws`（WebSocket）

## 安装依赖

```bash
cd dev-tool
pip install -r requirements.txt
```

依赖列表（`requirements.txt`）：

```
protobuf>=7.34.1
websocket-client>=1.8
requests>=2.28
```

## Proto 编译

`ws_pb2.py` 和 `gateway_pb2.py` 是预编译的 protobuf Python 模块，
从以下 proto 文件生成：

| Python 模块 | Proto 源 | 编译命令 |
|------------|---------|---------|
| `ws_pb2.py` | `shared/proto/ws/ws.proto` | `protoc --python_out=dev-tool ws.proto` |
| `gateway_pb2.py` | `shared/proto/gateway/gateway.proto` | `protoc --python_out=dev-tool gateway.proto` |

当 proto 文件变更后需重新编译：

```bash
cd shared/proto/ws
protoc --python_out=../../../dev-tool ws.proto
cd ../gateway
protoc --python_out=../../../dev-tool gateway.proto
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AIM_GATEWAY_HTTP` | `http://127.0.0.1:8888` | gateway REST 地址 |
| `AIM_GATEWAY_WS` | `ws://127.0.0.1:8888/ws` | gateway WebSocket 地址 |

> 使用 `127.0.0.1` 而非 `localhost`，避免 Windows 上 IPv6 解析延迟。

## 状态文件

工具将 token 持久化到 `dev-tool/` 目录下的 JSON 文件中：

| 文件 | Profile |
|------|---------|
| `.aim_state.json` | 默认 (default) |
| `.aim_state_alice.json` | alice |
| `.aim_state_bob.json` | bob |

每个文件包含 `access_token`、`refresh_token`、`expires_at`、`user_id`、`device_id`。

## 目标服务端口

### 本地开发环境

| 服务 | 端口 | 协议 |
|------|------|------|
| aim-gateway | `8888` | HTTP REST |
| aim-gateway | `9091` | gRPC（内部） |
| aim-auth | `8989` | gRPC |
| aim-core | `8080` | gRPC |
| aim-logic | `8082` | gRPC |

> dev tool 只直接访问 gateway 的 `8888` 端口，所有后端通信由 gateway 代理。

### 压测环境（`dev-tool/docker-compose.yaml`）

| 服务 | 端口 | 协议 |
|------|------|------|
| bench-gateway | `18888` | HTTP REST |
| bench-gateway | `19091` | gRPC（内部） |
| bench-auth | `18989` | gRPC |
| bench-core | `18081` | gRPC |
| bench-logic | — | gRPC（内部） |
| bench-postgres | `15432` | PostgreSQL |
| bench-redis | `16379` | Redis |
| bench-kafka | `19092` | Kafka |
| bench-nacos | `18848` | Nacos HTTP |
| bench-tempo | `13200` | Tempo HTTP API（Grafana 数据源） |


> `bench-tempo` 镜像固定为 `grafana/tempo:2.8.1`，与仓库共用 `deploy/tempo/tempo.yaml` 兼容；不要使用 `latest`。

压测环境配置文件：`dev-tool/etc/{auth,core,gateway-api,logic}.yaml`

启动/停止：

```bash
cd dev-tool
docker compose up -d
# ... 压测 ...
docker compose down -v  # 停止并清理数据
```

## Windows 注意事项

Windows 终端默认使用 GBK 编码，`run-all` 输出的 Unicode 字符（`✓` 等）会导致 `UnicodeEncodeError`。
运行所有 dev-tool 命令时需设置 `PYTHONIOENCODING=utf-8`：

```bash
PYTHONIOENCODING=utf-8 python aim_test.py run-all
PYTHONIOENCODING=utf-8 python aim_test.py interactive
```

或在 PowerShell 中持久设置：

```powershell
[Environment]::SetEnvironmentVariable("PYTHONIOENCODING", "utf-8", "User")
```

## Docker 镜像同步

当后端 Go 代码变更后，Docker 容器运行的是旧镜像，需重建并重启：

```bash
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  build --no-cache
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --force-recreate aim-auth aim-core aim-gateway aim-logic
```

> 提示：dev-tool 测试网关心路由（如新增 `/api/friends/me`），若接口 404 但 API 定义和 routes.go 已注册，先检查 Docker 镜像是否最新。
