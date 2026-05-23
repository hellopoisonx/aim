# Bot OpenAPI 运维 Provision

V0 阶段没有用户侧的 Bot 管理 REST 接口，Bot 身份由运维人员手工创建。
本目录提供 `provision_bot.sh`：纯 SQL + `openssl`，一键写入 `aim_auth` / `aim_logic`、签发 token，适用于生产、CI 和最小依赖环境。

> [`provision_bot.sql`](provision_bot.sql) 是底层 SQL 模板。当前 `dev-tool/aim_test.py` 只提供 Bot OpenAPI 调试命令，不提供 provision 子命令；本地联调也请使用本目录的 shell 脚本。

## 必要信息

执行前请准备：

- `BOT_USER_ID`：bot 的雪花 ID。生产环境推荐使用 logic.rpc 的 ID 生成器；
  开发环境可以用 `python -c "import time;print(int(time.time()*1000))"` 凑一个不冲突的 64 位整数。
- `BOT_EMAIL`：占位邮箱（不会用于登录）。约定写 `<nickname>@bots.aim`，
  避免和真人账号撞库。
- `BOT_NICKNAME`：群成员列表里展示的昵称。
- `CONVERSATION_IDS`：要预装这个 bot 的群会话 ID 列表（可空）。
- `TOKEN_NAME`：token 备注名，便于日后撤销。
- `TOKEN_SCOPES`：以逗号分隔的 action grant 列表；旧 scope 名称（如 `messages:send`）不再支持。默认建议 `bot.message.send,bot.conversation.list,bot.self.read`。Webhook 管理还需要 `bot.webhook.read,bot.webhook.write,bot.webhook.delete,bot.webhook.subscribe.message_created`。

PostgreSQL 连接：
- `aim_auth` 与 `aim_logic` 默认共用同一个 PostgreSQL 实例，两个 DB。
- docker-compose 里的连接串：`postgresql://user:password@localhost:5432/aim_auth`、
  `postgresql://user:password@localhost:5432/aim_logic`。

## 操作步骤

```bash
cd scripts/bot-provision
chmod +x provision_bot.sh
./provision_bot.sh \
  --bot-user-id 9000000001 \
  --bot-email broadcast@bots.aim \
  --bot-nickname broadcast-bot \
  --conversation-ids "1,2,3" \
  --token-name "default" \
  --token-scopes "bot.message.send,bot.conversation.list,bot.self.read" \
  --auth-dsn "postgresql://user:password@localhost:5432/aim_auth" \
  --logic-dsn "postgresql://user:password@localhost:5432/aim_logic"
```

成功后脚本会输出一行 `aim_bot_...` 明文 token，**且仅输出这一次**：

```
=== AIM Bot Provisioned ===
bot_user_id : 9000000001
nickname    : broadcast-bot
plaintext   : aim_bot_9d4a... (store this NOW; it is unrecoverable)
```

## 撤销 / 禁用

- 单个 token 撤销：直接 `UPDATE bot_tokens SET revoked_at = NOW() WHERE id = $1;`。
- 全 bot 禁用：`UPDATE user_info SET status = 0 WHERE id = $bot_user_id;`，
  之后所有该 bot 的 token 都会被 `BotAuth` 中间件拒绝（`CodeBotDisabled`）。
- 退群：`DELETE FROM conversation_members WHERE conversation_id = ? AND user_id = ?;`。

## 注意事项

- Auth 侧的 `password_hash` 字段是占位值（一个无效 bcrypt 字符串），bot 不会
  也不应该走 `auth.Login`。如果你想绝对禁止 bot 登录，可以同时执行
  `UPDATE user_credentials SET status = 0 WHERE id = $bot_user_id;`。
- token 明文 **只在 provision 时生成一次**，丢失只能撤销重发。
- 跳过了 `aim.user.events` Kafka 事件，所以必须让脚本同时写 auth 和 logic
  两侧；否则 logic 这边会缺失 `user_info` 行，导致后续 RPC 找不到该 bot。
