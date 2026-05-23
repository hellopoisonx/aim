-- name: UpsertBotWebhook :one
INSERT INTO bot_webhooks (bot_user_id, url, secret_hash, events, enabled, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (bot_user_id) DO UPDATE SET
    url = EXCLUDED.url,
    secret_hash = EXCLUDED.secret_hash,
    events = EXCLUDED.events,
    enabled = EXCLUDED.enabled,
    updated_at = NOW()
RETURNING bot_user_id, url, secret_hash, events, enabled, created_at, updated_at;

-- name: GetBotWebhook :one
SELECT bot_user_id, url, secret_hash, events, enabled, created_at, updated_at
FROM bot_webhooks
WHERE bot_user_id = $1;

-- name: DeleteBotWebhook :execrows
DELETE FROM bot_webhooks WHERE bot_user_id = $1;

-- name: ListActiveBotWebhooksForConversation :many
-- Returns enabled webhooks for bots that are members of the given conversation.
-- Used by the BotWebhookConsumer to fan out message.created events.
SELECT bw.bot_user_id,
       bw.url,
       bw.secret_hash,
       bw.events,
       bw.enabled,
       bw.created_at,
       bw.updated_at
FROM bot_webhooks bw
JOIN conversation_members cm ON cm.user_id = bw.bot_user_id
JOIN user_info ui ON ui.id = bw.bot_user_id
WHERE cm.conversation_id = $1
  AND bw.enabled = TRUE
  AND ui.user_type = 'bot'
  AND ui.status = 1;
