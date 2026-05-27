# AIM Bot Go SDK

`bot_sdk` 是 AIM Bot OpenAPI 的 Go SDK，导入路径为：

```go
import botsdk "github.com/hellopoisonx/aim/bot_sdk"
```

> 目录名保留为 `bot_sdk/`，Go 包名为更符合 Go 习惯的 `botsdk`。

## 功能

- Bot OpenAPI REST Client：发送消息、读取身份、会话、历史消息、成员、已读状态、附件下载、Webhook 配置。
- 统一解析 AIM `{code,msg,body}` 响应 envelope，并返回 `APIError`。
- Webhook rotate-secret 验签工具。
- `AsyncProcessor` 异步 Webhook 处理器：有界队列、worker pool、重试/退避、去重接口、失败回调、优雅关闭。

## 安装/引用

本 SDK 位于 AIM 主模块内，其他 Go 包可直接引用：

```go
import botsdk "github.com/hellopoisonx/aim/bot_sdk"
```

## REST Client

配置入口是 `NewClient` 与 Option：

```go
client, err := botsdk.NewClient(
    "http://127.0.0.1:8888",       // gateway baseURL
    os.Getenv("AIM_BOT_TOKEN"),    // Bot token，不要带 "Bot " 前缀
    botsdk.WithTimeout(10*time.Second),
    botsdk.WithUserAgent("my-bot/1.0"),
)
if err != nil {
    log.Fatal(err)
}
```

可用 Option：

- `WithHTTPClient(*http.Client)`：使用自定义 HTTP client。
- `WithUserAgent(string)`：覆盖默认 UA。
- `WithBasePath(string)`：覆盖默认 `/api/bot/v1`。
- `WithTimeout(time.Duration)`：设置默认 HTTP 超时。

## 常用接口示例

### 获取 Bot 身份

```go
me, err := client.GetMe(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Println(me.Bot.BotUserID, me.Bot.Nickname)
```

### 发送消息

```go
resp, err := client.SendMessage(ctx, botsdk.SendMessageRequest{
    ConversationID: "1",
    MessageType:    "text",
    Content:        "hello from bot sdk",
    ClientMsgID:    uuid.NewString(),
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.MessageID)
```

### 读取历史消息

```go
history, err := client.GetConversationHistory(ctx, botsdk.GetConversationHistoryRequest{
    ConversationID: "1",
    Limit:          50,
})
if err != nil {
    log.Fatal(err)
}
for _, msg := range history.Messages {
    fmt.Println(msg.ID, msg.SenderID, msg.Content)
}
```

下一页使用响应游标：

```go
next, err := client.GetConversationHistory(ctx, botsdk.GetConversationHistoryRequest{
    ConversationID:   "1",
    CursorCreatedAt: history.NextCursorCreatedAt,
    CursorID:        history.NextCursorID,
    Limit:           50,
})
```

### 读取成员与已读状态

```go
members, err := client.GetConversationMembers(ctx, "1")

readStates, err := client.ListReadStates(ctx, "1")

markRead, err := client.MarkRead(ctx, botsdk.MarkReadRequest{
    ConversationID:     "1",
    LastReadMessageID: "123456789",
})
```

### 获取附件下载地址

```go
dl, err := client.DownloadAttachment(ctx, fileID)
if err != nil {
    log.Fatal(err)
}
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, dl.URL, nil)
for k, v := range dl.Headers {
    req.Header.Set(k, v)
}
```

## Webhook 配置与 rotate-secret

推荐使用 `rotate_secret=true` 创建/轮换 Webhook secret。服务端只在本次响应返回 `plaintext_secret`，之后无法再次读取明文。

```go
enabled := true
resp, err := client.SetWebhook(ctx, botsdk.SetWebhookRequest{
    URL:          "https://your-bot.example.com/aim/webhook",
    Events:       []string{botsdk.EventMessageCreated},
    Enabled:      &enabled,
    RotateSecret: true,
})
if err != nil {
    log.Fatal(err)
}

// 必须安全保存，例如写入密钥管理系统或环境变量。
plaintextSecret := resp.PlaintextSecret
```

AIM V0 的签名规则为：

```text
X-AIM-Signature = sha256=<hex(HMAC-SHA256(SHA256(plaintext_secret), raw_body))>
```

SDK 的 `VerifySignature` 已封装该规则：

```go
ok := botsdk.VerifySignature(plaintextSecret, rawBody, r.Header.Get("X-AIM-Signature"))
```

如需兼容已有部署中直接保存 `secret_hash` 的场景，可使用：

```go
ok := botsdk.VerifySignatureWithSecretHash(secretHash, rawBody, signatureHeader)
```

## Webhook 异步处理器

`AsyncProcessor` 实现了 `http.Handler`，适合直接挂到 Bot 服务的 HTTP 路由上。它会：

1. 读取 raw body。
2. 使用 plaintext secret 验签。
3. 解析 `WebhookEvent`。
4. 基于 `event_id` 去重。
5. 将事件放入有界队列。
6. 快速返回 `202 Accepted`。
7. 后台 worker 异步执行业务 handler。

