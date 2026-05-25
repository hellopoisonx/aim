# AIM Docker 数据存储 pi 扩展

这是项目级 pi extension，自动发现路径：`.pi/extensions/aim-docker/index.ts`。

## 提供的工具

- `aim_docker_status`：检查 Docker Compose 中 Kafka、Redis、PostgreSQL 的运行与连接状态。
- `aim_kafka`：通过 `docker exec aim-kafka` 连接 Kafka，支持：
  - `list_topics`
  - `describe_topic`
  - `consume`
  - `list_groups`
  - `describe_group`
- `aim_redis`：通过 `docker exec aim-redis` 连接 Redis，支持：
  - `ping`
  - `info`
  - `dbsize`
  - `scan`
  - `type`
  - `ttl`
  - `get`
  - `set`
  - `del`
- `aim_pg`：通过 `docker exec aim-postgres` 连接 PostgreSQL，支持：
  - `list_databases`
  - `list_tables`
  - `describe_table`
  - `query`（默认只允许只读 SQL；显式 `readonly=false` 才允许写入本地开发数据）

所有工具输出按 pi 内置限制截断：最多 2000 行或 50KB；截断时会把完整输出保存到临时文件。

## 命令

- `/aim-docker-status`：在 pi TUI 底部 widget 显示 Kafka/Redis/PostgreSQL 连接状态。

## 使用前提

启动本地基础设施：

```bash
docker compose up -d postgres redis kafka
```

如果需要 Kafka 初始化 topic：

```bash
docker compose up kafka-init
```

## 快速验证

```bash
PI_OFFLINE=1 pi --no-session --no-skills --no-prompt-templates --tools aim_docker_status -p "调用 aim_docker_status 检查 kafka redis postgres"
```
