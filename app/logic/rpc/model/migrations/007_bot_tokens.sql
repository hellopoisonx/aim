-- 007_bot_tokens: API tokens used by external Bot services to call AIM OpenAPI.
-- The plaintext token is shown to operators only at provisioning time; only its
-- SHA-256 hash is persisted. scopes is a legacy text array kept for schema compatibility; action permissions live in bot_token_permissions from migration 009.
CREATE TABLE IF NOT EXISTS bot_tokens (
    id BIGINT PRIMARY KEY,
    bot_user_id BIGINT NOT NULL,
    token_hash VARCHAR(128) NOT NULL UNIQUE,
    name VARCHAR(64) NOT NULL DEFAULT '',
    scopes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bot_tokens_bot_user_id ON bot_tokens (bot_user_id);
