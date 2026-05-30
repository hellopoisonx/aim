-- name: CreateUserBotProfile :one
INSERT INTO user_info (id, email, nickname, avatar, status, user_type)
VALUES ($1, $2, $3, $4, 1, 'bot')
RETURNING id, email, status, nickname, avatar, created_at, updated_at, user_type;

-- name: CreateUserBotOwnership :one
INSERT INTO user_bots (bot_user_id, owner_user_id)
VALUES ($1, $2)
RETURNING bot_user_id, owner_user_id, deleted_at, created_at, updated_at;

-- name: GetManagedUserBot :one
SELECT ui.id,
       ui.email,
       ui.status,
       ui.nickname,
       ui.avatar,
       ui.created_at,
       ui.updated_at,
       ui.user_type,
       ub.owner_user_id,
       ub.deleted_at,
       ub.created_at AS ownership_created_at,
       ub.updated_at AS ownership_updated_at
FROM user_bots ub
JOIN user_info ui ON ui.id = ub.bot_user_id
WHERE ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot';

-- name: ListManagedUserBots :many
SELECT ui.id,
       ui.email,
       ui.status,
       ui.nickname,
       ui.avatar,
       ui.created_at,
       ui.updated_at,
       ui.user_type,
       ub.owner_user_id,
       ub.deleted_at,
       ub.created_at AS ownership_created_at,
       ub.updated_at AS ownership_updated_at
FROM user_bots ub
JOIN user_info ui ON ui.id = ub.bot_user_id
WHERE ub.owner_user_id = $1
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
ORDER BY ub.created_at DESC, ui.id DESC;

-- name: UpdateManagedUserBotProfile :one
UPDATE user_info ui
SET nickname = $3,
    avatar = $4,
    updated_at = NOW()
FROM user_bots ub
WHERE ui.id = ub.bot_user_id
  AND ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
RETURNING ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type;

-- name: UpdateManagedUserBotStatus :one
UPDATE user_info ui
SET status = $3,
    updated_at = NOW()
FROM user_bots ub
WHERE ui.id = ub.bot_user_id
  AND ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND ui.user_type = 'bot'
RETURNING ui.id, ui.email, ui.status, ui.nickname, ui.avatar, ui.created_at, ui.updated_at, ui.user_type;

-- name: SoftDeleteManagedUserBot :execrows
UPDATE user_bots
SET deleted_at = COALESCE(deleted_at, NOW()),
    updated_at = NOW()
WHERE owner_user_id = $1
  AND bot_user_id = $2
  AND deleted_at IS NULL;

-- name: RevokeAllBotTokensByBot :execrows
UPDATE bot_tokens
SET revoked_at = COALESCE(revoked_at, NOW())
WHERE bot_user_id = $1
  AND revoked_at IS NULL;

-- name: GetManagedBotToken :one
SELECT bt.id, bt.bot_user_id, bt.token_hash, bt.name, bt.scopes, bt.expires_at, bt.revoked_at, bt.created_at
FROM bot_tokens bt
JOIN user_bots ub ON ub.bot_user_id = bt.bot_user_id
WHERE ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND bt.id = $3;

-- name: UpdateManagedBotToken :one
UPDATE bot_tokens bt
SET name = $4,
    expires_at = $5
FROM user_bots ub
WHERE ub.bot_user_id = bt.bot_user_id
  AND ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND bt.id = $3
  AND bt.revoked_at IS NULL
RETURNING bt.id, bt.bot_user_id, bt.token_hash, bt.name, bt.scopes, bt.expires_at, bt.revoked_at, bt.created_at;

-- name: RevokeManagedBotToken :execrows
UPDATE bot_tokens bt
SET revoked_at = COALESCE(bt.revoked_at, NOW())
FROM user_bots ub
WHERE ub.bot_user_id = bt.bot_user_id
  AND ub.owner_user_id = $1
  AND ub.bot_user_id = $2
  AND ub.deleted_at IS NULL
  AND bt.id = $3
  AND bt.revoked_at IS NULL;

-- name: ClearBotTokenActions :execrows
DELETE FROM bot_token_permissions
WHERE token_id = $1;

-- name: ListEnabledBotActions :many
SELECT id, action, description, enabled, created_at, updated_at
FROM bot_actions
WHERE enabled = TRUE
ORDER BY action;

-- name: ListEnabledBotActionsByNames :many
SELECT id, action, description, enabled, created_at, updated_at
FROM bot_actions
WHERE action = ANY($1::text[])
  AND enabled = TRUE
ORDER BY action;

-- name: ListEnabledBotEvents :many
SELECT bea.event,
       ba.action,
       bea.description,
       bea.enabled,
       bea.created_at,
       bea.updated_at
FROM bot_event_actions bea
JOIN bot_actions ba ON ba.id = bea.action_id
WHERE bea.enabled = TRUE
  AND ba.enabled = TRUE
ORDER BY bea.event;
