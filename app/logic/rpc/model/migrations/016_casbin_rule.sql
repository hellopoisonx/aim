-- Casbin policy storage. Only grouping (g) rules live here; the static policy
-- matrix is seeded in memory at startup.
CREATE TABLE IF NOT EXISTS casbin_rule (
    id   BIGSERIAL PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL,
    v0   VARCHAR(100) NOT NULL DEFAULT '',
    v1   VARCHAR(100) NOT NULL DEFAULT '',
    v2   VARCHAR(100) NOT NULL DEFAULT '',
    v3   VARCHAR(100) NOT NULL DEFAULT '',
    v4   VARCHAR(100) NOT NULL DEFAULT '',
    v5   VARCHAR(100) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_casbin_rule ON casbin_rule (ptype, v0, v1, v2, v3, v4, v5);

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT 'g',
       CASE WHEN ui.user_type = 'bot' THEN 'bot:' ELSE 'user:' END || cm.user_id,
       'role:' || cm.role,
       'conv:' || cm.conversation_id
FROM conversation_members cm
JOIN user_info ui ON ui.id = cm.user_id
WHERE cm.role IN ('member', 'admin', 'owner')
ON CONFLICT DO NOTHING;
