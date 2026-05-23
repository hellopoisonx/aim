-- 008_bot_webhooks: outbound HTTP callback configuration per bot.
-- One bot has at most one webhook destination (PK on bot_user_id). The HMAC
-- signing secret is stored as a SHA-256 hash; the plaintext is shown once at
-- rotation time so the third-party Bot service can verify X-AIM-Signature.
CREATE TABLE IF NOT EXISTS bot_webhooks (
    bot_user_id BIGINT PRIMARY KEY,
    url VARCHAR(512) NOT NULL,
    secret_hash VARCHAR(128) NOT NULL,
    events TEXT[] NOT NULL DEFAULT ARRAY['message.created']::TEXT[],
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bot_webhooks_enabled ON bot_webhooks (enabled);
