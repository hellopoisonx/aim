-- name: CreateBotToken :one
INSERT INTO bot_tokens (id, bot_user_id, token_hash, name, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, bot_user_id, token_hash, name, scopes, expires_at, revoked_at, created_at;

-- name: GetBotTokenByHash :one
-- Returns the token entry plus the owner's user_type / status / nickname so
-- the BotAuth middleware can validate the bot identity in a single query.
SELECT bt.id,
       bt.bot_user_id,
       bt.token_hash,
       bt.name,
       bt.scopes,
       bt.expires_at,
       bt.revoked_at,
       bt.created_at,
       ui.user_type,
       ui.status AS user_status,
       ui.nickname,
       ui.avatar
FROM bot_tokens bt
JOIN user_info ui ON ui.id = bt.bot_user_id
WHERE bt.token_hash = $1;

-- name: ListBotTokensByBot :many
SELECT id, bot_user_id, token_hash, name, scopes, expires_at, revoked_at, created_at
FROM bot_tokens
WHERE bot_user_id = $1
ORDER BY created_at DESC;

-- name: RevokeBotToken :execrows
UPDATE bot_tokens
SET revoked_at = NOW()
WHERE id = $1 AND bot_user_id = $2 AND revoked_at IS NULL;

-- name: ListEnabledActionsByToken :many
SELECT ba.action
FROM bot_token_permissions btp
JOIN bot_actions ba ON ba.id = btp.action_id
WHERE btp.token_id = $1 AND ba.enabled = TRUE
ORDER BY ba.action;

-- name: GetBotActionByName :one
SELECT id, action, description, enabled, created_at, updated_at
FROM bot_actions
WHERE action = $1;

-- name: GrantBotTokenAction :exec
INSERT INTO bot_token_permissions (token_id, action_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: GetEnabledActionByWebhookEvent :one
SELECT ba.action
FROM bot_event_actions bea
JOIN bot_actions ba ON ba.id = bea.action_id
WHERE bea.event = $1
  AND bea.enabled = TRUE
  AND ba.enabled = TRUE;
