-- 011_bot_conversation_read_actions: add Bot OpenAPI conversation read-side actions.
--
-- These actions allow Bot tokens to read conversation history/member details and
-- manage/read conversation read cursors without granting broad message/webhook
-- privileges.

INSERT INTO bot_actions (id, action, description, enabled)
VALUES
    (9, 'bot.conversation.history', 'Read message history from conversations the bot belongs to', TRUE),
    (10, 'bot.conversation.members.read', 'Read member details from conversations the bot belongs to', TRUE),
    (11, 'bot.read_receipt.write', 'Update the bot read cursor for a conversation', TRUE),
    (12, 'bot.read_receipt.read', 'Read conversation read cursors', TRUE),
    (104, 'bot.conversation.*', 'Wildcard: all bot conversation read actions', TRUE),
    (105, 'bot.read_receipt.*', 'Wildcard: all bot read receipt actions', TRUE)
ON CONFLICT (action) DO UPDATE SET
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();
