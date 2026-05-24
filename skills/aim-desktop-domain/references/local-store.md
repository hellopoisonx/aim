# Desktop 本地存储

## 根目录

Desktop 使用系统用户配置目录下的 `aim-desktop` 作为根目录：

```text
<UserConfigDir>/aim-desktop/
├── config.json
└── accounts/
    └── {user_id}/
        ├── cache.db
        ├── cache.db-wal
        └── cache.db-shm
```

若无法获取系统用户配置目录，回退到当前工作目录下 `.aim-desktop`。

## `config.json`

`config.json` 保存全局 Gateway 配置和账号列表：

```json
{
  "gateway_url": "http://localhost:8888",
  "ws_url": "ws://localhost:8888/ws",
  "active_user_id": 42,
  "accounts": [
    {
      "device_id": "uuid",
      "access_token": "...",
      "refresh_token": "...",
      "expires_at": 1700000000,
      "user": {
        "user_id": 42,
        "email": "a@example.com",
        "nickname": "Alice",
        "avatar": ""
      },
      "updated_at": 1700000000000
    }
  ]
}
```

规则：

- `gateway_url` / `ws_url` 为空时使用默认值。
- `accounts[]` 按 `user.user_id` 去重。
- `device_id` 是账号在本机的设备标识，首次注册/登录生成，后续复用。
- `active_user_id` 指向当前账号；若失效则归零并回退到第一个账号。
- Token 当前明文保存在本机配置文件中；生产安全加固应接入系统 keychain/credential vault。

## 旧配置迁移

`internal/store/config.go` 仍能读取旧版单账号字段：

- `device_id`
- `access_token`
- `refresh_token`
- `expires_at`
- `user`

加载时会迁移为 `accounts[0]`，并在内存中设置 `LegacyCacheUserID`。首次打开该账号 DB 时，`copyLegacyCacheIfNeeded()` 会将旧根目录 `cache.db`、`cache.db-wal`、`cache.db-shm` 复制到 `accounts/{user_id}/`，随后清除迁移标记。

## SQLite

每个账号独立打开一个 `cache.db`。`store.Open(dir)`：

- 创建账号目录。
- 打开 `cache.db`。
- 执行迁移 DDL。
- 启用 WAL：`pragma journal_mode=WAL`。

### 表

| 表 | 用途 |
|---|---|
| `conversations` | 会话列表缓存，包含成员、群资料、展示名等原始 JSON。 |
| `messages` | 消息缓存、本机 pending 消息、WS 推送与历史消息；按 `message_id` 或 `(conversation_id, client_msg_id)` 去重。 |
| `conversation_sync` | 每个会话的本地游标和是否还有更早历史。 |
| `friends` | 好友和好友申请缓存。 |
| `members` | 群成员/会话成员缓存。 |
| `read_states` | 已读游标缓存（当前 Go 侧主要由历史响应和事件向前端提供）。 |
| `presence` | 好友在线状态缓存。 |

### 幂等策略

消息写入使用两类唯一约束：

- `message_id unique`：服务端已确认消息、历史分页、WS 推送重复到达时更新同一行。
- `unique(conversation_id, client_msg_id)`：本机 pending 消息收到 ACK 或后续推送时回填，不重复插入。

`UpsertMessages()` 同时推进 `conversation_sync`：

- `min_message_id`：已知最早服务端消息。
- `max_message_id`：已知最新服务端消息。
- `max_created_at`：已知最新创建时间。
- `last_synced_at`：本地同步时间。
- `has_more_before`：历史分页到头后由 `MarkNoMoreBefore()` 置为 0。

## 多账号隔离规则

- 切换账号时只能打开目标账号 DB。
- 当前 `App` 的 `db` 指针永远代表活跃账号。
- WS 推送写库前必须经过 `handleFrameFor(userID, frame)` 的活跃账号校验。
- 不同账号缓存目录不得共用，避免会话、消息、好友、Token 混写。

## 反模式

- 不要把服务端权威逻辑（权限、成员关系、限流）放入本地缓存判断。
- 不要修改已有账号的 `device_id`，否则服务端多设备会话管理会出现异常。
- 不要在迁移旧缓存时移动原文件；当前策略是复制，降低升级风险。
- 不要手动编辑 SQLite 里的 `raw_json` 后期待展示层自动兼容所有字段变化。
