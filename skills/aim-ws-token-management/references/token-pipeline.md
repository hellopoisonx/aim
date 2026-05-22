# Token 流水线与 WS 连接生命周期

## 问题背景

`ws-message` 压测跑了 440 秒（~7.3 分钟），结果：

| 指标 | 值 |
|------|-----|
| 总请求 | 3990 |
| 成功 | 1721 (43.1%) |
| 失败 | 2269 (56.9%) |
| `ws_disconnected` | 2244 (占错误 99%) |
| `recv_timeout` | 20 |
| `WebSocketConnectionClosedException` | 5 |

**时间线还原**：
- 0~2min：Step 1-4 注册/登录/Kafka 等待/建会话/连 WS
- 2~5min：消息正常收发（1721 成功）
- **~5min**：Token 过期 → 服务端批量踢连接 → 所有 WS 断开
- 5~7.3min：全部后续发送失败 → `ws_disconnected`（2244 个）

---

## 根因 1：配置 TTL 被静默忽略

### 配置链路（修复前）

```
auth.yaml                     auth/rpc/internal/svc            auth/rpc/internal/service      shared/jwt
┌────────────────┐           ┌─────────────────────────┐      ┌───────────────────────┐       ┌──────────────────────┐
│ Token:         │           │ c.Token.AccessTTL       │      │ JWTIssuer {           │       │ NewManager(secret)   │
│   AccessTTL: 5m│ ────────→ │                         │ ───→ │   secret: "aim-dev-.." │ ────→ │   secretKey: []byte   │
│   AccessSecret │  config   │ NewJWTIssuer(           │      │ }                      │       │ }                     │
│     : ...      │           │   c.Token.AccessSecret, │      │ Issue() {              │       │ GenerateAccessToken() │
└────────────────┘           │   c.Token.AccessTTL,    │      │   NewManager(secret)   │       │   AccessTokenTTL ─────┐│
                              │ )                        │      │     .GenerateAccess.. │       │   = 5 * time.Minute   ││
                              └─────────────────────────┘      │ }                      │       │                       ││
                                                                └───────────────────────┘       │   ❌ TTL 参数被丢弃！  ││
                                                                                                 └───────────────────────┘│
                                                                                                   硬编码 5min ────────────┘
```

`NewJWTIssuer` 接收了 `ttl time.Duration` 参数但 `JWTIssuer` 结构体没有存储它，`Issue()` 只用了 `secret` 创建 `NewManager`，TTL 参数被丢弃。

### 修复

**`shared/jwt/jwt.go`**：

```go
type Manager struct {
    secretKey []byte
    accessTTL time.Duration  // 新增字段
}

func NewManager(secretKey string) *Manager {
    return &Manager{
        secretKey: []byte(secretKey),
        accessTTL: AccessTokenTTL,  // 默认 5min，保持向后兼容
    }
}

func NewManagerWithTTL(secretKey string, ttl time.Duration) *Manager {
    return &Manager{
        secretKey: []byte(secretKey),
        accessTTL: ttl,             // 使用自定义 TTL
    }
}

func (m *Manager) GenerateAccessToken(...) (string, int64, error) {
    expiresAt := time.Now().Add(m.accessTTL)  // 使用实例字段而非常量
    // ...
}
```

**`auth/rpc/internal/service/auth_service.go`**：

```go
type JWTIssuer struct {
    secret string
    ttl    time.Duration  // 新增字段
}

func NewJWTIssuer(secret string, ttl time.Duration) *JWTIssuer {
    return &JWTIssuer{secret: secret, ttl: ttl}
}

func (i *JWTIssuer) Issue(...) (string, int64, error) {
    return sharedjwt.NewManagerWithTTL(i.secret, i.ttl).GenerateAccessToken(userID, deviceID)
}
```

---

## 根因 2：WSClient 重连不刷新 Token

### 服务端行为

