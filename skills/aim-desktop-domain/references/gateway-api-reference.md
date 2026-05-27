# Desktop Gateway API 参考

Desktop 只通过 gateway 通信：

- REST：`app/desktop/internal/api.Client`
- WebSocket：`app/desktop/internal/ws.Client`
- WS 编码：`shared/proto/ws/ws.proto` 的 Protobuf binary frame

## REST 通用规则

- Base URL 默认 `http://localhost:8888`，可在设置中修改。
- 登录态接口使用 `Authorization: Bearer <access_token>`。
- 响应可能是业务 Envelope：`{ code, msg, body }`。`code != 0` 时 Desktop 视为错误；`body` 反序列化为目标 DTO。
- Snowflake ID 在 Go 内部以 `int64` 处理，暴露给前端时转为字符串，避免 JS number 精度丢失。

## 认证

### 注册

```http
POST /api/auth/register
Content-Type: application/json
```

请求：

```json
{
  "email": "a@example.com",
  "password": "secret",
  "username": "Alice",
  "avatar": "",
  "device_id": "desktop-device-uuid"
}
```

响应：

```json
{ "user_id": 123 }
```

### 登录

```http
POST /api/auth/login
```

请求：

```json
{
  "email": "a@example.com",
  "password": "secret",
  "device_id": "desktop-device-uuid"
}
```

响应：

```json
{
  "user_id": 123,
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": 1700000000
}
```

### 刷新 Token

```http
POST /api/auth/refresh
```

请求：

```json
{ "refresh_token": "..." }
```

响应同登录 Token 字段。

### 登出

```http
POST /api/auth/logout
Authorization: Bearer <access_token>
```

Desktop 登出时 best-effort 调用。服务端会踢掉当前 JWT 对应设备的 WS 连接；Desktop 仍需本地断开 WS 并清空当前账号 Token。

## 用户与好友

| 方法 | 路径 | Desktop 方法 |
|---|---|---|
| 搜索用户 | `GET /api/users/by-name/{name}` | `SearchUsers(name)` |
| 按 ID 获取用户 | `GET /api/users/by-id/{id}` | `resolveUserInfo(id)` |
| 发送好友申请 | `POST /api/users/friends/{id}` | `AddFriend(id)` |
| 好友申请列表 | `GET /api/friends/applications` | `ListFriendApplications()` |
| 我的好友 | `GET /api/friends/me` | `ListFriends()` |
| 接受好友 | `POST /api/friends/accept/{id}` | `AcceptFriend(id)` |
| 拒绝好友 | `POST /api/friends/reject/{id}` | `RejectFriend(id)` |
| 好友在线状态 | `GET /api/presence/friends` | `GetFriendsPresence()` |

好友和在线状态响应会被 Desktop upsert 到本地 SQLite，用于离线展示和展示名补齐。

## 会话与群聊

| 方法 | 路径 | Desktop 方法 |
|---|---|---|
| 会话列表 | `GET /api/conversations/` | `ListConversations()` |
| 创建会话 | `POST /api/conversations/` | `CreateConversation(req)` |
| 创建群聊 | `POST /api/conversations/group` | `CreateGroup(req)` |
| 历史消息 | `GET /api/conversations/history/{id}` | `GetConversationHistory(cid, cursorCreatedAt, cursorID, limit)` |
| 成员列表 | `GET /api/conversations/{id}/members` | `GetConversationMembers(cid)` |
| 添加群成员 | `POST /api/conversations/{id}/members` | `AddGroupMembers(cid, ids)` |
| 移除群成员 | `DELETE /api/conversations/{id}/members/{uid}` | `RemoveGroupMember(cid, uid)` |
| 退出群聊 | `POST /api/conversations/{id}/leave` | `LeaveGroup(cid)` |
| 解散群聊 | `DELETE /api/conversations/{id}` | `DismissGroup(cid)` |
| 更新群资料 | `PUT /api/conversations/{id}` | `UpdateGroupInfo(cid, req)` |

### 创建会话请求

```json
{
  "conversation_type": "direct",
  "member_ids": [456],
  "name": "",
  "avatar": ""
}
```

`conversation_type` 合法值：`direct` / `group`。

### 历史消息查询

可选查询参数：

- `cursor_created_at`
- `cursor_id`
- `limit`

响应字段：

- `messages[]`
- `next_cursor_created_at`
- `next_cursor_id`
- `has_more`
- `read_states[]`

Desktop 会把历史消息 upsert 到本地 DB；`has_more=false` 时标记该会话更早历史已拉到底。

## WebSocket

### 建连

```http
GET /ws
Authorization: Bearer <access_token>
```

Desktop 使用 `coder/websocket`，只发送/接收 Protobuf binary frame。

### 帧规则

