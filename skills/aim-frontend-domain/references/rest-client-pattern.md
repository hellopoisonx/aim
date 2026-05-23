# REST 客户端模式

## 概述

`app/frontend/client` 包实现与 `app/gateway` 的 REST API 通信。所有 DTO 定义在客户端本地，不导入 gateway 内部包。

## 架构模式

```
Vue ──(Wails Bindings)──→ App (Go) ──→ RESTClient ──HTTP──→ Gateway
```

- Vue 不直接调用 REST；通过 Wails 生成的 `../wailsjs/go/main/App` 绑定调用 `App` 的公开方法。
- `App` 持有 `*RESTClient`，调用 `restClient()` 获取（受 `sync.RWMutex` 保护）。
- `RESTClient.doRequest()` 封装了 HTTP 请求、JSON 序列化/反序列化、`Envelope` 解码、`errorx.CodeError` 转换。

## Envelope 响应格式

Gateway 统一返回 `Envelope` 包裹结构：

```go
type Envelope struct {
    Code int             `json:"code"`
    Msg  string          `json:"msg"`
    Body json.RawMessage `json:"body,omitempty"`
}
```

- `Code == 0` 表示成功，`Body` 包含业务数据
- `Code != 0` 表示失败，`EnvelopeError()` 返回 `*errorx.CodeError`
- Vue 收到 `*errorx.CodeError` 后调用 `ElMessage.error(err.message)`

## Auth Token 传递

- 无需鉴权的端点（register、login、refresh）：`accessToken` 传空字符串
- 需要鉴权的端点：`doRequest` 自动设置 `Authorization: Bearer <token>`
- `App` 在成功 Login/Refresh 后调用 `setTokens()` 更新内存中的 `accessToken`
- `App` 在每个方法入口通过 `a.mu.RLock()` + `a.accessToken` 获取当前 token

## 请求/响应 DTO 命名约定

每个 DTO 以 `Gateway.api` 中的对应消息命名为前缀 `client.`：

| gateway.api 类型 | client DTO |
|---|---|
| `ConversationItem` | `client.ConversationItem` |
| `ListConversationsResponse` | `client.ListConversationsResponse` |
| `MessageItem` | `client.MessageItem` |

前端自身定义的请求 DTO 放在 `app.go` 的 `App` 包作用域，不放在 `client` 包：

| app.go 定义 | 用途 |
|---|---|
| `CreateConversationRequest` | 通用创建会话请求，含 `conversation_type` + `member_ids` |
| `SendMessageRequest` | 发送消息请求 |

DTO 定义见 `client.go`，注释标注 `// mirrors gateway.api xxx`。

## JSON Struct Tag 约定

### 背景

go-zero 的配置结构体使用 `json:",optional"` 标志字段可选，但**该选项是 go-zero 配置解析器专有的**，不是标准 `encoding/json` 行为。

Wails 客户端 DTO 通过标准 `encoding/json` 序列化/反序列化，`json:",optional"` 在此没有效果——标准库忽略不识别的 tag 选项，相当于没有指定 omit 行为。

### 规则

| 场景 | 使用 | 示例 |
|---|---|---|
| **配置结构体**（go-zero `Config`） | `json:",optional"` | `Name string `json:"name,optional"`` |
| **客户端 DTO**（`app/frontend` 下所有 `.go` 文件） | `json:",omitempty"` | `Name string `json:"name,omitempty"`` |
| **必须字段** | 不加任何选项 | `ConversationID string `json:"conversation_id"`` |

### 验证命令

```bash
# 确认前端 Go 代码中没有残留的 json:",optional"
rg 'json:"[^"]*optional' app/frontend -g '*.go'
# 期望结果：无输出（空）
```

### 已迁移字段（2026-05-23）

`app/frontend/app.go` 中以下可选字段已从 `optional` 改为 `omitempty`：

| 结构体 | 字段 |
|---|---|
| `CreateConversationRequest` | `Name` |
| `CreateGroupRequest` | `Name`, `Avatar` |
| `UpdateGroupInfoRequest` | `Name`, `Avatar` |

`app/frontend/client/client.go` 中对应字段也已同步迁移。

## 新增 REST 端点的步骤

1. 在 `client.go` 中定义请求/响应 DTO（如果不存在），使用 `json` tag
2. 在 `RESTClient` 上新增方法，调用 `c.doRequest()`
3. 在 `app.go` 中新增 `App.*` 方法（验证 token + 调用 RESTClient）
   - 如果需要前端直接传参的请求结构体，在 `app.go` 顶部 `type` 定义（如 `CreateConversationRequest`），**必须带 `validate` tag**
   - 参数校验错误统一返回 `errorx.NewCodeError(errorx.CodeBadInput, ...)`（code 40000）
4. 更新 `ProtocolCatalog()` 中的 REST 端点列表
5. 更新 `frontend-coverage-matrix.md` 覆盖矩阵
6. 执行 `go test ./app/frontend/client/...`
7. 从 `app/frontend` 目录执行 `wails generate module` 生成前端绑定
8. 在 Vue 中导入并使用新方法

| Method | Path | Auth | Client DTO |
|---|---|---|---|
| POST | `/api/auth/register` | 无 | `RegisterRequest` / `RegisterResponse` |
| POST | `/api/auth/login` | 无 | `LoginRequest` / `LoginResponse` |
| POST | `/api/auth/refresh` | 无 | `RefreshRequest` / `RefreshResponse` |
| POST | `/api/auth/logout` | Bearer | `LogoutResponse` |
| GET | `/api/users/by-name/{name}` | Bearer | `SearchUsersResponse` |
| GET | `/api/users/by-id/{id}` | Bearer | `GetUserByIdResponse` |
| POST | `/api/conversations` | Bearer | `CreateConversationRequest` (client) / `CreateConversationResponse` |
| POST | `/api/conversations/group` | Bearer | `CreateGroupRequest` / `CreateConversationResponse` |
| GET | `/api/conversations` | Bearer | `ListConversationsResponse` |
| GET | `/api/conversations/history/{id}` | Bearer | `GetConversationHistoryResponse` |
| GET | `/api/conversations/{id}/members` | Bearer | `GetConversationMembersResponse` |
| POST | `/api/conversations/{id}/members` | Bearer | `AddGroupMembersRequest` / `CreateConversationResponse` |
| DELETE | `/api/conversations/{id}/members/{uid}` | Bearer | — |
| POST | `/api/conversations/{id}/leave` | Bearer | — |
| DELETE | `/api/conversations/{id}` | Bearer | — |
| PUT | `/api/conversations/{id}` | Bearer | `UpdateGroupInfoRequest` / `UpdateGroupInfoResponse` |
| POST | `/api/users/friends/{id}` | Bearer | `AddFriendResponse` |
| POST | `/api/friends/accept/{id}` | Bearer | `AcceptFriendResponse` |
| POST | `/api/friends/reject/{id}` | Bearer | `RejectFriendResponse` |
| GET | `/api/friends/me` | Bearer | `ListFriendsResponse` |
| GET | `/api/friends/applications` | Bearer | `ListFriendApplicationsResponse` |
