# Bot SDK 集成测试 Docker 环境

该目录提供独立于根目录本地开发环境的 Docker Compose 环境，用于后续运行需要真实 AIM 服务栈的 Bot SDK 集成测试。

## 端口偏移

容器内部端口和服务发现地址保持与主环境一致；发布到宿主机的端口全部基于根 `docker-compose.yaml` 做 `+3000` 偏移，避免与本地开发环境冲突。

| 服务 | 主环境宿主端口 | 集成测试宿主端口 |
|---|---:|---:|
| gateway REST | 8888 | 11888 |
| gateway gRPC | 9090 | 12090 |
| auth RPC | 8989 | 11989 |
| core RPC | 8081 | 11081 |
| PostgreSQL | 5432 | 8432 |
| Redis | 6379 | 9379 |
| Kafka | 9092 | 12092 |
| Kafbat UI | 8000 | 11000 |
| SeaweedFS master | 9333 | 12333 |
| SeaweedFS volume | 8088 | 11088 |
| SeaweedFS filer | 8889 | 11889 |
| SeaweedFS S3 | 8333 | 11333 |
| Nacos HTTP | 8848 | 11848 |
| Nacos client gRPC | 9848 | 12848 |
| Nacos server gRPC | 9849 | 12849 |
| Jaeger UI | 16686 | 19686 |
| Jaeger OTLP gRPC | 4317 | 7317 |
| Jaeger OTLP HTTP | 4318 | 7318 |

## 使用方式

在仓库根目录执行：

```bash
docker compose -f bot_sdk/testdata/integration/docker-compose.yaml up -d
```

常用访问地址：

```bash
export AIM_BOTSDK_INTEGRATION_GATEWAY=http://127.0.0.1:11888
export AIM_BOTSDK_INTEGRATION_POSTGRES='postgres://user:password@127.0.0.1:8432/aim_logic?sslmode=disable'
export AIM_BOTSDK_INTEGRATION_REDIS=127.0.0.1:9379
export AIM_BOTSDK_INTEGRATION_KAFKA=127.0.0.1:12092
```

停止并清理数据：

```bash
docker compose -f bot_sdk/testdata/integration/docker-compose.yaml down -v
```

## 配置说明

`etc/` 下的 YAML 是从主环境配置复制出的集成测试专用配置。由于 Compose 内部服务名仍保持 `postgres`、`redis`、`kafka`、`nacos`、`aim-auth` 等，服务内部连接无需端口偏移。仅附件服务的 `Seaweed.PublicEndpoint` 改为宿主机可访问的 `http://localhost:11333`。
