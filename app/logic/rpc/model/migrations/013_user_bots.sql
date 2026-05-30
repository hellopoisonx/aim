-- 013_user_bots: user-owned Bot management metadata.
--
-- user_info.user_type='bot' remains the runtime identity marker. This table
-- records who may manage a bot through user-side REST APIs. owner_user_id=0 is
-- reserved for system-owned official bots.

CREATE TABLE IF NOT EXISTS user_bots (
    bot_user_id BIGINT PRIMARY KEY REFERENCES user_info(id),
    owner_user_id BIGINT NOT NULL,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_bots_owner_user_id
    ON user_bots (owner_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_bots_owner_active
    ON user_bots (owner_user_id, created_at DESC)
    WHERE deleted_at IS NULL;