```go
processor, err := botsdk.NewAsyncProcessor(
    plaintextSecret,
    botsdk.MessageHandlerFunc(func(ctx context.Context, event botsdk.WebhookEvent) error {
        // 业务处理。返回 error 会触发重试。
        log.Printf("message %s from %s: %s", event.Message.MessageID, event.Message.SenderID, event.Message.Content)
        return nil
    }),
    botsdk.WithProcessorWorkers(4),
    botsdk.WithProcessorQueueSize(1024),
    botsdk.WithProcessorMaxRetries(3),
    botsdk.WithProcessorFailureHandler(func(ctx context.Context, event botsdk.WebhookEvent, err error) {
        log.Printf("webhook event failed: event_id=%s err=%v", event.EventID, err)
    }),
)
if err != nil {
    log.Fatal(err)
}

defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _ = processor.Shutdown(shutdownCtx)
}()

http.Handle("/aim/webhook", processor)
log.Fatal(http.ListenAndServe(":9000", nil))
```

### 运行时密钥轮换

`AsyncProcessor.UpdateSecret` 允许在进程运行期间更新验签密钥，无需重启服务：

```go
processor.UpdateSecret(newPlaintextSecret)
```

传入空字符串会被忽略；方法内部加锁，与 `ServeHTTP` 并发调用安全。

典型的轮换流程：先调 `PUT /api/bot/v1/webhook` 设置 `rotate_secret=true`，拿到新的
`plaintext_secret` 后调用 `UpdateSecret` 更新即可。


### 队列满与 AIM 重试

默认队列满时返回 `503 Service Unavailable`，AIM Webhook 投递端会按指数退避重试。可自定义状态码：

```go
botsdk.WithQueueFullStatus(http.StatusTooManyRequests)
```

### 幂等去重

默认 `MemoryDeduper` 仅适合本地开发或单实例部署。生产环境建议实现 `Deduper` 接口并接入 Redis/数据库：

```go
type Deduper interface {
    Seen(ctx context.Context, eventID string) (bool, error)
    MarkSeen(ctx context.Context, eventID string) error
}
```

接入方式：

```go
processor, err := botsdk.NewAsyncProcessor(
    plaintextSecret,
    handler,
    botsdk.WithProcessorDeduper(redisDeduper),
)
```

## 错误处理

SDK 会将非 2xx 或 `code != 0` 的响应解析为 `*botsdk.APIError`：

```go
_, err := client.SendMessage(ctx, req)
if err != nil {
    if apiErr, ok := botsdk.AsAPIError(err); ok {
        log.Printf("AIM error: http=%d code=%d msg=%s", apiErr.StatusCode, apiErr.Code, apiErr.Message)
    }
    if botsdk.IsCode(err, 40310) {
        log.Println("token 缺少所需 action")
    }
}
```

## 测试

SDK 单元测试默认不依赖外部服务：

```bash
go test ./bot_sdk
```

`integration` build tag 会启用包内集成测试环境。该环境使用 `httptest` 构造内存版 Bot Gateway，覆盖 REST Client 全链路方法、APIError 解析，以及 Gateway Webhook 投递到 `AsyncProcessor` 的签名/去重/异步处理流程：

```bash
go test -tags=integration ./bot_sdk
```

需要真实 AIM 服务栈时，可使用包内 Docker Compose 环境。它独立于根目录本地开发环境，宿主机端口全部按主环境 `+3000` 偏移：

```bash
docker compose -f bot_sdk/testdata/integration/docker-compose.yaml up -d
# Gateway REST: http://127.0.0.1:11888
docker compose -f bot_sdk/testdata/integration/docker-compose.yaml down -v
```

## ID 与时间约定

- Bot OpenAPI 与 Webhook 中的 Snowflake ID 使用十进制字符串，避免 JavaScript/JSON number 精度问题。
- `created_at`、`updated_at`、`expires_at` 等时间字段通常为 Unix milliseconds。
- `client_msg_id` 是消息幂等键，建议使用 UUID。

## 需要的 Bot action

| SDK 方法 | 需要的 action |
|---|---|
| `GetMe` | `bot.self.read` |
| `ListConversations` | `bot.conversation.list` |
| `GetConversationHistory` | `bot.conversation.history` |
| `GetConversationMembers` | `bot.conversation.members.read` |
| `SendMessage` | `bot.message.send` |
| `MarkRead` | `bot.read_receipt.write` |
| `ListReadStates` | `bot.read_receipt.read` |
| `DownloadAttachment` | `bot.attachment.download` |
| `GetWebhook` | `bot.webhook.read` |
| `SetWebhook` | `bot.webhook.write` 与对应订阅 action，如 `bot.webhook.subscribe.message_created` |
| `DeleteWebhook` | `bot.webhook.delete` |

支持通配 grant：`*`、`bot.*`、`bot.conversation.*`、`bot.message.*`、`bot.read_receipt.*`、`bot.webhook.subscribe.*`。
