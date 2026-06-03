# AIM 部署说明

`deploy/` 将 Docker Compose、环境变量、部署配置、初始化脚本与部署自动化从根目录拆开，避免本地开发、生产、安全配置、观测与工具混在同一个 Compose 文件中。AIM 容器不再内置反向代理 / TLS 终止，由宿主或集群侧（nginx / Caddy / 云 LB / k8s ingress）负责。

## 目录

```text
deploy/
├── deploy.sh           # 生产部署自动化（preflight / init / migrate / up / down / rollback 等）
├── compose/
│   ├── base.yaml          # 核心服务 + 必需基础设施，不发布宿主机端口
│   ├── dev.yaml           # 本地开发端口映射，全部绑定 127.0.0.1
│   ├── prod.yaml          # 生产覆盖钩子（默认空，宿主或集群侧反代负责 80/443）
│   ├── observability.yaml # Prometheus / Loki / Promtail / Grafana
│   └── tools.yaml         # Kafbat UI 等调试工具
├── config/
│   ├── local/             # 本地 Docker Compose 配置
│   ├── prod.example/      # 生产配置模板，真实生产文件不要提交
│   └── bench/             # 压测环境配置副本
├── env/
│   ├── local.env
│   └── prod.example.env
├── scripts/
│   ├── migrate-postgres.sh
│   ├── init-kafka-topics.sh
│   └── init-seaweed-bucket.sh
├── grafana/  loki/  prometheus/  promtail/  tempo/   # 可观测性组件配置
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

AIM 容器只暴露内部端口（`expose:`），不直接对外。生产环境的 TLS 终止、域名分发、限流与静态缓存由宿主或集群侧的反向代理（nginx / Caddy / 云 LB / k8s ingress 等）负责。

### 1. 准备服务器目录结构

```bash
sudo mkdir -p /etc/aim/config
sudo cp -r deploy/config/prod.example/* /etc/aim/config/
sudo cp deploy/env/prod.example.env /etc/aim/aim.env
# 编辑 /etc/aim/config/*.yaml、/etc/aim/config/seaweed-s3.json、/etc/aim/aim.env
# 替换所有 CHANGE_ME / example.com，配置真实域名、强随机密钥与生产密码。
```

仓库自带 `deploy.sh` 会按这套约定校验目录与文件：

```bash
deploy/deploy.sh preflight   # 仅校验，不修改任何东西
deploy/deploy.sh init        # 第一次部署时使用：不会覆盖已有 /etc/aim 内容
```

### 2. 附件文件域名对齐

`/etc/aim/config/attachment.yaml` 中的 `Seaweed.PublicEndpoint` 决定客户端拿到附件上传/下载 URL 时访问的域名。该值必须与客户端实际可访问的域名一致，否则会拿到错误 host 签名的预签名 URL。

```yaml
Seaweed:
  PublicEndpoint: https://files.your-domain.com   # 与外部反代里给 seaweed-s3:8333 暴露的域名一致
```

如果使用外部对象存储 / CDN，请把 `Seaweed.Endpoint` 也切到对应端点，并保证：

- 内部 `expose: 8333` 仍可被 aim-attachment 访问（如果继续走内置 SeaweedFS）。
- 凭据 / 桶名 / region 与外部存储匹配，且 `Seaweed.SecretKey` 强度满足生产要求。
- attachment 服务的 gRPC 端口 `8091` 不被反代或 ingress 暴露在公网。

### 3. 启动 / 停止 / 回滚

```bash
deploy/deploy.sh migrate     # 显式跑一次 auth / logic 的 PostgreSQL 迁移 + Kafka topic + SeaweedFS bucket
deploy/deploy.sh up          # docker compose up -d --build（自动包含 migrate）
deploy/deploy.sh status      # 容器与 healthcheck 状态
deploy/deploy.sh logs aim-gateway --tail 200
deploy/deploy.sh down
deploy/deploy.sh rollback    # 回滚到上一次 deploy 前的 /etc/aim 快照（详见脚本说明）
```

`up` / `down` / `rollback` 都会先在 `/var/backups/aim/YYYYmmdd-HHMMSS/` 留一份当前 `/etc/aim` 快照，`rollback [id]` 可按时间戳回退到任一历史快照。生产默认只暴露宿主或集群侧反代的 80/443；PostgreSQL、Redis、Kafka、etcd、gateway gRPC 等只在 Docker 内部网络访问。

### 4. 自接反向代理

外部反代至少需要把以下两条流量转到 `aim-gateway:8888` 与 `seaweed-s3:8333`（容器端口以 Compose 为准）：

- 客户端 REST + WebSocket：建议路径 `/`（带 `Upgrade` / `Connection: upgrade` 头透传，长连接 idle 超时建议 ≥ 60s）。
- 附件上传 / 下载：宿主机或 ingress 监听 `https://files.your-domain.com` 转 `seaweed-s3:8333`，保留 `Host` 头或显式改写为内部 S3 端点。

> 注意：`AIM_CONFIG_DIR` / `AIM_ENV_FILE` 如果使用相对路径，会按 Compose 文件所在目录解析。本仓库的 `deploy/env/local.env` 已使用 `../config/local` 与 `../env/local.env`，建议直接通过 `--env-file deploy/env/local.env` 使用；生产环境请使用绝对路径（例如 `/etc/aim/aim.env`）。

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
