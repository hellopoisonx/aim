-- conversations: chat sessions (direct or group)
CREATE TABLE IF NOT EXISTS conversations (
    id BIGINT PRIMARY KEY,
    conversation_type VARCHAR(16) NOT NULL DEFAULT 'direct', -- 'direct' or 'group'
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- conversation_members: membership and mute status
CREATE TABLE IF NOT EXISTS conversation_members (
    conversation_id BIGINT NOT NULL REFERENCES conversations(id),
    user_id BIGINT NOT NULL,
    is_muted BOOLEAN NOT NULL DEFAULT false,
    muted_until TIMESTAMPTZ,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, user_id)
);

-- friendships: user-to-user relationship
CREATE TABLE IF NOT EXISTS friendships (
    user_id BIGINT NOT NULL,
    friend_id BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending', -- 'pending', 'accepted', 'blocked'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id)
);

CREATE INDEX IF NOT EXISTS idx_cm_user ON conversation_members(user_id);
CREATE INDEX IF NOT EXISTS idx_friendships_friend ON friendships(friend_id);