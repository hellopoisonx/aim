# AIM Bot 开放能力落地计划

## Context
本次需求为 AIM 对外提供 Bot 开放能力，采用「Bot 是特殊普通用户」的设计原则：
- 复用现有用户、会话、消息、权限主链路，不破坏已有业务逻辑
- Bot 拥有统一 `user_id`、用户资料、群成员身份，和普通用户体验一致
- 额外提供 Bot Token、Webhook 事件订阅、开发者管理能力
- 目标 V0 版本支持：创建 Bot、安装到群、API 发消息、Webhook 收群消息

## Approach
1. **最小侵入原则**：只新增 Bot 专属扩展表，不修改现有核心表结构语义
2. **分层落地**：分 4 个阶段逐步实现，每个阶段都可独立上线验证
3. **异步优先**：Webhook 事件投递完全异步，不阻塞主消息发送链路
4. **权限明确**：Bot Token 有独立 scope，群内 Bot 有独立权限配置
5. **兼容现有**：消息、会话、成员接口完全兼容现有协议，前端只需新增 Bot 标识展示

## Files to modify
### 数据库迁移
- `migrations/005_bot_user_type.sql`：users 表新增 user_type 字段
- `migrations/006_bot_extension.sql`：新增 bots 扩展表
- `migrations/007_bot_tokens.sql`：新增 bot_tokens 表
- `migrations/008_conversation_bot_settings.sql`：新增会话 Bot 配置表
- `migrations/009_bot_webhooks.sql`：新增 Bot Webhook 配置表

### shared 层
- `shared/proto/logic.proto`：新增 Bot 相关 RPC 定义
- `app/gateway/api/gateway.api`：新增用户侧 Bot 管理、Bot 自调用接口
- `app/shared/constants`：新增 Bot 常量、user_type 枚举、权限枚举
- `app/shared/token`：新增 BotToken 生成、校验、哈希工具
- `app/shared/errorx`：新增 Bot 相关错误码

### gateway 层
- `app/gateway/api/internal/middleware/bot_auth.go`：新增 BotToken 鉴权中间件
- `app/gateway/api/internal/handler/bots/`：用户侧 Bot 管理 handler
- `app/gateway/api/internal/handler/conversations/bots.go`：会话 Bot 管理 handler
- `app/gateway/api/internal/handler/bot/`：Bot 自调用接口 handler
- `app/gateway/api/internal/logic/bots/`：用户侧 Bot 管理逻辑
- `app/gateway/api/internal/logic/bot/`：Bot 自调用逻辑

### logic 层
- `app/logic/rpc/internal/logic/bots/`：Bot 核心 CRUD、Token、Webhook 逻辑
- `app/logic/rpc/internal/consumer/bot_event_consumer.go`：Bot 事件消费、Webhook 投递逻辑
- `app/logic/rpc/internal/service/bot_service.go`：Bot RPC 服务实现
- `app/logic/rpc/internal/config/config.go`：新增 Bot 限流、Webhook 配置

## Reuse
本次实现大量复用现有 AIM 能力，避免重复开发：
1. **用户系统**：复用 `users` 表、用户资料查询、用户状态逻辑
2. **会话系统**：复用 `conversations`、`conversation_members` 表、群成员管理逻辑
3. **消息链路**：复用 core `SendMessage` RPC、消息持久化、Kafka 投递、WebSocket 推送链路
4. **鉴权框架**：复用 JWT 工具、RPC 鉴权拦截器，仅扩展 BotAuth 中间件
5. **消费框架**：复用 Kafka consumer 框架实现 Bot 事件消费
6. **限流组件**：复用现有 gateway 限流组件配置 Bot 流量规则

## Steps
### Phase 1: Bot 身份模型落地（可独立上线）
核心目标：Bot 能作为特殊用户实体存在，支持基础管理。
- [ ] 新增 `users.user_type` 字段迁移脚本，默认值 `human`，支持 `bot`/`system`
- [ ] 新增 `bots` 扩展表，存储 Bot 专属信息（owner、描述、状态等）
- [ ] logic.proto 新增 `CreateBot`/`GetBot`/`UpdateBot`/`DisableBot` RPC
- [ ] logic 侧实现 Bot 基础 CRUD 逻辑，校验 owner 权限
- [ ] gateway.api 新增用户侧 Bot 管理接口（创建/列表/详情/更新/禁用）
- [ ] gateway 侧实现用户 Bot 管理 handler，复用用户 JWT 鉴权
- [ ] 用户信息查询接口返回 `user_type` 字段，支持前端标识 Bot
- [ ] 单元测试覆盖：Bot CRUD、权限校验、禁用逻辑

