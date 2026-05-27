---
name: aim-bot-domain
description: aim 的 Bot OpenAPI 域。覆盖第三方 Bot 接入、token 鉴权、Webhook 投递与运维 provision。
---
# aim-bot-domain

## 使用范围

当需求涉及以下任一项时使用本 Skill：

- 第三方 Bot 通过 `Authorization: Bot <token>` 调 `/api/bot/v1/*` REST 接口
- Bot Token 的生成、撤销、action 权限校验
- Bot Webhook（`message.created`）的订阅、HMAC 签名、重试与防回调循环
- 运维侧批量 provision Bot 身份（`auth + logic` 双写、入群、签发 token）
- 涉及 `user_info.user_type='bot'`、`bot_tokens`、`bot_actions`、`bot_token_permissions`、`bot_event_actions`、`bot_webhooks` 等表

## 设计原则（V0）

- **Bot = 特殊用户**：Bot 拥有真实 `user_id`、`user_info` 资料、`conversation_members`
  身份；与人类用户共用主消息链路（core.Transfer → Kafka `aim-message-transfer`
  → WebSocket 推送 + DB 归档）。
- **Token 哈希存储**：plaintext 仅在 provision 时输出一次，服务端只持有 SHA-256
  哈希，撤销靠 `revoked_at` 时间戳。
- **运维 provision**：V0 不暴露用户侧 Bot 创建 / 安装 REST；脚本一次性写入两个 DB。
- **Webhook 异步**：独立 Kafka consumer group `aim-logic-bot-webhook`，与归档 offset
  解耦，单 Bot 故障不会拖累整体消息持久化。

## 模块边界

- 鉴权与请求处理：`app/gateway/api/internal/middleware/botauth_middleware.go`
  + `app/gateway/api/internal/handler/bot/`、`logic/bot/`。
- Token / Webhook 业务：`app/logic/rpc/internal/service/bot_service.go`，
  通过 `BotService` gRPC 暴露给 gateway。
- Webhook 投递：`app/logic/rpc/internal/mqs/bot_webhook_consumer.go`，独立消费组。
- 公用工具：`app/shared/bottoken`（token / secret 生成、SHA-256 哈希、constant-time 比较）。
- Bot 上下文：`app/gateway/api/internal/botctx`（context-stored `BotIdentity`）。

## 数据模型

| 表 | 关键字段 | 说明 |
|----|----------|------|
| `user_info.user_type` | `human` / `bot` / `system`，默认 `human` | 区分身份类型；migration 006 |
| `bot_tokens` | `id`, `bot_user_id`, `token_hash`, `expires_at`, `revoked_at` | migration 007；`scopes` 字段保留但不再作为鉴权来源 |
| `bot_actions` | `id`, `action`, `enabled` | migration 009；运行时可调整的 action 字典 |
| `bot_token_permissions` | `token_id`, `action_id` | migration 009；token 授权的 action_id 列表 |
| `bot_event_actions` | `event`, `action_id`, `enabled` | migration 009；webhook event → 订阅 action 映射 |
| `bot_webhooks` | `bot_user_id` PK, `url`, `secret_hash`, `events`, `enabled` | migration 008；一个 bot 一个 webhook |

## 接入流程

1. 运维执行 `scripts/bot-provision/provision_bot.sh`：写入 `aim_auth.user_credentials`
   + `aim_logic.user_info(user_type='bot')`，按需 `conversation_members` 入群，
   并签发首个 token。
2. 运维把 plaintext token 通过安全渠道交给 Bot 开发者。
3. Bot 服务用 `Authorization: Bot <token>` 调 `/api/bot/v1/messages` 发消息；
   通过 `GET /api/bot/v1/conversations/:id/history` 读历史、
   `GET /api/bot/v1/conversations/:id/members` 读成员、
   `POST /api/bot/v1/conversations/:id/read-receipt` 上报已读，
   `GET /api/bot/v1/conversations/:id/read-states` 读取已读状态。
4. Go 业务可导入 `github.com/hellopoisonx/aim/bot_sdk`（包名 `botsdk`）使用
   REST Client、rotate-secret Webhook 验签与异步消息处理器。
5. 第三方 HTTP 服务在 `X-AIM-Signature` 校验通过后处理 `message.created` 事件。

## 错误码

| code | 含义 |
|------|------|
| 40110 | Bot token 缺失/格式错误/未知（`CodeBotTokenInvalid`） |
| 40111 | Bot token 已撤销 / 已过期（`CodeBotTokenRevoked`） |
| 40112 | Bot 用户被禁用（`CodeBotDisabled`） |
| 40310 | Token action 不足（`CodeBotScopeDenied`） |
| 40010 | Webhook 配置非法（`CodeBotWebhookInvalid`） |

详见 `app/shared/errorx/errorx.go`。

## 参考资料

- 开发者指南：`docs/bot-developer-guide.md`
- OpenAPI 规范：`docs/api/gateway-openapi.yaml`（Bot 路径前缀 `/api/bot/v1/`）
- 运维 provision：`scripts/bot-provision/README.md`
- 历史规划：`PLAN_BOT_ABILITY.md`（V0 之外的扩展计划）
- Go SDK：`bot_sdk/`

## 反模式

- 不要在 gateway 直接读 `bot_tokens` 表 —— 一律通过 logic 的 `BotService`，
  方便后续把校验逻辑（包括 action、群成员、群内权限）集中到 logic 层。
- 不要为 Bot 增设独立的消息热路径 —— 复用 `core.Transfer`，配额、限流、
  幂等性沿用现有机制；只是 `device_id` 固定为 `bot-api` 以便区分。
- 不要在 webhook 投递路径里阻塞归档 consumer —— 这两个消费者已经是不同的
  consumer group，单 Bot 故障不应回流到 archive。
- Webhook 失败不要把错误返回给 kq —— 当前实现内部重试 5 次后吞掉，offset 前进，
  避免单 bot 死信拖死消费者。
- 不要让 `user_info.user_type` 对应的 enum 被人手 SET 成奇怪值 —— V0 仅允许
  `human`/`bot`/`system`；新增类型需先扩展 `app/logic/rpc/internal/service/bot_service.go`
  以及 `BotAuth` 校验逻辑。

## 最近变更

- 2026-05-27: `bot_sdk` 新增 `integration` build tag 集成测试环境，使用内存 Gateway 覆盖 REST Client、APIError、Webhook 投递到 `AsyncProcessor` 的端到端流程。
- 2026-05-27: Bot direct 会话不再消耗/触发非好友临时会话累计消息上限；`DatabasePermissionChecker` 识别发送者或对端为 `user_type='bot'` 后跳过限额，但仍保留 direct 成员校验与 block 拦截。
- 2026-05-27: 补全 Bot 会话读侧接口（历史消息、成员详情、已读上报/读取状态），新增 `bot.conversation.*`、`bot.read_receipt.*` action，并新增 Go SDK `bot_sdk`（REST Client、rotate-secret Webhook 验签、异步处理器）。
- 2026-05-23: 新增基于 action 的 Bot 权限系统。新增 migration 009（`bot_actions`、`bot_token_permissions`、`bot_event_actions`），`ValidateBotToken` 从 action 关联表加载权限，Gateway Bot API 全量校验 action，webhook event 通过 `bot_event_actions` 映射订阅 action。
- 2026-05-23: V0 落地。新增 migrations 006-008、`BotService` RPC、`BotAuth`
  middleware、`/api/bot/v1/*` REST、`BotWebhookConsumer`（HMAC 签名 + 指数退避）、
  运维 provision 脚本、dev-tool `bot-*` 子命令、Swagger 与开发者指南。
