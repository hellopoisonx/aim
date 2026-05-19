CREATE TABLE IF NOT EXISTS user_info (
                                         id BIGINT PRIMARY KEY,
                                         email VARCHAR(255) NOT NULL UNIQUE,
    status SMALLINT NOT NULL DEFAULT 1,
    nickname VARCHAR(255) NOT NULL,
    avatar VARCHAR(255) NOT NULL DEFAULT 'https://implement.me',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );

-- 昵称索引（用于搜索）
CREATE INDEX idx_user_info_nickname ON user_info (nickname);

-- 创建时间索引（用于后台按时间排序、统计）
CREATE INDEX idx_user_info_created_at ON user_info (created_at DESC);

-- 昵称倒排索引（模糊匹配）
CREATE INDEX idx_user_info_nickname_trgm ON user_info USING gin (nickname gin_trgm_ops);