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
docker compose build --no-cache
docker compose up -d --force-recreate aim-auth aim-core aim-gateway aim-logic

# 增量重建（仅代码变更时，利用缓存更快）
docker compose build
docker compose up -d --force-recreate aim-auth aim-core aim-gateway aim-logic
```

> 常见触发场景：新增 API 路由、proto 变更、配置结构体变更、业务逻辑修改。
