# 登录与会话加载流程

## 概述

登录成功后立即加载服务端已有会话列表，完成后端数据与客户端状态同步。

## 触发点

Vue `handleLogin()` 在以下步骤之后触发会话加载：

1. `Login()` → 设置 `currentUserId` / `currentUserLabel`
2. 清空本地状态（`conversations`、`messagesMap`、`historyLoadedSet`、`onlineUserIds` 等）
3. `ConnectWS()` → 建立 WebSocket 连接
4. **`ListConversations()` → 加载服务端会话列表**（这是本轮新增的步骤）
5. 对每个返回的会话调用 `loadConversationHistory(conv.id)` 预拉最新消息

## 数据流

```
Vue handleLogin()
  └─ Login(email, password, device_id)           // Wails bindings → App.Login()
  └─ ConnectWS()                                  // Wails bindings → App.ConnectWS()
  └─ ListConversations()                          // Wails bindings → App.ListConversations()
       └─ restClient.ListConversations(ctx, token) // HTTP GET /api/conversations
            └─ Gateway Auth 中间件验签
            └─ LogicRpc.GetUserConversations(userId)
            └─ 返回 []ConversationItem
  └─ for each item:
       └─ 解析成员 ID → 通过 GetUserById(otherId) 获取昵称和头像
       └─ 构建 Conversation 对象（title、avatar、memberIds）
       └─ loadConversationHistory(conv.id)        // 预拉消息
            └─ GetConversationHistory(convId, 0, 0, 50)  // HTTP GET /api/conversations/history/{id}
```

## 状态快照

登录过程中的状态可能包括：
- 会话列表：服务端已有对话 + 成员信息
- 历史消息：每个会话最新的 50 条消息
- 在线状态：初始为 `Set<number>()`（空），后续通过 WS `PUSH_PRESENCE` 填充

## 关键约束

- 会话加载是**非关键路径**——失败时不影响登录流程，仅显示空会话列表
- 隐藏的空会话由 `historyLoadedSet` 管理（`Set<conversationId>`），避免反复请求空历史
- `messagesMap` 键为 `conversation_id`，值为 `ChatMessage[]`
- 会话标题从对方（直聊）的邮箱前缀提取：`email.split('@')[0]`
- 加载历史时跳过已有消息的会话（`messagesMap.get(convId)?.length > 0`）
- 每条 `ChatMessage` 包含 `clientMsgId` 用于去重和乐观消息替换

## 历史消息游标分页

`getConversationHistory` 返回的响应包含分页游标，首次加载和后续滚动加载使用相同的游标字段：

```json
{
  "messages": [...],
  "next_cursor_created_at": 1715679000000,
  "next_cursor_id": 12345,
  "has_more": true
}
```

- 首次加载（`loadConversationHistory`）：`GetConversationHistory(convId, 0, 0, 50)`
- 加载更多（`handleLoadMore`）：`GetConversationHistory(convId, cursorCreatedAt, cursorId, 50)`
- `has_more === false` 时停止加载
- 游标信息存储在 `Conversation.historyCursor`，在 `loadConversationHistory` 返回后更新

## 相关的 DTO

```go
// client/ConversationItem — 对应 gateway 返回的单个会话
type ConversationItem struct {
    ConversationID   int64   `json:"conversation_id"`
    ConversationType string  `json:"conversation_type"`
    IsActive         bool    `json:"is_active"`
    CreatedAt        int64   `json:"created_at"`
    MemberIDs        []int64 `json:"member_ids"`
}

// client/ListConversationsResponse — 会话列表包裹
type ListConversationsResponse struct {
    Conversations []ConversationItem `json:"conversations"`
}
```

## 反模式

- 不要在登录成功之前调用 `ListConversations()`（没有 token）
- 不要将客户端本地生成的假会话数据混入服务端返回的 `ConversationItem` 列表
- 不要在每次会话切换时重新拉全量列表；`handleLogin` 只拉一次
- 加载更多历史时不要重置 `historyLoadedSet`；它用于跳过空历史会话的重复请求
- 不要在 `load-more` 处理中滚动到底部；使用 `isLoadingMore` 标记阻止自动滚动
- 不要在每次 `load-more` 时覆盖原有 `historyCursor`；只在首次加载后保存游标
