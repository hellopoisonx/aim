-- 017_drop_bot_token_scopes: action permissions live in bot_token_permissions
-- (009). Removes the legacy scopes dual-write column.
ALTER TABLE bot_tokens DROP COLUMN IF EXISTS scopes;
