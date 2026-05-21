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
| GET | `/api/conversations` | Bearer | `ListConversationsResponse` |
| GET | `/api/conversations/history/{id}` | Bearer | `GetConversationHistoryResponse` |
| POST | `/api/users/friends/{id}` | Bearer | `AddFriendResponse` |
| POST | `/api/friends/accept/{id}` | Bearer | `AcceptFriendResponse` |
| POST | `/api/friends/reject/{id}` | Bearer | `RejectFriendResponse` |
| GET | `/api/friends/me` | Bearer | `ListFriendsResponse` |
| GET | `/api/friends/applications` | Bearer | `ListFriendApplicationsResponse` |
