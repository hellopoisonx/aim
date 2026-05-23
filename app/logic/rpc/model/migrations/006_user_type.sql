-- 006_user_type: distinguish human / bot / system identities on the shared user_info table.
-- 'human' — regular AIM users authenticated via auth.user_credentials (default).
-- 'bot'   — programmatic identities provisioned for the OpenAPI Bot interface.
-- 'system'— reserved for internal system-issued messages (sender_id = 0 today).
ALTER TABLE user_info ADD COLUMN IF NOT EXISTS user_type VARCHAR(16) NOT NULL DEFAULT 'human';

CREATE INDEX IF NOT EXISTS idx_user_info_user_type ON user_info (user_type);
