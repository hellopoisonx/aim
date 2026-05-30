# AIM 客户端实现指南

本文档面向 AIM 即时通讯系统的客户端开发者（桌面端、移动端、Web 端），提供从认证、消息收发、本地缓存到云端消息漫游的完整实现指引。

> **参考文件**：
> - REST API 规范：[`docs/api/gateway-openapi.yaml`](api/gateway-openapi.yaml)
> - WebSocket 帧协议：[`docs/ws.md`](ws.md)
> - Bot 接入：[`docs/bot-developer-guide.md`](bot-developer-guide.md)
> - Proto 源文件：`shared/proto/ws/ws.proto`、`shared/proto/gateway/gateway.proto`

---

## 目录

1. [系统架构概览](#1-系统架构概览)
2. [认证与会话管理](#2-认证与会话管理)
3. [REST API 使用](#3-rest-api-使用)
4. [WebSocket 帧协议](#4-websocket-帧协议)
5. [消息发送与 ACK](#5-消息发送与-ack)
6. [消息接收与推送](#6-消息接收与推送)
7. [客户端本地缓存策略](#7-客户端本地缓存策略)
8. [云端消息漫游](#8-云端消息漫游)
9. [已读回执](#9-已读回执)
10. [在线状态与输入状态](#10-在线状态与输入状态)
11. [附件消息](#11-附件消息)
12. [群聊与系统消息](#12-群聊与系统消息)
13. [错误处理与重试](#13-错误处理与重试)
14. [完整生命周期示例](#14-完整生命周期示例)
15. [附录：性能与安全建议](#15-附录性能与安全建议)

---

## 1. 系统架构概览

```
┌──────────────────────────────────────────────┐
│                   AIM 服务端                   │
│                                              │
│  ┌──────────────┐   ┌──────────────────────┐ │
│  │ Gateway API  │◄──│ 客户端 / 第三方 Bot   │ │
│  │ (唯一入口)    │   │  REST + WebSocket     │ │
│  └──────┬───────┘   └──────────────────────┘ │
│         │ gRPC/Kafka                          │
│  ┌──────┼──────┬──────┬───────────────┐       │
│  │  Auth  │ Core │ Logic │ Attachment  │      │
│  └───────┴──────┴──────┴───────────────┘      │
│         │       │                            │
│  ┌──────┴───────┴──────────────┐             │
│  │ Kafka / Redis / PostgreSQL │             │
│  └────────────────────────────┘             │
└──────────────────────────────────────────────┘
```

**关键原则**：

1. **客户端只与 Gateway 通信**，不直接访问 Auth/Core/Logic/Attachment 等微服务
2. **REST 用于请求-响应操作**（登录、拉取历史、会话管理等）
3. **WebSocket 用于实时双向通信**（消息收发、推送、心跳）
4. **所有 WS 帧使用 Protobuf 二进制编码**，不支持 JSON 文本帧

### 统一响应格式

REST API 统一返回：

```json
{
  "code": 0,        // 业务错误码，0 表示成功
  "msg": "ok",      // 错误描述
  "body": { }       // 返回数据
}
```

常见错误码：

| code | 含义 |
|------|------|
| 0 | 成功 |
| 40000 | 参数错误 |
| 40100 | 未认证 |
| 40300 | 无权限 |
| 40400 | 资源不存在 |
| 42900 | 触发限流 |
| 50000 | 服务器内部错误 |

---

## 2. 认证与会话管理

### 2.1 注册

```
POST /api/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "min8char",        // 最少 8 位
  "username": "alice",
  "avatar": "https://...",       // 可选
  "device_id": "device-uuid"     // 必填，设备唯一标识
}
```

成功返回 `{ "code": 0, "msg": "ok", "body": { "user_id": 123 } }`。

### 2.2 登录

```
POST /api/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "min8char",
  "device_id": "device-uuid"
}
```

成功返回：

```json
{
  "code": 0,
  "msg": "ok",
  "body": {
    "user_id": 123,
    "access_token": "eyJhbG...",       // JWT，5 分钟有效期
    "refresh_token": "uuid-string",    // UUID，7 天有效期
    "expires_at": 1700000000           // access_token 过期时间（Unix seconds）
  }
}
```

### 2.3 Token 刷新

```
POST /api/auth/refresh
Content-Type: application/json

{
  "refresh_token": "uuid-string"
}
```

成功返回新的 `access_token`、`refresh_token` 和 `expires_at`。旧 token 立即失效。

### 2.4 登出

```
POST /api/auth/logout
Authorization: Bearer <access_token>
```

成功返回 `{ "code": 0, "msg": "ok" }`（当前实现 body 可能为 null）。

### 2.5 Token 使用规范

| 场景 | 使用 |
|------|------|
| REST 鉴权端点 | `Authorization: Bearer <access_token>` |
| WS `/ws` 握手 | `Authorization: Bearer <access_token>` |
| Token 快过期 | `POST /api/auth/refresh` 静默刷新 |
| WS 收到 `TOKEN_EXPIRED` | 停止旧连接 → refresh → 新 token 重连 `/ws` |
| 登出 | `POST /api/auth/logout` → 清除本地 token 存储 |

**建议实现**：

- 存储 token 到设备安全区域（Keychain/Keystore/加密 SQLite）
- 启动时若有有效 `refresh_token`，先尝试 `POST /api/auth/refresh` 避免重复登录
- 定时提前刷新 access token（如过期前 60s）

---

## 3. REST API 使用

本节仅保留客户端实现视角的速查与流程说明；完整 REST 路径、参数、响应 schema 与错误响应以 [`docs/api/gateway-openapi.yaml`](api/gateway-openapi.yaml) 为准。

### 3.1 必读端点速查

| 方法 | 路径 | 用途 | 需鉴权 |
|------|------|------|--------|
| `POST` | `/api/auth/login` | 登录 | 否 |
| `POST` | `/api/auth/refresh` | 刷新 token | 否 |
| `POST` | `/api/auth/logout` | 登出 | 是 |
| `GET` | `/api/users/by-name/:name` | 按昵称搜索用户 | 是 |
| `GET` | `/api/users/by-id/:id` | 获取用户详情 | 是 |
| `POST` | `/api/users/friends/:id` | 添加好友 | 是 |
| `GET` | `/api/friends/me` | 获取好友列表 | 是 |
| `GET` | `/api/friends/applications` | 好友申请列表 | 是 |
| `POST` | `/api/friends/accept/:id` | 接受好友申请 | 是 |
| `POST` | `/api/friends/reject/:id` | 拒绝好友申请 | 是 |
| `GET` | `/api/friends/tags` | 列出好友标签 | 是 |
| `POST` | `/api/friends/tags` | 创建好友标签 | 是 |
| `PUT` | `/api/friends/tags/:id` | 重命名好友标签 | 是 |
| `DELETE` | `/api/friends/tags/:id` | 删除好友标签 | 是 |
| `PUT` | `/api/friends/:id/tags` | 设置好友的标签列表 | 是 |
| `DELETE` | `/api/friends/:id/tags/:tag_id` | 删除好友的某个标签 | 是 |
| `POST` | `/api/conversations` | 创建直聊或群聊 | 是 |
| `POST` | `/api/conversations/group` | 创建群聊 | 是 |
| `GET` | `/api/conversations` | 获取会话列表 | 是 |
| `GET` | `/api/conversations/history/:id` | 拉取消息历史 | 是 |
| `GET` | `/api/conversations/:id/members` | 群成员详情 | 是 |
| `POST` | `/api/conversations/:id/members` | 添加群成员 | 是 |
| `DELETE` | `/api/conversations/:id/members/:uid` | 移除群成员 | 是 |
| `POST` | `/api/conversations/:id/members/:uid/admin` | 授予群管理员 | 是 |
| `DELETE` | `/api/conversations/:id/members/:uid/admin` | 撤销群管理员 | 是 |
| `POST` | `/api/conversations/:id/owner` | 转让群主 | 是 |
| `POST` | `/api/conversations/:id/leave` | 退出群聊 | 是 |
| `DELETE` | `/api/conversations/:id` | 解散群聊 | 是 |
| `PUT` | `/api/conversations/:id` | 更新群信息 | 是 |
| `GET` | `/api/presence/friends` | 好友在线快照 | 是 |
| `POST` | `/api/attachments/init` | 初始化附件上传 | 是 |
| `POST` | `/api/attachments/:id/complete` | 完成附件上传 | 是 |
| `GET` | `/api/attachments/:id` | 获取附件元数据 | 是 |
| `GET` | `/api/attachments/:id/download` | 获取下载授权 URL | 是 |
| `GET` | `/api/search` | 统一搜索（用户/好友/会话/消息） | 是 |

### 3.2 会话列表

```
GET /api/conversations
```

返回当前用户的所有会话：

```json
{
  "code": 0,
  "body": {
    "conversations": [
      {
        "conversation_id": 456,
        "conversation_type": "direct",       // "direct" | "group"
        "is_active": true,
        "created_at": 1700000000000,
        "member_ids": [123, 456],
        "name": "Alice",                      // 单聊时为对方昵称；群聊时为群名
        "avatar": "",
        "creator_id": 123
      }
    ]
  }
}
```

### 3.3 拉取消息历史

```
GET /api/conversations/history/:conversation_id
  ?cursor_created_at=1700000000000   // 可选，上一页最后一条的 created_at
  &cursor_id=10001                    // 可选，上一页最后一条的 id
  &limit=50                           // 可选，默认 50，最大 100
```

返回示例：

```json
{
  "code": 0,
  "body": {
    "messages": [
      {
        "id": 10001,
        "conversation_id": 456,
        "sender_id": 123,
        "sender_info": {
          "name": "alice",
          "email": "alice@example.com"
        },
        "message_type": "text",
        "content": "你好",
        "client_msg_id": "c1-aabbccdd",
        "created_at": 1700000000000,
        "is_system": false,
        "mentions": [],
        "read_details": [
          {
            "user_id": 456,
            "is_read": true,
            "last_read_message_id": 10005,
            "updated_at": 1700000100000,
            "email": "bob@example.com",
            "avatar": "https://..."
          }
        ]
      }
    ],
    "next_cursor_created_at": 1699999900000,
    "next_cursor_id": 9998,
    "has_more": true,
    "read_states": [
      {
        "user_id": 456,
        "last_read_message_id": 10005,
        "updated_at": 1700000100000,
        "email": "bob@example.com",
        "avatar": "https://...",
        "name": "Bob"
      }
    ]
  }
}
```

**翻页说明**：

- 首次拉取：不传 cursor 参数，获取最新 N 条
- 向上翻页：使用响应中的 `next_cursor_created_at` 和 `next_cursor_id` 作为下一次请求的 cursor 参数
- `has_more: false` 表示已到最早的消息
- 消息按时间降序返回（最新在前）
- 时间字段均为 **Unix milliseconds**

### 3.4 好友在线状态快照

```
GET /api/presence/friends
```

返回好友在线状态的初始快照，WS 连接/重连后先调此接口填充状态，后续由 WS 实时更新。

### 3.5 好友标签管理

**列出标签**：
```
GET /api/friends/tags
```
返回 `{"tags": [{"id":1,"user_id":123,"name":"同事","created_at":...,"updated_at":...}]}`。

**创建标签**：
```
POST /api/friends/tags
{"name": "同事"}
```
返回 `{"tag": {...}}`。参数 `name` 必填，最多 64 字符。

**重命名标签**：
```
PUT /api/friends/tags/:id
{"name": "新名称"}
```

**删除标签**：
```
DELETE /api/friends/tags/:id
```
返回 `{"deleted": true}`。删除标签会级联清除所有好友的该标签。

**设置好友标签**（覆盖式）：
```
PUT /api/friends/:friend_id/tags
{"tag_ids": [1, 3]}
```
返回该好友的最新 FriendshipItem（含 tags）。空数组清空所有标签。

**删除好友的单个标签**：
```
DELETE /api/friends/:friend_id/tags/:tag_id
```
删除后其他标签不受影响。

`GET /api/friends/me` 响应中 `FriendshipItem` 现已含 `tags` 字段，类型为 `[{id, user_id, name, created_at, updated_at}]`。

### 3.6 统一搜索

```
GET /api/search
  ?q=搜索关键词
  &scope=users,friends,conversations,messages   // 可选，逗号分隔
  &conversation_id=456                           // 可选，限定消息搜索会话
  &cursor_created_at=1700000000000              // 可选，消息分页游标
  &cursor_id=10001                              // 可选
  &limit=20                                     // 可选，默认 20，最大 100
```

返回按 scope 分类的结果，每条含 `snippet` 高亮片段：

```json
{
  "code": 0,
  "body": {
    "users": [{"user": {...}, "snippet": "含<mark>关键</mark>词的片段"}],
    "friends": [{"friendship": {...}, "user": {...}, "snippet": "..."}],
    "conversations": [{"conversation": {...}, "snippet": "..."}],
    "messages": [{"message": {...}, "snippet": "..."}],
    "next_cursor_created_at": ...,
    "next_cursor_id": ...,
    "has_more": false
  }
}
```

**搜索特点**：

- scope 为空时搜索所有类型
- 消息搜索通过 `conversation_members` 做访问控制，只返回当前用户参与的会话消息
- snippet 中命中部分用 `<mark>...</mark>` 标记
- 消息搜索使用 `(created_at DESC, id DESC)` 游标分页

---

## 4. WebSocket 帧协议

### 4.1 连接

```
GET /ws
Authorization: Bearer <access_token>
```

- **只支持 Protobuf 二进制帧**，不接受 JSON/text frame
- 鉴权失败时返回 HTTP 401 JSON（不升级）
- 同一 `(user_id, device_id)` 只能有一个活跃连接，新连接会踢旧连接

### 4.2 通用帧格式（WsFrame）

```protobuf
message WsFrame {
  FrameType type = 1;     // 帧类型
  int64 seq = 2;          // 序列号
  bytes payload = 3;      // 按 type 反序列化
  int64 timestamp = 4;    // Unix milliseconds
}
```

**编码顺序**：
1. 先序列化具体 Payload（如 `SendMessagePayload`）为 `bytes`
2. 填入 `WsFrame.payload`
3. 再序列化整个 `WsFrame` 为二进制
4. 通过 WebSocket binary message 发送

### 4.3 序列号规则

- **客户端 seq**：单调递增，每次发送自增 1
- **服务端 seq**：单调递增，但不保证连续
- `SERVER_ACK.ack_seq` 匹配被确认的客户端帧 `seq`
- `ClientAckPayload.ack_seq` 匹配被确认的服务端帧 `seq`
- 内部 gRPC 推送帧写出前会由 per-connection writer 为 `seq=0` 的待分配帧补齐正数服务端 seq；客户端仍应容忍历史版本或异常实现中的 `seq=0`，可只对 `seq>0` 的帧发送 `FRAME_TYPE_ACK`

### 4.4 帧类型总览

#### 客户端 → 服务端

| 类型 | 编号 | Payload | 返回 ACK | 说明 |
|------|------|---------|---------|------|
| `FRAME_TYPE_SEND_MESSAGE` | 1 | `SendMessagePayload` | 是（SERVER_ACK） | 发送消息 |
| `FRAME_TYPE_HEARTBEAT` | 2 | `HeartbeatPayload` | 是（SERVER_ACK） | 心跳，保持在线 |
| `FRAME_TYPE_TYPING` | 3 | `TypingPayload` | 否 | 输入状态通知 |
| `FRAME_TYPE_READ_RECEIPT` | 4 | `ReadReceiptPayload` | 是（SERVER_ACK） | 已读游标更新 |
| `FRAME_TYPE_ACK` | 5 | `ClientAckPayload` | 否 | 确认收到服务端推送 |

#### 服务端 → 客户端

| 类型 | 编号 | Payload | 说明 |
|------|------|---------|------|
| `FRAME_TYPE_PUSH_MESSAGE` | 101 | `PushMessagePayload` | 推送聊天消息 / 附件解析更新 |
| `FRAME_TYPE_PUSH_PRESENCE` | 102 | `PushPresencePayload` | 在线状态 |
| `FRAME_TYPE_PUSH_NOTIFICATION` | 103 | `PushNotificationPayload` | 系统通知 |
| `FRAME_TYPE_PUSH_TYPING` | 104 | `PushTypingPayload` | 输入状态 |
| `FRAME_TYPE_RECONNECT` | 105 | `ReconnectPayload` | 服务端要求重连 |
| `FRAME_TYPE_SERVER_ACK` | 106 | `ServerAckPayload` | 服务端确认客户帧 |
| `FRAME_TYPE_TOKEN_EXPIRED` | 107 | `TokenExpiredPayload` | Token 过期 |
| `FRAME_TYPE_PUSH_FRIEND_APPLICATION` | 108 | `PushFriendApplicationPayload` | 好友申请通知 |
| `FRAME_TYPE_PUSH_READ_RECEIPT` | 109 | `PushReadReceiptPayload` | 已读游标更新 |

---

## 5. 消息发送与 ACK

### 5.1 发送流程

```
客户端                          Gateway                         Core
  │                                │                              │
  │  WsFrame{                      │                              │
  │    type=SEND_MESSAGE,          │                              │
  │    seq=N,                      │                              │
  │    payload=SendMessagePayload{ │                              │
  │      conversation_id,          │                              │
  │      message_type,             │                              │
  │      content,                  │   TransferReq.gRPC           │
  │      client_msg_id,            │ ────────────────────────────►│
  │      mentions                  │                              │
  │    }                           │                              │
  │  }                             │                              │
  │ ─────────────────────────────► │                              │
  │                                │   幂等检查 (Redis)            │
  │                                │   权限检查 (Logic)            │
  │                                │   配额检查 (Redis 滑动窗口)   │
  │                                │   Snowflake 生成 message_id   │
  │                                │   发布 Kafka                  │
  │                                │   ◄──── TransferResp ────────│
  │                                │                              │
  │  WsFrame{                      │                              │
  │    type=SERVER_ACK,            │                              │
  │    seq=serviceSeq,             │                              │
  │    payload=ServerAckPayload{   │                              │
  │      ack_seq=N,                │                              │
  │      client_msg_id="c1-...",   │                              │
  │      status=ACCEPTED,          │                              │
  │      code=0,                   │                              │
  │      message_id=10001          │                              │
  │    }                           │                              │
  │  }                             │                              │
  │ ◄───────────────────────────── │                              │
```

### 5.2 SendMessagePayload 结构

```protobuf
message SendMessagePayload {
  int64 conversation_id = 1;      // 目标会话 ID
  string message_type = 2;        // "text" / "image" / "video" / "audio" / "file"
  string content = 3;             // 消息内容（文本或 JSON）
  string client_msg_id = 4;       // 客户端幂等 ID，建议 UUID
  repeated string mentions = 5;   // @用户的 ID 列表
}
```

**字段要求**：

| 字段 | 约束 |
|------|------|
| `conversation_id` | 必填，正数 |
| `message_type` | 必填，最长 32 字符。普通文本用 `"text"`；附件用 `"image"/"video"/"audio"/"file"`；客户端不应使用 `"system"` |
| `content` | 必填。文本消息为纯文本字符串。附件消息为 `aim.attachment.v1` JSON 字符串（详见 §11） |
| `client_msg_id` | 必填，建议 UUID（如 `c1-aabbccdd-1234`），用于幂等和本地消息匹配 |
| `mentions` | 可选，最多 20 个，各元素为被 @ 用户 ID 的十进制字符串 |

### 5.3 ServerAckPayload 结构与 ACK 映射

```protobuf
message ServerAckPayload {
  int64 ack_seq = 1;              // 被确认的客户端帧 seq
  string client_msg_id = 2;       // 回传的 client_msg_id
  int32 code = 3;                 // 0=成功, 其他=错误码
  string msg = 4;                 // 错误描述
  AckStatus status = 5;           // ACCEPTED / REJECTED / RETRYABLE
  int64 message_id = 6;           // 服务端消息 ID（非消息 ACK 时为 0）
}

enum AckStatus {
  ACK_STATUS_UNSPECIFIED = 0;
  ACK_STATUS_ACCEPTED = 1;        // 消息已被接受
  ACK_STATUS_REJECTED = 2;        // 消息被拒绝（不可重试）
  ACK_STATUS_RETRYABLE = 3;       // 可重试
}
```

**ACK 处理建议**：

| status | code | 场景 | 客户端行为 |
|--------|------|------|-----------|
| `ACCEPTED` | 0 | 发送成功 | 标记消息为 `sent`，保存 `message_id` |
| `ACCEPTED` | 0 | 幂等命中（重复发送） | 使用已有的 `message_id`，不重复保存 |
| `REJECTED` | 40000 | 参数错误 | 展示错误提示，不自动重试 |
| `REJECTED` | 40100 | 认证失败 | 刷新 token 后重连 |
| `REJECTED` | 40300 | 无权限/被禁言/拉黑 | 展示业务错误，不自动重试 |
| `REJECTED` | 40400 | 会话不存在 | 刷新会话列表 |
| `REJECTED` | 42900 | 触发限流 | 退避等待后重试 |
| `RETRYABLE` | 50000 | 基础设施错误 | 保留 pending，用相同 `client_msg_id` 退避重试 |
| `UNSPECIFIED` | 0 | 心跳/已读回执成功 | 视为成功 |

### 5.4 client_msg_id 生成建议

```
格式：<设备前缀>-<UUID>
示例：d1-a1b2c3d4e5f6

生成时机：消息发出前在本地生成
用途：
  1. 发送前插入本地缓存，状态=sending
  2. 发送帧携带
  3. 匹配 SERVER_ACK → 更新本地状态
  4. 匹配 PUSH_MESSAGE → 去除自己发的回声消息
  5. 重试时复用同一个值
```

---

## 6. 消息接收与推送

### 6.1 PushMessagePayload 结构

```protobuf
message PushMessagePayload {
  int64 message_id = 1;           // 服务端消息 ID（主键）
  int64 conversation_id = 2;      // 会话 ID
  string message_type = 3;        // 消息类型
  string content = 4;             // 消息内容
  int64 sender_id = 5;            // 发送者（系统消息为 0）
  int64 sent_at = 6;              // 发送时间 Unix ms
  string conversation_type = 7;   // "direct" | "group" | ""
  string client_msg_id = 8;       // 发送者的客户端 ID
  bool is_system = 9;             // 是否为系统消息
  SenderInfo sender_info = 10;    // 发送者快照信息
  repeated string mentions = 11;  // @用户列表
}

message SenderInfo {
  string name = 1;                // 用户名（昵称）
  string email = 2;               // 邮箱
}
```

### 6.2 接收处理流程

```
收到 PUSH_MESSAGE
  │
  ├─ 按 message_id 去重（本地缓存查找）
  │     ├─ 已存在 → 忽略（重复推送）
  │     └─ 不存在 → 继续
  │
  ├─ 判断是否为系统消息
  │     is_system=true && sender_id=0 && message_type="system"
  │     → 按系统消息渲染（群变更通知）
  │
  │     is_system=true && sender_id=0 && message_type in (image/video/audio)
  │     → 附件解析更新：按 file_id 匹配本地消息并原地更新（详见 §11.4）
  │
  ├─ 按 client_msg_id 匹配本地 pending
  │     命中 → 将本地 sending → sent，保存 message_id
  │     未命中 → 新增到本地缓存，状态=received
  │
  ├─ 插入到对应会话的消息列表
  │     按 sent_at / message_id 排序
  │
  └─ 更新 UI（添加到聊天列表、显示未读角标）
```

### 6.3 消息去重

客户端使用 **`message_id` 为主键** 进行缓存去重：

| 场景 | 去重方式 |
|------|---------|
| 收到别人发的消息 | `message_id` 去重 |
| 收到自己发的消息（自己也在会话中） | `client_msg_id` 匹配本地 pending，然后用 `message_id` 去重 |
| 同设备重发 | `SERVER_ACK` 幂等命中，message_id 不变 |
| 离线后收到多条 | 历史拉取结果 + 在线推送的交集用 `message_id` 去重 |

---

## 6.4 多设备同步注意事项

当用户同时登录多个设备（如手机 + 桌面）时，消息同步机制如下：

### 6.4.1 发信设备的推送行为

```
用户 A 在手机发送消息
  ├─ 手机收到 SERVER_ACK（上行确认）                    ← ✅ 正常
  ├─ 手机收不到 PUSH_MESSAGE（被 source_device_id 跳过） ← ✅ 不会重复
  ├─ A 的桌面端收到 PUSH_MESSAGE                         ← ✅ 实时同步
  └─ B 的所有设备收到 PUSH_MESSAGE                       ← ✅ 正常投递
```

服务端通过 `source_device_id` 跳过发信设备，避免一条消息被推回给发信的连接。

### 6.4.2 推送消息去重

所有设备收到 `PUSH_MESSAGE` 后统一以 **`message_id`** 为主键去重：

| 场景 | 去重方式 |
|------|---------|
| 收到别人发的消息 | `message_id` 去重 |
| 本设备发了消息 -> 收到自己的 `PUSH_MESSAGE` | `client_msg_id` 匹配本地 pending 记录，然后用 `message_id` 去重 |
| 其他设备发了消息 -> 本设备收到 `PUSH_MESSAGE` | 本地不存在该 `client_msg_id` → 正常追加，`status=received` |
| 历史拉取 + 在线推送的交集 | `message_id` 去重 |

### 6.4.3 client_msg_id 跨设备说明

`client_msg_id` 由**发信设备**在本地生成，其他设备不会匹配到该 `client_msg_id`：

- **发信设备**：本地有 `client_msg_id` 的 pending 记录 → 匹配后更新 `status=sent`
- **非发信设备**：本地不存在该 `client_msg_id` → 将消息作为新收到的消息插入本地，`status=received`
- 重连后拉历史时，`client_msg_id` 同步到所有设备的历史记录中，但不影响去重逻辑

### 6.4.4 已读回执跨设备共享

已读游标以 **`user_id`（而非 `device_id`）** 为维度，跨设备共享：

- 设备 A 读了一条消息 → 服务端更新该用户的已读游标
- 设备 B 实时收到 `PUSH_READ_RECEIPT`（含最新的 `last_read_message_id`）
- 设备 B 重连后拉历史时，`read_states` 直接反映最新游标
- 服务端使用 `GREATEST(existing, new)` 防倒退，不同设备并发读不会覆盖进度

### 6.4.5 在线状态跨设备

在线状态（presence）以 **`user_id`** 为维度：

- 任一设备在线 → 该用户标记为 `online`
- 设备 A 心跳正常、设备 B 断线 → 用户仍为 `online`
- 所有设备都离线后才标记 `offline`
- `FRAME_TYPE_PUSH_PRESENCE` 仅在 online↔offline 切换时推送

### 6.4.6 Token 过期影响

Token 过期是 per-connection 的，不影响其他设备：

- 设备 A 的 token 过期 → 收到 `FRAME_TYPE_TOKEN_EXPIRED` → 断开重连
- 设备 B 不受影响，继续正常收发
- 服务端 `KickUser{device_id="specific"}` 只踢指定设备，不影响其他连接

### 6.4.7 本地消息缓存要点

多设备场景下本地缓存需要额外注意：

| 字段 | 建议 |
|------|------|
| `created_locally` | 标记是否本设备创建，帮助 UI 区分来源
| `local_status` | 本设备创建的才有 `sending→sent→failed`；其他设备来的直接为 `received`
| `message_id` | 所有设备统一作为主键，不接受客户端生成

---

## 7. 客户端本地缓存策略

### 7.1 数据模型

推荐使用 SQLite 存储本地消息缓存：

```sql
-- 消息主表
CREATE TABLE local_messages (
    message_id INTEGER PRIMARY KEY,        -- 服务端 ID（主键）
    client_msg_id TEXT,                    -- 客户端 ID（可为 NULL）
    conversation_id INTEGER NOT NULL,
    sender_id INTEGER NOT NULL,
    sender_name TEXT,
    sender_email TEXT,
    message_type TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at INTEGER NOT NULL,           -- Unix milliseconds
    is_system INTEGER DEFAULT 0,
    mentions TEXT,                          -- JSON array
    local_status INTEGER DEFAULT 0,        -- 0=received, 1=sending, 2=sent, 3=failed
    created_locally BOOLEAN DEFAULT 0,     -- 是否本设备创建
    synced_at INTEGER                      -- 同步时间
);

CREATE INDEX idx_local_msgs_conv_time ON local_messages(conversation_id, created_at DESC, message_id DESC);
CREATE INDEX idx_local_msgs_client ON local_messages(client_msg_id) WHERE client_msg_id IS NOT NULL;

-- 已读状态表
CREATE TABLE local_read_states (
    conversation_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,              -- 当前登录者
    last_read_message_id INTEGER NOT NULL,  -- 最大已读 message_id
    updated_at INTEGER NOT NULL,            -- Unix milliseconds
    PRIMARY KEY (conversation_id)
);
```

### 7.2 消息生命周期

```
发送前                     发送中                     发送后
  │                          │                          │
  │ 生成 client_msg_id       │                          │
  │ 插入 local_messages      │                          │
  │ local_status=sending     │                          │
  │ message_id=NULL (临时)    │                          │
  ├───────────────────────►  │                          │
  │                          │ 发送 WS 帧               │
  │                          ├──────────────────────────┤
  │                          │                          │ 收到 SERVER_ACK
  │                          │                          │ local_status=sent
  │                          │                          │ message_id=10001
  │                          │                          │
  │                          │                          │ 收到 PUSH_MESSAGE
  │                          │                          │ 按 client_msg_id 匹配
  │                          │                          │ 更新 message_id（若 ACK 未到）
```

### 7.3 状态转换

| 状态 | 含义 | 触发条件 |
|------|------|---------|
| `sending` (1) | 本设备正在发送 | 用户点击发送后立即设置 |
| `sent` (2) | 发送成功 | 收到 `SERVER_ACK{status=ACCEPTED}` |
| `failed` (3) | 发送失败 | 收到 `SERVER_ACK{status=REJECTED}` 或 `RETRYABLE` 重试耗尽 |
| `received` (0) | 收到别人的消息 | 收到 `PUSH_MESSAGE` 且不是自己发的 |

### 7.4 序列号与已确认范围

客户端维护服务端帧的已确认范围：

```go
type SeqTracker struct {
    lastAckedSeq int64   // 已连续处理的最大白名单 pending 推送 seq；心跳 last_seq 应填写该值，不能越过空洞/非 pending 帧
}
```

gateway 会记录 `LastAckedSeq`，并在当前连接生命周期内维护短 TTL、有限容量的 L1 pending 重放窗口。客户端发送 `FRAME_TYPE_ACK.ack_seq` 或心跳 `HeartbeatPayload.last_seq` 都会推进服务端游标；该值必须是“已连续处理”的最大白名单 pending 推送 `seq`，不能用乱序收到的更大 `seq`、心跳 ACK 或控制帧 `seq` 越过空洞；心跳时服务端会在返回 `SERVER_ACK` 后补发 `seq > last_seq` 的白名单 pending 帧。断线/重连后仍通过 REST 拉取历史/快照补全。

### 7.5 服务端推送轻量 pending 队列

轻量 pending 用于降低在线抖动、UI 消费窗口或服务端 ACK/心跳补发中的短暂丢帧影响；它不是权威存储。客户端重连后仍必须按 §8.4 拉取历史、presence 快照、read states 和业务列表校准最终状态。

推荐白名单：

| 服务端帧 | 是否进入 pending | 客户端处理建议 |
|------|------|------|
| `FRAME_TYPE_PUSH_MESSAGE` | 是 | 按 `message_id` 去重；发送方可用 `client_msg_id` 合并本地 `sending` 记录。最终仍以 `GET /api/conversations/history/:id` 为准；附件解析更新只触发对应消息刷新，不覆盖历史权威结果。 |
| `FRAME_TYPE_PUSH_NOTIFICATION` | 是 | 短 TTL、低容量保存；用于提示系统公告/维护/强升级等。若后续有通知中心或配置快照接口，最终以 REST 快照为准。 |
| `FRAME_TYPE_PUSH_FRIEND_APPLICATION` | 是 | 按好友双方与 `updated_at` 合并最新状态；收到后可先更新入口红点，再调用 `GET /api/friends/applications` 或好友列表接口校准。 |
| `FRAME_TYPE_PUSH_READ_RECEIPT` | 是 | 按 `(conversation_id, user_id)` 合并，只保留最大 `last_read_message_id` 与最新 `updated_at`；最终以 history 响应中的 `read_states` / `read_details` 校准。 |

不进入 pending：

- `FRAME_TYPE_PUSH_TYPING`：瞬时 UI 信号，按 4s 左右超时自动清除，不重放。
- `FRAME_TYPE_PUSH_PRESENCE`：重连后直接拉 `GET /api/presence/friends` 快照覆盖本地状态。
- `FRAME_TYPE_RECONNECT`、`FRAME_TYPE_TOKEN_EXPIRED`：连接控制帧，由连接状态机、close code、token refresh 处理，重放没有意义。
- `FRAME_TYPE_SERVER_ACK`：只确认客户端上行帧，不进入服务端推送 pending；发送消息的本地 pending 由 `client_msg_id`、重试和 history 兜底。

gateway 已启用基于 `LastAckedSeq` / `HeartbeatPayload.last_seq` 的当前连接轻量补发：只对白名单帧启用，默认短 TTL、最大容量，禁止 typing/presence/control/ack 入队；服务端补发窗口按连接维护，只保存已分配正数 `seq` 且已成功写出的出站帧，不跨断线/重连持久化。客户端仍应保留本地 pending/去重逻辑，并以 history/快照作为最终状态来源。

---

## 8. 云端消息漫游

### 8.1 漫游原理

```
                        ┌─────────────────────────────┐
                        │   Server (PostgreSQL)        │
                        │   messages 表（永久存储）       │
                        │   conversation_read_states   │
                        └─────────────────────────────┘
                          │                    │
                    REST /history          WS PUSH_MESSAGE
                          │                    │
          ┌───────────────┼────────────────────┼───────────┐
          │               ▼                    ▼           │
          │    新设备登录    历史拉取         在线实时推送     │
          │    (首次加载)   (按需翻页)        (增量同步)     │
          └────────────────────────────────────────────────┘
```

### 8.2 新设备首次加载

```
1. 重连 WS                                              # 步骤 1：连接 /ws
   ├─ 登录 / refresh 获取 access_token
   └─ 连接 WS /ws
2. 拉会话列表                                          # 步骤 2
   └─ GET /api/conversations
3. 拉最近消息 / 未读消息                              # 步骤 3
   └─ 对每个会话：
       a. GET /api/conversations/history/:id?limit=50
       b. 将返回消息批量插入 local_messages（按 message_id 去重）
       c. 若 has_more=true，记录 next_cursor_* 备用
       d. 保留 read_states 响应给步骤 5 使用
4. 拉好友 presence 快照                                 # 步骤 4
   └─ GET /api/presence/friends
5. 更新 read state 快照                                  # 步骤 5
   └─ 将各会话的 read_states 写入 local_read_states
6. 开启 WS 消息接收循环
```

### 8.3 增量同步（上下翻页）

```
用户向上滚动（更早消息）：
  GET /api/conversations/history/:id
    ?cursor_created_at=<上一页最后一条的 created_at>
    &cursor_id=<上一页最后一条的 id>
    &limit=50

用户向下到底（最新消息流）：
  不需要主动拉取，WS 实时推送即可。

下拉刷新（异步确保无遗漏）：
  1. 取本地缓存中该会话最新的 message_id
  2. GET /history 不传 cursor（获取最新 N 条）
  3. 将返回结果与本地做 merge（按 message_id 去重）
```

### 8.4 离线恢复

```
检测到 WS 断线
  │
  ├─ 启动自动重连（指数退避，详见 §13.2）
  │
  └─ 重连成功后按以下顺序恢复：
      1. 重连 WS
         └─ 使用最新 access_token 重新握手 /ws；若收到 TOKEN_EXPIRED，
             先 POST /api/auth/refresh 再重连。
      2. 拉会话列表
         └─ GET /api/conversations
             覆盖断线期间新增/退出的会话，更新会话索引、群名/头像、成员列表。
      3. 拉最近消息 / 未读消息
         └─ 对每个活跃会话，以本地最新 message_id 作为锚点：
             GET /api/conversations/history/:id?limit=50（必要时翻页）
             → 将 message_id > 本地最新的消息插入 local_messages（去重合并）
             → 用于补齐断线期间漏掉的消息和未读角标
      4. 拉好友 presence 快照
         └─ GET /api/presence/friends
             覆盖断线期间错过的 PUSH_PRESENCE，恢复好友在线/离线显示。
      5. 更新 read state 快照
         └─ 取步骤 3 各会话 history 响应中的 read_states 字段，
             写入 local_read_states（GREATEST 取大，不允许倒退）。
             用于刷新已读标记、重新计算每个会话的未读数。
```

**实现要点**：

- 步骤 2~5 是恢复客户端权威状态的必要顺序，不要省略或调换：先有会话索引才知道要为哪些会话拉历史，先有最新消息才能正确计算未读数，最后用 read state 校准已读边界。
- 历史消息合并以 **`message_id` 为主键去重**；推送通道在重连完成前可能已经投递了部分新消息，本地缓存可能与 history 响应有交集，应直接以 `message_id` 覆盖/忽略，不要重复展示。
- `read_states` 来自 `GET /api/conversations/history/:id` 响应（详见 §8.2 步骤 3d 与 §9.3），客户端目前没有单独的 read-state 快照端点；如果某些会话不需要拉历史，可只调用 `history?limit=1` 等占位请求获取最新 `read_states`，或在用户进入该会话时再补齐。
- 未读角标 = `本地消息中 message_id > 自己的 last_read_message_id 的数量`。read state 与最近消息都更新后再统一刷新未读计数，避免中间态闪烁。
- 实时通道恢复后，`PUSH_MESSAGE` / `PUSH_NOTIFICATION` / `PUSH_FRIEND_APPLICATION` / `PUSH_READ_RECEIPT` 可按 §7.5 做轻量 pending 增量处理；`PUSH_PRESENCE` 不做 pending，步骤 4 的快照负责覆盖断线期间的 presence 差异。步骤 3~5 负责把断线期间的权威状态补齐。

### 8.5 跨设备同步

设备 A 发送消息后，设备 B 会通过 WS `PUSH_MESSAGE` 实时收到。若 B 离线，重连后通过以下方式同步：

1. **新消息**：重连后的增量拉取（§8.4）
2. **已读状态**：`conversation_read_states` 以 `user_id` 为维度，是服务端权威数据。B 重连后拉取历史时会附带 `read_states`
3. **已读回执推送**：A 读了某消息后，B 会通过 WS `PUSH_READ_RECEIPT` 收到实时更新

> **多设备场景的客户端细节请参考 §6.4 多设备同步注意事项**。
---

## 9. 已读回执

### 9.1 发送已读回执

```
客户端读取到某条消息后：

├─ 计算该会话中最大的已读 message_id
├─ 发送 WS 帧：
│    type = READ_RECEIPT
│    payload = ReadReceiptPayload{
│      conversation_id = <会话ID>,
│      last_msg_id = <当前已读到的最大 message_id>
│    }
└─ 等待 SERVER_ACK
     code=0 → 成功（当前 ACK status 可能为 UNSPECIFIED）
     code≠0 → 参考 §13 错误处理
```

**发送时机建议**：

- 用户进入会话时：发最后一次读取位置的 `last_msg_id`
- 用户滚动到新消息时：节流发送，建议每 1s 最多 1 次
- 不要在本地已经读过的消息上重复发送（`last_msg_id` 没有变化）

### 9.2 接收已读回执推送

```protobuf
message PushReadReceiptPayload {
  int64 conversation_id = 1;      // 会话 ID
  int64 user_id = 2;              // 推进游标的用户
  int64 last_read_message_id = 3; // 该用户已读到的最大消息 ID
  int64 updated_at = 4;           // 更新时间 Unix ms
}
```

收到后更新 local_read_states，并刷新 UI（如消息旁的已读标记）。

### 9.3 已读状态持久化（服务端）

服务端 `conversation_read_states` 表：

```sql
PRIMARY KEY (conversation_id, user_id)
last_read_message_id BIGINT      -- 单调递增
updated_at TIMESTAMPTZ           -- MONOTONIC 更新
```

关键特性：
- 以 `user_id`（非 `device_id`）为维度 → 跨设备共享
- `GREATEST(existing, new)` 防倒退 → 不同设备并发读不会覆盖进度

---

## 10. 在线状态与输入状态

### 10.1 心跳

每 20~30 秒发送一次心跳：

```
type = HEARTBEAT
payload = HeartbeatPayload{ last_seq = <客户端已连续处理的最大白名单 pending 推送 seq> }
```

- 收到 `SERVER_ACK` 表示心跳成功
- 心跳 `last_seq` 会触发服务端清理已确认 pending，并补发当前连接内 `seq > last_seq` 的白名单推送帧
- 如果连续 60s 未收到服务端任何消息，考虑重连
- 心跳同时续约 Redis presence TTL（服务端 45s TTL）

### 10.2 在线状态拉取

- 初始快照：`GET /api/presence/friends`
- 实时更新：接收 WS `PUSH_PRESENCE`

```protobuf
message PushPresencePayload {
  int64 user_id = 1;
  string status = 2;      // "online" | "offline"
  int64 updated_at = 3;   // Unix ms
}
```

### 10.3 输入状态

**发送输入状态**（节流 2.5s 一次）：

```
type = TYPING
payload = TypingPayload{ conversation_id = <会话ID> }
```

- 该帧不返回 `SERVER_ACK`
- 服务端 best-effort 投递，客户端不依赖强一致到达

**接收输入状态**：

```
PUSH_TYPING → { user_id, conversation_id }
```

- 按 `conversation_id` 隔离
- 如果 4s 内未再次收到同用户+会话的输入通知，清除 UI 状态

---

## 11. 附件消息

### 11.1 附件上传流程

```
客户端                      Gateway                    Attachment       SeaweedFS/S3
  │                           │                           │                │
  │ POST /api/attachments/init│                           │                │
  │ ─────────────────────────►│    InitUpload gRPC        │                │
  │                           │ ────────────────────────► │                │
  │                           │                           │  预签名 URL     │
  │                           │                           │ ──────────────►│
  │                           │   ◄── 返回 upload_url ───│                │
  │ ◄── upload_url, file_id ──│                           │                │
  │                           │                           │                │
  │ PUT/Direct upload ───────────────────────────────────────────────────►│
  │                           │                           │                │
  │ POST /api/attachments/:id/complete                    │                │
  │ ─────────────────────────►│    CompleteUpload gRPC    │                │
  │                           │ ────────────────────────► │  校验上传完整性  │
  │                           │   ◄── 附件元数据 ─────────│                │
  │ ◄── 附件信息 ──────────────│                           │                │
```

### 11.2 附件消息内容格式（aim.attachment.v1）

发送附件消息时，`SendMessagePayload.content` 必须是 JSON 字符串：

```json
{
  "schema": "aim.attachment.v1",
  "file_id": "att_abc123",
  "kind": "image",
  "original": {
    "name": "photo.jpg",
    "mime": "image/jpeg",
    "size": 2048576,
    "sha256": "a1b2c3..."
  },
  "thumbnail_file_id": "att_thumb_xyz",
  "parse_status": "ready",
  "duration_ms": 0,
  "width": 1920,
  "height": 1080,
  "metadata": {}
}
```

**kind 取值**：`image` | `video` | `audio` | `file`

客户端可按 `Content` 结构体（Go 参考 `app/shared/attachment/content.go`）在自己语言中实现解析。

### 11.3 附件下载

获取预签名下载 URL：

```
GET /api/attachments/:file_id/download
```

返回 `{ url, headers, expires_at }`，客户端直接用 `url` 下载二进制。
### 11.3 附件下载

获取预签名下载 URL：

```
GET /api/attachments/:file_id/download
```

返回 `{ url, headers, expires_at }`，客户端直接用 `url` 下载二进制。

### 11.4 附件解析状态更新推送

附件上传后，服务端异步解析媒体文件（图片生成缩略图、提取尺寸；视频/音频当前为占位解析）。

解析完成后，服务端通过 **`PUSH_MESSAGE` 帧** 主动推送更新后的附件内容到会话所有成员。

**帧特征**：

| 字段 | 值 | 说明 |
|------|------|------|
| `message_type` | `"image"` / `"video"` / `"audio"` | 与原始附件消息类型一致 |
| `is_system` | `true` | 标识为系统推送（非用户发送） |
| `sender_id` | `0` | 系统消息发送者 |
| `content` | 附件 JSON | 更新后的 `aim.attachment.v1` 内容 |
| `sender_info` | `{name:"system",...}` | 系统发送者信息 |

**content 示例（解析完成）**：

```json
{
  "schema": "aim.attachment.v1",
  "file_id": "att_abc123",
  "kind": "image",
  "original": {
    "name": "photo.jpg",
    "mime": "image/jpeg",
    "size": 2048576,
    "sha256": "a1b2c3..."
  },
  "thumbnail_file_id": "attachments/derived/att_abc123/thumbnail.png",
  "parse_status": "ready",
  "width": 1920,
  "height": 1080,
  "duration_ms": 0,
  "metadata": {"format": "png"}
}
```

**客户端处理**：

```
收到 PUSH_MESSAGE（message_type=image/video/audio, is_system=true）
  │
  ├─ 1. 解析 content 为 attachment JSON
  ├─ 2. 提取 file_id
  └─ 3. 在当前会话的本地消息缓存中查找相同 file_id 的附件消息
         │
         ├─ 找到 → 原地更新该消息的 content（替换为最新的 JSON）
         │         └─ UI 刷新：显示缩略图、尺寸等
         │
         └─ 未找到 → 忽略（附件可能在其他会话，或消息尚未拉取到本地）
```

**关键要点**：

- 附件解析更新以 `file_id` 为匹配键，不是 `message_id`
- 更新是幂等的：相同 `file_id` 多次推送，直接覆盖即可
- `parse_status` 可能为 `"ready"`（成功）或 `"failed"`（失败）；失败状态暂不推送，客户端可通过 `GET /api/attachments/:id` 轮询
- 如果 `parse_status=ready` 但 `thumbnail_file_id` 为空、`width`/`height` 为 0，表示解析生成了占位缩略图（不可解码格式或非图片媒体）
- 缩略图下载：使用 `thumbnail_file_id` 作为 object_key，格式固定为 PNG（最长边 512px）

## 12. 群聊与系统消息

### 12.1 群聊创建

```
POST /api/conversations/group
{
  "member_ids": [123, 456, 789],
  "name": "项目讨论组",     // 必填
  "avatar": "https://..."   // 可选
}
```

### 12.2 群管理系统消息

当群发生变更时，服务端推送系统消息（`is_system=true`, `sender_id=0`, `message_type="system"`）。

> **注意**：`PUSH_MESSAGE`（`is_system=true`）还用于附件解析状态更新（`message_type=image/video/audio`），详见 §11.4。

`content` 为 JSON 字符串，含事件类型：

```json
{
  "event": "member_joined",
  "operator_id": 123,
  "target_user_ids": [456, 789]
}
```

常见事件类型：

| 事件 | 含义 |
|------|------|
| `member_joined` | 成员加入（被邀请） |
| `member_left` | 成员退出 |
| `member_removed` | 成员被移除 |
| `group_renamed` | 群名变更 |
| `group_avatar_changed` | 群头像变更 |
| `group_dismissed` | 群被解散 |

### 12.3 群管理 REST 端点

| 方法 | 路径 | 用途 | 权限 |
|------|------|------|------|
| `GET` | `/api/conversations/:id/members` | 成员详情 | 群成员 |
| `POST` | `/api/conversations/:id/members` | 添加成员 | 群主/管理员 |
| `DELETE` | `/api/conversations/:id/members/:uid` | 移除成员 | 群主/管理员（admin 仅可移除普通成员） |
| `POST` | `/api/conversations/:id/members/:uid/admin` | 授予管理员 | 群主 |
| `DELETE` | `/api/conversations/:id/members/:uid/admin` | 撤销管理员 | 群主 |
| `POST` | `/api/conversations/:id/owner` | 转让群主 | 群主 |
| `POST` | `/api/conversations/:id/leave` | 退出群聊 | 群成员（群主需先转让或解散） |
| `DELETE` | `/api/conversations/:id` | 解散群聊 | 群主 |
| `PUT` | `/api/conversations/:id` | 更新群信息 | 群主/管理员 |

---

## 13. 错误处理与重试

### 13.1 REST 错误处理

```
HTTP 200 + code != 0  → 业务错误
HTTP 4xx             → 参数/认证错误
HTTP 5xx             → 服务器错误

// 标准处理
if response.code != 0 {
    switch response.code {
        case 40100:  // 尝试 refresh token
        case 42900:  // 退避重试 (1s, 2s, 4s...)
        case 40000, 40300, 40400, 40900: // 不可重试，提示用户
        case 50000:  // 退避重试
    }
}
```

### 13.2 WS 重连策略

```
断线检测：
  - 心跳超时 > 90s
  - WebSocket 连接关闭

重连策略（指数退避）：
  - 第 1 次：立即重连
  - 第 2 次：等 1s
  - 第 3 次：等 2s
  - 第 4 次：等 4s
  - 第 5 次+：等 10s（上限）
  - 重连成功 → 重置计数

重连后恢复（与 §8.4 一致）：
  1. 重连 WS（必要时先 refresh token）
  2. GET /api/conversations              # 拉会话列表
  3. GET /api/conversations/history/:id  # 拉最近/未读消息（以本地最新 message_id 为锚点）
  4. GET /api/presence/friends           # 拉好友 presence 快照
  5. 用 history 响应中的 read_states 更新 local_read_states  # read state 快照
```

### 13.3 消息发送失败处理

| 场景 | 行为 |
|------|------|
| WS 断线无法发送 | 保留 pending 状态，重连后自动补发 |
| `RETRYABLE` ACK | 退避重试（1s, 2s, 4s），相同 `client_msg_id`，最多 5 次 |
| `REJECTED` ACK | 标记 `failed`，不再重试，提示用户 |
| 重试耗尽 | 标记 `failed`，按钮变为"重发"，点击后重新生成 `client_msg_id` 发送 |

### 13.4 幂等保证

核心逻辑位于服务端 `TransferLogic`：

```
幂等键: idempotency:transfer:{sender_id}:{device_id}:{client_msg_id}
TTL: 24 小时

检查时机：消息到达 Transfer 的第一个步骤
命中 → 返回已有 message_id，不重新发布 Kafka

注意：
  - 幂等键以 (sender_id, device_id, client_msg_id) 三元组为范围
  - 不同设备、不同用户发的相同 client_msg_id 不会被去重
  - Redis 故障时幂等检查失败按正常消息处理（可能重复投递）
```

---

## 14. 完整生命周期示例

### 14.1 应用启动

```
1. 检查本地是否存储了 refresh_token
   ├─ 有 → POST /api/auth/refresh
   │         ├─ 成功 → 使用新 token
   │         └─ 失败 → 跳转登录页
   └─ 无 → 跳转登录页

2. 登录成功（首次加载，顺序与 §8.4 重连恢复一致）：
   ├─ 存储 access_token, refresh_token, user_id
   ├─ 建立 WS 连接 (GET /ws)                  # 步骤 1：重连 WS
   ├─ GET /api/conversations                  # 步骤 2：拉会话列表
   ├─ 对每个会话 GET /api/conversations/history/:id?limit=50  # 步骤 3：拉最近/未读消息
   │     → 插入 local_messages（按 message_id 去重）
   │     → 保留 read_states 响应供步骤 5 使用
   ├─ GET /api/presence/friends                # 步骤 4：拉好友 presence 快照
   └─ 将各会话的 read_states 写入 local_read_states  # 步骤 5：read state 快照

3. 进入消息监听循环：
   ├─ WS 帧处理循环
   ├─ 心跳定时器 (每 25s)
   └─ Token 刷新定时器 (过期前 60s)
```

### 14.2 发送文本消息

```
1. 生成 client_msg_id = "d1-" + UUID-v4
2. 本地插入：
   message_id=NULL, client_msg_id=..., status=sending
3. 构建 SendMessagePayload → WsFrame → 发送
4. 等待 SERVER_ACK：
   ├─ ACCEPTED → status=sent, 保存 message_id
   ├─ RETRYABLE → 退避重试
   └─ REJECTED → status=failed, 提示错误

5. 可能同时收到自己的 PUSH_MESSAGE（回声）：
   ├─ 按 client_msg_id 匹配 → 确认已存储
   └─ 若 ACK 未到 → 用推送的 message_id 更新
```

### 14.3 接收并显示消息

```
1. 收到 PUSH_MESSAGE
2. 按 message_id 查本地缓存
   ├─ 已存在 → 忽略
   └─ 不存在 → 生成新的本地记录，status=received
3. 判断是否为当前查看的会话
   ├─ 是 → 直接添加到 UI 底部
   └─ 否 → 更新 converse_list 的最后一条消息和未读计数
4. 若当前正在查看该会话 → 发送 READ_RECEIPT
```

### 14.4 应用进入后台 / 回到前台

```
进入后台：
  ├─ 不主动断连（后台 WS 连接视平台而定）
  └─ 暂停心跳（避免被踢）

回到前台：
  ├─ 检查 WS 连接是否存活
  │   ├─ 存活 → 恢复心跳，拉 presence 快照以补上漏掉的 PUSH_PRESENCE
  │   └─ 断开 → 按 §8.4 的重连恢复顺序执行：
  │            1) 重连 WS
  │            2) GET /api/conversations
  │            3) 对活跃会话 GET /api/conversations/history/:id（拉最近/未读）
  │            4) GET /api/presence/friends
  └─            5) 用 history.read_states 更新 local_read_states
```

### 14.5 应用退出

```
正常退出：
  1. POST /api/auth/logout（可选，清除服务端 session）
  2. 关闭 WS 连接
  3. 清除内存中的 token

异常退出（崩溃）：
  - 下次启动时 refresh_token 仍有效，自动恢复
  - 服务端在 presence TTL（45s）后自动标记离线
```

---

## 15. 附录：性能与安全建议

### 15.1 性能建议

| 项目 | 建议 |
|------|------|
| WS 帧大小 | 当前服务端限制 1024 bytes，超长消息应分片或使用附件 |
| 历史拉取页大小 | 默认 50，最大 100。首次加载按需，不要一次性拉全部 |
| 图片缓存 | 附件下载 URL 有时效（expires_at），缓存需考虑刷新 |
| 本地数据库 | 使用 WAL 模式 SQLite，禁止阻塞主线程 |
| 消息去重 | 使用 HashSet 索引已处理的 message_id（定期清理超过 7 天的 ID） |
| 输入状态节流 | 发送间隔 ≥ 2.5s |

### 15.2 安全建议

| 项目 | 建议 |
|------|------|
| Token 存储 | 使用 Keychain（iOS）/ Keystore（Android）/ 加密存储（Desktop/Web） |
| Token 传输 | 只通过 HTTPS/WSS 传输，不在 URL 参数中传递 |
| 日志脱敏 | 不在日志中打印 access_token, refresh_token, 或消息内容 |
| 证书固定 | 生产环境建议启用证书固定 |
| client_msg_id | 不使用可预测的算法（如递增数字），使用 UUID |
| 附件下载 | 验证下载 URL 的域名是否属于预期范围 |

### 15.3 兼容性要求

| 项目 | 要求 |
|------|------|
| Protobuf | proto3，必须生成对应语言的序列化代码 |
| 未知字段 | 忽略而不报错（向前兼容） |
| 新增帧类型 | 只追加编号，不复用历史编号 |
| `conversation_type` | 合法值 `direct` / `group`；可能为空字符串，客户端需兼容 |
| `mentions` | 使用十进制用户 ID 字符串（不用 int64），避免 JS 精度问题 |

### 15.4 WebSocket 关闭码参考

| Code | 含义 | 客户端行为 |
|------|------|-----------|
| 1000 | 正常关闭 | 不自动重连 |
| 1008 (Policy Violation) | Token 过期 / 重复登录 / 权限变更 | Token 过期 → refresh 后重连；重复登录 → 提示覆盖 |

### 15.5 配置常量

| 常量 | 值 | 说明 |
|------|------|------|
| Access Token TTL | 5 min | 服务端 JWT 过期时间 |
| Refresh Token TTL | 7 days | 服务端 refresh token 过期时间 |
| WS 读取限制 | 1024 bytes | 服务端帧大小上限 |
| 写帧超时 | 5s | 服务端写超时 |
| Presence TTL | 45s | Redis 在线状态 TTL |
| Stale 连接扫描 | 60s | 服务端清理无心跳连接 |
| Token 过期宽限 | 30s | 重连窗口期 |
| 幂等键 TTL | 24h | 相同 client_msg_id 的去重窗口 |
| 建议心跳间隔 | 20~30s | 客户端心跳频率 |
