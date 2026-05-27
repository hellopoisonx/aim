-- 010_bot_attachment_actions: add bot.attachment.download action.
--
-- Bot OpenAPI V0 can receive attachment metadata via webhook but cannot
-- download the actual file data. This migration adds the required token
-- action so that authorized bots may call GET /api/bot/v1/attachments/:id/download.

INSERT INTO bot_actions (id, action, description, enabled)
VALUES (8, 'bot.attachment.download', 'Download attachments from conversations the bot belongs to', TRUE)
ON CONFLICT (action) DO UPDATE SET
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();
