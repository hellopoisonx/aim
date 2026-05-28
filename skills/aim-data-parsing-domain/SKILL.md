---
name: aim-data-parsing-domain
description: AIM 的附件解析域。涉及 `app/data_parsing`、`aim.attachment.uploaded` 消费、媒体元数据提取、缩略图/派生对象、解析结果回写、`aim.attachment.parsed` 事件时使用。
---

# aim-data-parsing-domain

## 边界

- `data_parsing` 是后台 Kafka worker，不向客户端/公网暴露 REST/WS/gRPC 服务。
- 输入只来自媒体附件的 `aim.attachment.uploaded`；输出为 PostgreSQL 解析状态更新、SeaweedFS 派生对象、`aim.attachment.parsed` 事件。普通 `file` 附件不应进入该链路。
- 客户端不可直接访问 data_parsing；解析结果通过 gateway/logic/attachment 的既有查询链路体现。
- data_parsing 可以访问 SeaweedFS/S3 与 PostgreSQL，但不得承担上传授权、下载授权、消息投递或客户端协议职责。

## 关键位置

- `app/data_parsing/data_parsing.go`：worker 入口，加载配置并启动 Kafka consumer。
- `app/data_parsing/internal/config/`：配置结构。
- `app/data_parsing/internal/worker/`：消费 `aim.attachment.uploaded`、拉取对象、解析、写库、发布 `aim.attachment.parsed` 的主流程。
- `app/data_parsing/internal/parser/`：解析器接口与默认实现。
- `app/data_parsing/etc/data_parsing.yaml`：本地 `go run` / 单服务调试默认配置；Docker Compose 部署挂载 `deploy/config/<env>/data_parsing.yaml` 到 `/app/etc/data_parsing.yaml`。
- 相关共享事件：`app/shared/events/attachment.go`。

## 变更规则

- 仅 `image`/`video`/`audio` 需要解析；如收到 `file` 等非媒体上传事件，应直接跳过解析并保持/设置 `parse_status=ready`。
- Kafka trace context 必须从上传完成事件恢复，并在解析完成/失败事件中继续传播。
- 解析成功：写入/更新 `attachment_objects` 的 thumbnail 记录、`attachment_parse_results`，并将 `attachment_files.parse_status` 置为 `ready`。
- 解析失败：将 `parse_status` 置为 `failed`，记录错误，并发布失败事件；不要吞掉可重试基础设施错误。
- SeaweedFS/S3 对象读写必须使用 context-aware request；派生对象 key 需稳定、可幂等覆盖。
- 事件发布 key 使用 `file_id`，保证单附件解析事件顺序。
- 不要在 data_parsing 中新增客户端 REST/WS 接口；如需暴露解析结果，走 gateway/logic/attachment 既有边界。

## 常用验证

```bash
go test ./app/data_parsing/...
go test ./app/shared/events/... ./app/shared/attachment/...
go build ./app/data_parsing/...
```

## 最近变更

- 2026-05-27: 普通 `file` 附件不再进入 data_parsing；worker 对意外收到的非媒体事件直接跳过对象读取/解析并置 `parse_status=ready`。
- 2026-05-25: 图片解析对 Go 标准库可解码的图片（PNG/JPEG/GIF）生成最长边 512px 的 PNG 缩略图；不可解码的 `image/*` 仍降级为 1x1 占位缩略图并在 metadata 标记 fallback，避免上传链路因 WebP 等格式回归失败；解析结果/事件记录原图尺寸，thumbnail 对象记录缩略图尺寸。
