-- friend tags: user-scoped labels for accepted friends.
CREATE TABLE IF NOT EXISTS friend_tags (
    id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT friend_tags_user_name_unique UNIQUE (user_id, name)
);

CREATE TABLE IF NOT EXISTS friend_tag_assignments (
    user_id BIGINT NOT NULL,
    friend_id BIGINT NOT NULL,
    tag_id BIGINT NOT NULL REFERENCES friend_tags(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_friend_tags_user ON friend_tags(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_friend_tag_assignments_user_friend ON friend_tag_assignments(user_id, friend_id);
CREATE INDEX IF NOT EXISTS idx_friend_tag_assignments_tag ON friend_tag_assignments(tag_id);

-- Search helpers. Keep the DDL idempotent because local compose reruns migrations.
CREATE INDEX IF NOT EXISTS idx_conversations_name_trgm ON conversations USING gin (name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_messages_content_text_trgm
ON messages USING gin ((content #>> '{}') gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_messages_content_text_tsv
ON messages USING gin (to_tsvector('simple', content #>> '{}'));