[ws_handler.go](file:///c:/Users/hpxx/GolandProjects/aim/app/gateway/api/internal/handler/ws/ws_handler.go#L103-L118) 在注册连接时设置了 Token 过期定时器：

```go
connEntry.ExpiryTimer = time.AfterFunc(duration, func() {
    h.sendTokenExpired(ctx, conn, identity, tokenExpiresAt.Unix())
})
```

`sendTokenExpired` 发送 `FRAME_TYPE_TOKEN_EXPIRED` 帧后 `conn.Close(StatusPolicyViolation, "token expired")`，连接被服务端主动断开。

### 客户端行为（修复前）

```python
def _on_close(self, ws, status, msg):
    if not self._intentional_close and self._reconnect_max > 0:
        self.reconnect(max_retries=1)  # 用同一个过期 token 重连 → 必然失败
```

```python
# benchmark send_one 中：
with reconnect_locks[sender_idx]:
    if not ws.is_connected():
        ws.reconnect(max_retries=1)  # 同样用过期 token
```

重连用的是同一个 `self.token.access_token`，已经过期，gateway 的 `wsauth.ExtractAndValidate` 验签会失败，返回 401 拒绝升级。

### 修复：WSClient 自动刷新 Token

```python
class WSClient:
    def __init__(self, ..., rest_client: 'RESTClient' = None):
        self._rest_client = rest_client  # 新增

    def reconnect(self, max_retries=None):
        self._try_refresh_token()  # 新增：重连前刷新
        # ... 原有重连逻辑 ...

    def _try_refresh_token(self):
        if not self.token.is_expired():
            return
        if self._rest_client is None:
            return
        try:
            self._rest_client.refresh()  # POST /api/auth/refresh
        except Exception:
            pass
```

**关键设计点**：
- `WSClient` 和 `RESTClient` 共享**同一个 `TokenManager` 实例**（Python 对象引用），`refresh()` 更新 `self.token.access_token` 后，`connect()` 读取的 `self.token.access_token` 自然是最新的。
- `RESTClient._post()` 每次请求动态调用 `self.token.auth_header()`，不需要额外清理 session。

---

## 根因 3：Benchmark 数据流缺失 Token 字段

### 修复前

```python
user_creds = []    # (user_id, access_token)          ← 只有 2 个字段
conv_pairs = []    # (conv_id, a_id, a_token, b_id, b_token)  ← 没有 refresh_token

# Step 3/4 创建 WSClient 时：
token_mgr = TokenManager.load()   # 从文件加载（可能过期或属于其他用户）
token_mgr.access_token = a_token   # 只设了 access_token
# ❌ refresh_token / expires_at / device_id 全为空
```

`TokenManager.load()` 从磁盘加载的是多线程竞争下最后一个注册用户的 token 状态，且缺少 `refresh_token`、`expires_at`、`device_id`。即使 `WSClient` 有了 `_try_refresh_token()`，也无法真正刷新（`RESTClient.refresh()` 需要 `self.token.refresh_token`）。

### 修复后

```python
user_creds = []    # (user_id, access_token, refresh_token, expires_at, device_id)
conv_pairs = []    # (conv_id, a_id, a_token, a_refresh, a_expires, a_device,
                   #           b_id, b_token, b_refresh, b_expires, b_device)

# reg_login_and_conv 中保存完整信息：
uid = client.token.user_id
token = client.token.access_token
refresh = client.token.refresh_token
expires = client.token.expires_at
device = client.token.device_id
user_creds.append((uid, token, refresh, expires, device))

# Step 3/4 创建 WSClient 时：
token_mgr = TokenManager.load()
token_mgr.access_token = b_token
token_mgr.refresh_token = b_refresh    # ✅
token_mgr.expires_at = b_expires      # ✅
token_mgr.device_id = b_device        # ✅
token_mgr.user_id = b_id
rest = RESTClient(token=token_mgr)
ws = WSClient(token=token_mgr, rest_client=rest)
```

`WsMessageScenario` 和 `MixedScenario` 都已同步修复。

---

## 压测配置

`dev-tool/etc/auth.yaml`：

```yaml
Token:
  AccessTTL: 30m    # 从 5m 增加到 30m，覆盖典型压测时长
```

现在该值会真正生效（经过 `JWTIssuer` → `NewManagerWithTTL` → `GenerateAccessToken`）。

---

## 排查方法论

当压测出现大面积 `ws_disconnected` 时：

1. **看时间线**：对比压测 `duration_s` 和 Token TTL。若 `duration_s > TTL`，大概率是过期问题。
2. **确认服务端是否主动踢人**：查 gateway 日志，应能看到 `"token expired"` 或 `StatusPolicyViolation` 的 close reason。
3. **确认客户端是否尝试重连**：`WSClient` 日志中应有 `"WS reconnect attempt"` 和错误信息。
4. **确认 Token 是否刷新**：检查 `WSClient` 和 `RESTClient` 的 `TokenManager` 是否为同一实例。

---

## 涉及文件

| 文件 | 角色 |
|------|------|
| `shared/jwt/jwt.go` | JWT Manager，签发/验签，持有 TTL |
| `auth/rpc/internal/service/auth_service.go` | JWTIssuer，封装 Manager |
| `auth/rpc/internal/svc/service_context.go` | 组装 JWTIssuer，读取配置 TTL |
| `auth/rpc/etc/auth.yaml` | 配置 AccessTTL/RefreshTTL |
| `dev-tool/etc/auth.yaml` | 压测环境配置 |
| `gateway/api/internal/handler/ws/ws_handler.go` | Token 过期定时器、sendTokenExpired |
| `gateway/api/internal/ws/auth/token.go` | WS 握手时 JWT 验签 |
| `dev-tool/aim_test.py` | WSClient、RESTClient、TokenManager |
| `dev-tool/benchmark.py` | WsMessageScenario、MixedScenario |
