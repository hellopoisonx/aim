-- conversation_read_states: last read cursor per member in each conversation
CREATE TABLE IF NOT EXISTS conversation_read_states (
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    last_read_message_id BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_read_states_conversation
    ON conversation_read_states(conversation_id, updated_at DESC, user_id);
