# 前端协议覆盖矩阵

## REST 端点（14 个）

| # | Method | Path | Auth | 客户端方法 | 状态 |
|---|---|---|---|---|---|
| 1 | POST | `/api/auth/register` | 无 | `Login()` 内部调用 | ✅ |
| 2 | POST | `/api/auth/login` | 无 | `Login()` | ✅ |
| 3 | POST | `/api/auth/refresh` | 无 | `Refresh()` | ✅ |
| 4 | POST | `/api/auth/logout` | Bearer | `Logout()` | ✅ |
| 5 | GET | `/api/users/by-name/{name}` | Bearer | `SearchUsersByName()` | ✅ |
| 6 | GET | `/api/users/by-id/{id}` | Bearer | `GetUserById()` | ✅ |
| 7 | POST | `/api/users/friends/{id}` | Bearer | 在 Vue `FriendsView` 中通过 Wails 调用 | ✅ |
| 8 | POST | `/api/friends/accept/{id}` | Bearer | 在 Vue `FriendsView` 中通过 Wails 调用 | ✅ |
| 9 | POST | `/api/friends/reject/{id}` | Bearer | 在 Vue `FriendsView` 中通过 Wails 调用 | ✅ |
| 10 | GET | `/api/friends/applications` | Bearer | `ListFriendApplications()` | ✅ |
| 11 | GET | `/api/friends/me` | Bearer | `ListFriends()` | ✅ |
| 12 | POST | `/api/conversations` | Bearer | `CreateConversation()` / `CreateDirectConversation()` | ✅ |
| 13 | GET | `/api/conversations` | Bearer | `ListConversations()` | ✅ |
| 14 | GET | `/api/conversations/history/{id}` | Bearer | `GetConversationHistory()` | ✅ |

## WebSocket 升级端点（1 个）

| 端点 | Auth | 客户端方法 |
|---|---|---|
| `GET /ws` | `Authorization: Bearer <access_token>` | `ConnectWS()` |

## WebSocket 帧：Client → Gateway（5 种）

| # | FrameType | 值 | Payload | 发送方法 | 说明 |
|---|---|---|---|---|---|
| 1 | `SEND_MESSAGE` | 1 | `SendMessagePayload` | `SendMessage()` | 发送消息 |
| 2 | `HEARTBEAT` | 2 | `HeartbeatPayload` | `SendHeartbeat()` | 30 秒间隔心跳 |
| 3 | `TYPING` | 3 | `TypingPayload` | `SendTyping()` | 输入状态提示 |
| 4 | `READ_RECEIPT` | 4 | `ReadReceiptPayload` | `SendReadReceipt()` | 已读回执 |
| 5 | `ACK` | 5 | `ClientAckPayload` | `SendAck()` | 帧确认 |

## WebSocket 帧：Gateway → Client（8 种）

| # | FrameType | 值 | Payload | Vue 分发 | 说明 |
|---|---|---|---|---|---|
| 1 | `PUSH_MESSAGE` | 101 | `PushMessagePayload` | 替换乐观消息、插入会话、发 ACK | 消息推送 |
| 2 | `PUSH_PRESENCE` | 102 | `PushPresencePayload` | 更新 `onlineUserIds` | 在线状态变更 |
| 3 | `PUSH_NOTIFICATION` | 103 | `PushNotificationPayload` | `ElMessage.info()` 显示 | 系统通知 |
| 4 | `PUSH_TYPING` | 104 | `TypingPayload` | 4 秒后自动清除 | 输入状态提示 |
| 5 | `RECONNECT` | 105 | `ReconnectPayload` | 设置 `connecting`，延迟重连 | 服务端要求重连 |
| 6 | `SERVER_ACK` | 106 | `ServerAckPayload` | 更新消息 `ackStatus` | 服务端确认 |
| 7 | `TOKEN_EXPIRED` | 107 | `TokenExpiredPayload` | 尝试 Refresh → 重连 WS 或退出登录 | token 过期 |
| 8 | `PUSH_FRIEND_APPLICATION` | 108 | `PushFriendApplicationPayload` | 按 status 显示不同 `ElMessage` | 好友申请推送 |

## 覆盖统计

| 类别 | 总数 | 已覆盖 |
|---|---|---|
| REST 端点 | 14 | 14 |
| WS 升级 | 1 | 1 |
| Client→Gateway 帧 | 5 | 5 |
| Gateway→Client 帧 | 8 | 8 |
| **合计** | **28** | **28（100%）** |

## 维护说明

- 新增 REST 端点时同步更新 `app.go` 中 `ProtocolCatalog` 的 `REST` 数组、本矩阵以及 `rest-client-pattern.md`。
- 新增帧类型时同步更新 `ProtocolCatalog` 的 `Frames` 数组、本矩阵以及 `ws-frame-handling.md`。
- 测试中 `len(catalog.REST)` 和 `len(catalog.Frames)` 的期望值必须与矩阵保持一致。