CREATE TABLE IF NOT EXISTS attachment_files (
    file_id UUID PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    conversation_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('image', 'video', 'audio')),
    original_name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 5368709120),
    sha256 TEXT,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'uploaded', 'failed', 'deleted')),
    parse_status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (parse_status IN ('pending', 'ready', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    uploaded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachment_files_owner ON attachment_files(owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attachment_files_conversation ON attachment_files(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS attachment_objects (
    id BIGSERIAL PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES attachment_files(file_id) ON DELETE CASCADE,
    object_role VARCHAR(32) NOT NULL CHECK (object_role IN ('original', 'thumbnail', 'preview')),
    bucket TEXT NOT NULL,
    object_key TEXT NOT NULL,
    mime TEXT NOT NULL,
    size_bytes BIGINT,
    sha256 TEXT,
    width INT,
    height INT,
    duration_ms BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bucket, object_key),
    UNIQUE(file_id, object_role)
);

CREATE INDEX IF NOT EXISTS idx_attachment_objects_file ON attachment_objects(file_id);

CREATE TABLE IF NOT EXISTS attachment_references (
    id BIGSERIAL PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES attachment_files(file_id) ON DELETE CASCADE,
    message_id BIGINT,
    conversation_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(file_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_attachment_references_conversation ON attachment_references(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS attachment_parse_jobs (
    id BIGSERIAL PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES attachment_files(file_id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachment_parse_jobs_ready ON attachment_parse_jobs(status, available_at);

CREATE TABLE IF NOT EXISTS attachment_parse_results (
    file_id UUID PRIMARY KEY REFERENCES attachment_files(file_id) ON DELETE CASCADE,
    thumbnail_object_id BIGINT REFERENCES attachment_objects(id),
    duration_ms BIGINT,
    width INT,
    height INT,
    metadata JSONB NOT NULL DEFAULT '{}',
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
