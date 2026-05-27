---
name: aim-attachment-domain
description: AIM 的附件域。涉及 `app/attachment`、AttachmentService gRPC、上传/下载授权、SeaweedFS/S3 签名、附件元数据、`aim.attachment.uploaded` 事件时使用。
---

# aim-attachment-domain

## 边界

- `attachment` 是内部 gRPC 服务，不向客户端/公网暴露 REST/WS。
- 客户端附件能力只能经 gateway 的 `/api/attachments/*` REST 入口访问；gateway 通过 `AttachmentRpc` 调用 `attachment.rpc`。
- core 校验附件消息引用只能通过 `AttachmentService.ValidateReference` gRPC，不再调用内部 HTTP。
- `attachment` 可直接访问 SeaweedFS/S3 与 PostgreSQL；媒体附件（`image`/`video`/`audio`）上传完成后发布 `aim.attachment.uploaded`，普通文件（`file`）上传完成后直接置为解析完成，不进入 data_parsing。
- 不要恢复 `internal/httpx`、`http.Server`、`/v1/attachments/*` 等模块间 HTTP 接口。

## 关键位置

- `app/attachment/attachment.go`：服务入口，启动 go-zero `zrpc`，注册 Nacos 服务 `attachment.rpc`。
- `app/attachment/rpc/attachment.proto`：AttachmentService 协议源。
- `app/attachment/rpc/pb/`：protoc 生成代码，除重新生成外不要手改。
- `app/attachment/internal/server/`：gRPC server 适配层，负责 pb 与业务 service DTO 转换、错误映射。
- `app/attachment/internal/service/`：附件核心业务、SeaweedFS/S3 预签名与对象校验。
- `app/attachment/model/migrations/`：附件表结构。
- `app/attachment/rpc/etc/attachment.yaml`：本地/Compose 运行配置。

## 变更规则

- RPC 变更必须先改 `app/attachment/rpc/attachment.proto`，再在 `app/attachment/rpc` 下执行：
  `protoc --go_out=. --go-grpc_out=. attachment.proto`。
- 新增字段只追加字段号，不复用旧字段号；保持 gateway/core 调用兼容。
- 对外 REST 字段转换在 gateway 完成；attachment 只暴露内部 pb 类型。
- gRPC 业务错误使用 `errorx.NewCodeError` 映射：参数错误 `CodeBadInput`，无记录 `CodeNotFound`，权限/引用不匹配 `CodeForbidden`，基础设施错误对外清洗为 `internal error`。
- SeaweedFS/S3 HTTP 调用是对象存储协议，不是 AIM 模块间 API；必须用 context-aware request 并保留 tracing。
- 媒体上传完成事件使用 `aim.attachment.uploaded`，key 使用 `file_id`，payload 要携带 trace context；`file` 普通附件不得发布给 data_parsing。

## 常用验证

```bash
go test ./app/attachment/...
go test ./app/gateway/api/internal/handler/attachments/... ./app/core/rpc/internal/logic/...
go build ./app/attachment/... ./app/gateway/api/... ./app/core/rpc/...
```
