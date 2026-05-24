# Desktop 多账号登录与缓存隔离

Wails 桌面客户端支持在同一设备上保存并切换多个账号，每个账号的数据（SQLite 缓存）完全隔离。

## 设计要点

### 账号存储（`config.json`）

- Gateway 地址等全局设置保留在根字段。
- 账号级信息（token、`device_id`、用户资料）保存在 `accounts[]` 数组。
- 单例 `active_user_id` 标记当前活跃账号。

```json
{
  "gateway_url": "http://localhost:8888",
  "ws_url": "ws://localhost:8888/ws",
  "active_user_id": 42,
  "accounts": [
    {
      "device_id": "desktop-xxx",
      "access_token": "...",
      "refresh_token": "...",
      "expires_at": 1700000000,
      "user": { "user_id": 42, "email": "a@example.com" },
      "updated_at": 1700000000000
    }
  ]
}
```

### 缓存隔离

每个账号的 SQLite 文件存储在独立目录：

```
~/.config/aim-desktop/
├── config.json
└── accounts/
    ├── 42/
    │   ├── cache.db
    │   ├── cache.db-wal
    │   └── cache.db-shm
    └── 99/
        ├── cache.db
        ├── cache.db-wal
        └── cache.db-shm
```

### 切换流程

1. `SwitchAccount(userID)` 被调用
2. 保存 `cfg.ActiveUserID`
3. `resetRuntimeLocked()`：断开旧 WS、关闭旧 DB、emit `ws:connection {connected: false}`
4. `openActiveDB()`：打开目标账号的 SQLite
5. 若 token 过期则自动刷新
6. `connectWSAsyncLocked()`：异步建立新 WS 连接

### WS 事件隔离

- `prepareWSLocked()` 创建 ws client 时闭包捕获 `userID`
- `handleFrameFor(userID, frame)` → `activeRuntimeForUser(userID)` 检查 `a.cfg.ActiveUserID == userID`
- 若不匹配直接丢弃；匹配后返回当前 `db` 与 `ws` 指针供事件处理使用
- 避免旧连接的回掉写入新账号的 DB

### 旧版升级（无感迁移）

首次启动检测内存字段 `LegacyCacheUserID`（来自旧版 `config.json` 的 `user.user_id`），若数据库目录不存在 `accounts/{id}/cache.db` 则将 `cache.db` + `-wal` + `-shm` 复制过去，此后不再复制。

## 新增/修改的 Go API

| 方法 | 说明 |
|---|---|
| `ListAccounts() []AccountView` | 返回所有已保存账号的视图 |
| `SwitchAccount(userID string) SessionInfo` | 切换到指定账号（切换 WS + DB） |
| `Logout()` | 清除当前账号 token，保持在账号列表 |
| `Login(input) SessionInfo` | 登录后将账号 upsert 到列表并设为活跃 |

## 前端账号管理

- 头部 + 下拉选择器 (`a-select`) 列出所有保存的账号，可即时切换
- "添加账号"按钮：显示登录/注册面板，不影响已有账号
- "退出当前账号"：清除 token 但保留账号记录，下次可快捷切换
- 登录面板底部可看到本机账号列表，直接点击切换

## 反模式

- **不要在切换账号时调用 `Logout()` 接口**——`SwitchAccount` 不执行服务端登出，仅切本地上下文。
- **不要在同一进程内打开多个账号的 DB**——`openActiveDB()` 总是先关闭当前 DB 再打开目标 DB。
- **不要修改 `AccountProfile` 的 `device_id`**——同一账号在同一设备上应使用固定的 `device_id`，由首次注册/登录时生成并持久化。