### Phase 2: Bot 群聊安装能力（可独立上线）
核心目标：Bot 能作为群成员加入群聊，支持权限配置。
- [ ] 新增 `conversation_bot_settings` 表，存储 Bot 在群内的权限、状态
- [ ] `conversation_members.role` 枚举支持 `bot` 值
- [ ] logic.proto 新增 `InstallBotToConversation`/`ListConversationBots`/`UninstallBot` RPC
- [ ] logic 侧实现 Bot 安装逻辑：校验群管理员权限、自动添加 Bot 为群成员、写入配置
- [ ] logic 侧实现 Bot 卸载逻辑：移除配置、从群成员中删除 Bot
- [ ] gateway.api 新增会话 Bot 管理接口（安装/列表/卸载）
- [ ] gateway 侧实现会话 Bot 管理 handler，复用群权限校验
- [ ] 群成员列表接口返回 Bot 标识
- [ ] 单元测试覆盖：安装/卸载权限校验、群成员同步、Bot 状态校验

### Phase 3: Bot Token + 消息发送能力（可独立上线）
核心目标：外部开发者可通过 Bot Token 调用 API 发送群消息。
- [ ] 新增 `bot_tokens` 表，存储 Bot Token 哈希、scope、过期时间
- [ ] shared 层新增 BotToken 工具：生成随机 Token、哈希存储、校验
- [ ] gateway 新增 `BotAuth` 中间件，支持 `Authorization: Bot <token>` 鉴权
- [ ] logic.proto 新增 `CreateBotToken`/`ListBotTokens`/`RevokeBotToken` RPC
- [ ] gateway.api 新增 Bot 自调用接口：`/api/bot/me`、`/api/bot/conversations`、`/api/bot/messages`
- [ ] logic 侧实现 Bot 发消息校验逻辑：Token 权限、Bot 状态、是否为群成员、群内权限
- [ ] 复用 core `SendMessage` RPC 链路投递 Bot 消息，和普通用户消息逻辑一致
- [ ] 配置限流规则：单 Bot 全局 60 条/分钟，单 Bot 单群 20 条/分钟
- [ ] 单元测试覆盖：Token 生成/校验/撤销、消息发送权限校验、限流生效

### Phase 4: Webhook 事件投递能力（可独立上线）
核心目标：Bot 可通过 Webhook 接收群聊消息和事件。
- [ ] 新增 `bot_webhooks` 表，存储 Webhook 地址、事件订阅、签名 secret
- [ ] logic.proto 新增 `SetWebhook`/`GetWebhook`/`RotateWebhookSecret`/`TestWebhook` RPC
- [ ] gateway.api 新增 Bot Webhook 管理接口
- [ ] 新增 Bot 事件 Kafka consumer，消费群消息生成 Webhook 投递任务
- [ ] 实现 Webhook 投递逻辑：HMAC 签名、指数退避重试、幂等 EventID、失败日志记录
- [ ] 支持 V0 事件类型：`message.created`、`bot.installed`、`bot.uninstalled`
- [ ] 事件 payload 带 sender 简要快照：`user_id`/`user_type`/`nickname`/`avatar`（不带隐私字段）
- [ ] 默认跳过 Bot 自己发送的消息，避免回调循环
- [ ] 单元测试覆盖：签名校验、重试逻辑、事件过滤、投递失败处理

## Verification
### 冒烟测试用例
1. 用户流程：普通用户登录 -> 创建 Bot -> 得到 Bot 信息
2. 安装流程：群管理员 -> 安装 Bot 到群 -> Bot 出现在群成员列表
3. 发消息流程：创建 Bot Token -> 调用 `/api/bot/messages` -> 群内所有成员收到 Bot 消息
4. 收消息流程：配置 Webhook -> 群内用户发消息 -> Webhook 收到 `message.created` 事件回调

### 边界测试用例
1. 非 Bot owner 尝试修改 Bot 信息 -> 返回 403
2. 普通群成员尝试安装 Bot -> 返回 403
3. Bot 禁用后，使用旧 Token 发消息 -> 返回 401
4. Bot 被移除群后，尝试发消息到该群 -> 返回 403
5. 重复发送相同 `client_msg_id` -> 返回幂等成功，不重复发消息
6. Webhook 返回非 2xx -> 自动重试最多 5 次，超时 5s

### 性能测试用例
1. Bot 发送消息延迟和普通用户发送延迟差值 < 10ms
2. Webhook 投递峰值 1000 TPS 下，主消息链路延迟无影响
3. 100 个 Bot 同时订阅同一个群，每个群消息都能被所有 Bot 收到，无丢事件

### 安全测试用例
1. Bot Token 泄露后，撤销 Token -> 旧 Token 立即失效
2. Webhook 请求篡改 payload -> 签名校验失败，第三方拒收
3. Bot 尝试访问未加入的会话 -> 返回 403
4. 限流规则生效：Bot 1 分钟内发 21 条消息到同一个群 -> 第 21 条返回 429
