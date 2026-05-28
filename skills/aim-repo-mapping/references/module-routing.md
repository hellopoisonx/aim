# 模块路由

## 模块职责

- `auth`：登录态、`RefreshToken`、 `AccessToken` 、 刷新 tokens。
- `gateway`：面向客户端的http网关，负责维持管理 `ws` 连接并转发请求给下游grpc服务。
- `core`: 负责转发消息。
- `logic`：逻辑判断层。
- `attachment`：聊天附件服务，负责 SeaweedFS 落盘、附件元数据、下载授权。
- `data_parsing`：附件解析服务，负责媒体元数据提取、缩略图/派生对象生成。
- `dev-tool`：Python CLI 测试工具，覆盖 gateway REST/WS 接口，用于冒烟测试和接口调试。

## 外部接口边界

- 只有 `gateway` 能向外暴露 REST API 与 WebSocket：新增客户端 REST/WS 能力必须落到 `app/gateway/api`（先改 `gateway.api`），再由 gateway 调内部 gRPC/Kafka。
- `auth`、`core`、`logic`、`attachment`、`data_parsing` 等模块不得新增面向客户端/公网的 REST/WS 服务；服务间能力优先使用 gRPC；Compose 只能使用内部网络访问，不得 `ports` 映射到宿主机。

## 交接信号

- 需求同时涉及一个域的 API 和另一个域的配置，说明已经越过总路由层，应该切到对应领域 Skill。

## 基础设施 / 运维

- Kafka topic 创建、consumer group 诊断、#PARTITIONS=0 排查：参考 `references/kafka-ops.md`
- Docker 构建约定：参考 `references/docker-build.md`

## 可观测性

- **Prometheus**：每个 AIM 服务通过 go-zero 内置 agent 暴露 `/metrics` 端点，端口 9191-9196
- **Grafana**：预置仪表盘 `deploy/grafana/dashboards/`，包含 go-zero 运行时指标 + AIM 服务 KPI
- **Loki + Promtail**：通过 Docker socket 采集各容器 stdout JSON 日志，Grafana Loki datasource 可查询
- **Prometheus 配置**：`deploy/prometheus/prometheus.yml` 定义 scrape targets
- **Grafana Tempo**：OpenTelemetry 链路追踪后端；OTLP HTTP `tempo:4318`，Grafana 通过 Tempo datasource 查询 trace；docker compose 固定 `grafana/tempo:2.8.1`，避免 `latest` 拉到不兼容的 RC schema