# AIM 部署说明

`deploy/` 将 Docker Compose、环境变量、部署配置、反向代理与初始化脚本从根目录拆开，避免本地开发、生产、安全配置、观测与工具混在同一个 Compose 文件中。

## 目录

```text
deploy/
├── compose/
│   ├── base.yaml          # 核心服务 + 必需基础设施，不发布宿主机端口
│   ├── dev.yaml           # 本地开发端口映射，全部绑定 127.0.0.1
│   ├── prod.yaml          # 生产反向代理，只发布 80/443
│   ├── observability.yaml # Prometheus / Loki / Promtail / Grafana
│   └── tools.yaml         # Kafbat UI 等调试工具
├── config/
│   ├── local/             # 本地 Docker Compose 配置
│   ├── prod.example/      # 生产配置模板，真实生产文件不要提交
│   └── bench/             # 压测环境配置副本
├── env/
│   ├── local.env
│   └── prod.example.env
├── proxy/
│   ├── Caddyfile
│   └── nginx/aim.conf
└── scripts/
    ├── migrate-postgres.sh
    ├── init-kafka-topics.sh
    └── init-seaweed-bucket.sh
```

## 本地开发

启动核心服务与基础设施：

```bash
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --build
```

启动可观测组件：

```bash
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  -f deploy/compose/observability.yaml \
  up -d
```

启动 Kafbat UI 等工具：

```bash
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  -f deploy/compose/tools.yaml \
  --profile tools \
  up -d
```

本地发布端口均绑定 `127.0.0.1`，避免在远程开发机上误暴露数据库、Redis、Kafka、etcd 或管理 UI。

> 注意：`AIM_CONFIG_DIR` / `AIM_ENV_FILE` 如果使用相对路径，会按 Compose 文件所在目录解析。本仓库的 `deploy/env/local.env` 已使用 `../config/local` 与 `../env/local.env`，建议直接通过 `--env-file deploy/env/local.env` 使用；生产环境建议使用绝对路径。

## 生产部署

1. 在服务器准备真实配置和 env：

```bash
sudo mkdir -p /etc/aim/config
sudo cp -r deploy/config/prod.example/* /etc/aim/config/
sudo cp deploy/env/prod.example.env /etc/aim/aim.env
# 编辑 /etc/aim/config/*.yaml、/etc/aim/config/seaweed-s3.json、/etc/aim/aim.env
# 替换 CHANGE_ME / example.com，并配置真实域名、强随机密钥和生产密码。
```

2. 启动生产栈：

```bash
docker compose --env-file /etc/aim/aim.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/prod.yaml \
  up -d --build
```

生产默认只发布 Caddy 的 `80/443`；PostgreSQL、Redis、Kafka、etcd、gateway gRPC 等仅在 Docker 内部网络访问。

### 附件文件域名

`deploy/proxy/Caddyfile` 默认包含：

- `AIM_API_HOST`：反代到 `aim-gateway:8888`，提供 REST / WebSocket。
- `AIM_FILES_HOST`：反代到 `seaweed-s3:8333`，用于 attachment 服务返回给客户端的 S3 预签名上传/下载 URL。

如果生产不希望直接暴露内置 SeaweedFS，请删除或替换 Caddyfile 中的 files 站点，并将 `/etc/aim/config/attachment.yaml` 的 `Seaweed.PublicEndpoint` 改为你的外部对象存储 / CDN 域名。无论使用哪种方式，都必须确保：

```yaml
Seaweed:
  PublicEndpoint: https://<AIM_FILES_HOST 或外部对象存储域名>
```

与客户端实际可访问的文件域名一致，并先确认 SeaweedFS/S3 鉴权、网络策略和密钥强度满足生产要求。

## 初始化与迁移

- auth / logic 的 PostgreSQL 迁移由 `deploy/scripts/migrate-postgres.sh` 执行，按 `NNN_*.sql` 字典序自动执行目录内所有 SQL，避免在 Compose command 中硬编码迁移文件列表。attachment 迁移仍保留现有服务启动期逻辑，后续可继续迁入统一 migration runner。
- Kafka topic 由 `deploy/scripts/init-kafka-topics.sh` 创建，使用 `--if-not-exists` 保证幂等。
- SeaweedFS bucket 由 `deploy/scripts/init-seaweed-bucket.sh` 创建。

`app/*/etc/*.yaml` 仍保留为本地 `go run` / 单服务调试默认配置；Docker Compose 部署统一挂载 `deploy/config/<env>/*.yaml`。

## 兼容入口

根目录 `docker-compose.yaml` 现在只是本地开发兼容入口，include：

- `deploy/compose/base.yaml`
- `deploy/compose/dev.yaml`
- `deploy/compose/observability.yaml`
- `deploy/compose/tools.yaml`

旧命令如 `docker compose up -d postgres redis kafka etcd tempo grafana` 仍可作为兼容入口启动部分本地依赖；完整服务、迁移与新增部署请直接使用上面的分层 Compose 命令。