- 外层：`ws.WsFrame{ type, seq, payload, timestamp }`
- payload：按 `type` 对应的 Protobuf message 编码。
- 客户端发出的 `seq` 单连接递增。
- 服务端推送带 `seq` 时，Desktop 处理后发送 `CLIENT_ACK`。

### Desktop 发送帧

| FrameType | Payload | 触发方法 |
|---|---|---|
| `FRAME_TYPE_SEND_MESSAGE` | `SendMessagePayload` | `SendMessage()` |
| `FRAME_TYPE_TYPING` | `TypingPayload` | `SendTyping()` |
| `FRAME_TYPE_READ_RECEIPT` | `ReadReceiptPayload` | `SendReadReceipt()` |
| `FRAME_TYPE_ACK` | `ClientAckPayload` | 推送处理后自动 ACK |
| `FRAME_TYPE_HEARTBEAT` | `HeartbeatPayload` | 20 秒心跳 |

### Desktop 接收帧

| FrameType | 行为 |
|---|---|
| `FRAME_TYPE_PUSH_MESSAGE` | upsert 消息缓存，emit `ws:message`，ACK。 |
| `FRAME_TYPE_PUSH_PRESENCE` | upsert 在线状态，emit `ws:presence`，ACK。 |
| `FRAME_TYPE_PUSH_TYPING` | emit `ws:typing`，ACK。 |
| `FRAME_TYPE_PUSH_READ_RECEIPT` | emit `ws:read-receipt`，ACK。 |
| `FRAME_TYPE_PUSH_FRIEND_APPLICATION` | emit `ws:friend-application`，ACK。 |
| `FRAME_TYPE_SERVER_ACK` | emit `ws:server-ack`，用于回填发送状态。 |
| `FRAME_TYPE_TOKEN_EXPIRED` | emit `ws:token-expired`，前端触发刷新。 |

### 消息发送语义

`SendMessagePayload`：

- `conversation_id`
- `message_type`（常用 `text`）
- `content`
- `client_msg_id`（Desktop 生成 UUID，幂等关键）
- `mentions[]`（十进制用户 ID 字符串）

收到 `SERVER_ACK`：

- accepted：按 `client_msg_id` 回填服务端 `message_id` 与状态。
- rejected：展示错误，不要自动重发同一 `client_msg_id`。
- 限流会以 WS ACK 形式返回：`code=42900`，不是 HTTP 429。

### 已读与输入中

- 输入中：`TYPING(conversation_id)`；其他成员收到 `PUSH_TYPING`。
- 已读：`READ_RECEIPT(conversation_id, last_msg_id)`；其他成员收到 `PUSH_READ_RECEIPT`。
- 历史接口同时返回 `read_states[]`，用于初始已读展示。

## 附件

### 上传初始化

```http
POST /api/attachments/init
Authorization: Bearer <access_token>
```

请求：

```json
{
  "conversation_id": 123456,
  "kind": "image",
  "original_name": "photo.png",
  "mime": "image/png",
  "size": 204800,
  "sha256": "abc..."
}

```

`kind` 支持 `image` / `video` / `audio` / `file`；普通文件使用 `file`，上传完成后不进入 data_parsing。

响应 `body` 为 uploaded URL + fields，Desktop 随后直传 SeaweedFS。

### 上传完成确认

```http
POST /api/attachments/{fileID}/complete
Authorization: Bearer <access_token>
```

请求 body 可选 `{"sha256": "..."}`。响应为 `AttachmentFileInfo`。

### 获取附件信息

```http
GET /api/attachments/{fileID}
Authorization: Bearer <access_token>
```

### 获取下载授权

```http
GET /api/attachments/{fileID}/download
Authorization: Bearer <access_token>
```

响应为临时下载 URL + headers。

| 方法 | Desktop 方法 |
|---|---|
| `InitAttachmentUpload` | `a.api.InitAttachmentUpload()` |
| `CompleteAttachmentUpload` | `a.api.CompleteAttachmentUpload()` |
| `GetAttachment` | `a.api.GetAttachment()` |
| `GetAttachmentDownload` | `a.api.GetAttachmentDownload()` |

Desktop 发送附件：`ChooseAttachmentAndSend(cid)` → 选择文件 → `UploadAttachmentAndSend(cid, path, kind)` → init → SeaweedFS 直传 → complete → WS `SEND_MESSAGE`（`message_type=image/video/audio/file`，content 为 `aim.attachment.v1` JSON）。

## 兼容规则

- 客户端必须容忍推送中的 `conversation_type` 为空。
- `mentions` 始终按字符串用户 ID 处理。
- 系统消息使用 `is_system=true` 或 `message_type="system"` 区分。
- 不新增 JSON/text WS 协议；跨端线缆协议统一走 Protobuf。
