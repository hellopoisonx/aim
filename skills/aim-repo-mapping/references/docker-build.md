# Docker 构建约定

## BuildKit

- 服务镜像 Dockerfile 使用 BuildKit syntax directive：`# syntax=docker/dockerfile:1.7`。
- Go 构建阶段应声明 `GOCACHE=/root/.cache/go-build` 与 `GOMODCACHE=/go/pkg/mod`。
- `go mod download` 使用 `--mount=type=cache,target=/go/pkg/mod` 缓存模块下载。
- `go build` 同时挂载 `/go/pkg/mod` 与 `/root/.cache/go-build`，复用模块缓存和编译缓存。

## 本地启用

PowerShell:

```powershell
$env:DOCKER_BUILDKIT="1"
$env:COMPOSE_DOCKER_CLI_BUILD="1"
docker compose build
```

Docker Buildx/新版 Docker Desktop 通常默认启用 BuildKit；显式设置环境变量可避免旧环境退回 legacy builder。

## 何时必须重建

Go 源码变更后，Docker 容器运行的是旧二进制，必须重建镜像并重启服务：

```bash
# 全量重建（建议首次或依赖变更时）
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  build --no-cache
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --force-recreate aim-auth aim-core aim-gateway aim-logic

# 增量重建（仅代码变更时，利用缓存更快）
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  build
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --force-recreate aim-auth aim-core aim-gateway aim-logic
```

> 常见触发场景：新增 API 路由、proto 变更、配置结构体变更、业务逻辑修改。

## 配置文件变更

Docker Compose 中服务配置文件应通过 bind mount 注入容器，避免修改配置后必须重建镜像才能生效。分层部署统一从 `deploy/config/<env>/` 挂载，默认本地环境为：

- `deploy/config/local/auth.yaml -> /app/etc/auth.yaml`
- `deploy/config/local/gateway-api.yaml -> /app/etc/gateway-api.yaml`
- `deploy/config/local/core.yaml -> /app/etc/core.yaml`
- `deploy/config/local/logic.yaml -> /app/etc/logic.yaml`
- `deploy/config/local/attachment.yaml -> /app/etc/attachment.yaml`
- `deploy/config/local/data_parsing.yaml -> /app/etc/data_parsing.yaml`
- `deploy/config/local/seaweed-s3.json -> /etc/seaweedfs/s3.json`

`app/*/etc/*.yaml` 保留为本地 `go run` / 单服务调试默认配置，不再作为 Docker 部署配置源。

纯配置变更后通常只需：

```bash
docker compose --env-file deploy/env/local.env \
  -f deploy/compose/base.yaml \
  -f deploy/compose/dev.yaml \
  up -d --force-recreate <service>
```

无需 `docker compose build`，除非 Go 源码、Dockerfile、生成文件或依赖发生变化。
