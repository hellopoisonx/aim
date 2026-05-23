-- 009_bot_actions: action-based permissions for Bot OpenAPI tokens.
--
-- bot_actions is the runtime-adjustable action dictionary. Tokens are granted
-- actions through bot_token_permissions(action_id). Webhook event subscription
-- requirements are stored separately in bot_event_actions so event -> action
-- mapping can be adjusted without code changes.

CREATE TABLE IF NOT EXISTS bot_actions (
    id BIGINT PRIMARY KEY,
    action VARCHAR(128) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bot_token_permissions (
    token_id BIGINT NOT NULL REFERENCES bot_tokens(id) ON DELETE CASCADE,
    action_id BIGINT NOT NULL REFERENCES bot_actions(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (token_id, action_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_token_permissions_action_id
    ON bot_token_permissions (action_id);

CREATE TABLE IF NOT EXISTS bot_event_actions (
    event VARCHAR(128) PRIMARY KEY,
    action_id BIGINT NOT NULL REFERENCES bot_actions(id),
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bot_event_actions_action_id
    ON bot_event_actions (action_id);

INSERT INTO bot_actions (id, action, description, enabled)
VALUES
    (1, 'bot.self.read', 'Read current bot identity', TRUE),
    (2, 'bot.conversation.list', 'List conversations the bot belongs to', TRUE),
    (3, 'bot.message.send', 'Send messages as the bot', TRUE),
    (4, 'bot.webhook.read', 'Read bot webhook configuration', TRUE),
    (5, 'bot.webhook.write', 'Create or update bot webhook configuration', TRUE),
    (6, 'bot.webhook.delete', 'Delete bot webhook configuration', TRUE),
    (7, 'bot.webhook.subscribe.message_created', 'Subscribe to message.created webhook events', TRUE),
    (100, '*', 'Wildcard: all bot actions', TRUE),
    (101, 'bot.*', 'Wildcard: all bot actions', TRUE),
    (102, 'bot.message.*', 'Wildcard: all bot message actions', TRUE),
    (103, 'bot.webhook.subscribe.*', 'Wildcard: all bot webhook subscription actions', TRUE)
ON CONFLICT (action) DO UPDATE SET
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();

INSERT INTO bot_event_actions (event, action_id, description, enabled)
SELECT 'message.created', id, 'message.created webhook subscription requires bot.webhook.subscribe.message_created', TRUE
FROM bot_actions
WHERE action = 'bot.webhook.subscribe.message_created'
ON CONFLICT (event) DO UPDATE SET
    action_id = EXCLUDED.action_id,
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();
