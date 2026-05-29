-- name: GetConversation :one
SELECT id, conversation_type, is_active, name, avatar, creator_id, created_at
FROM conversations
WHERE id = $1;

-- name: CreateConversation :one
INSERT INTO conversations (id, conversation_type, name, avatar, creator_id, is_active)
VALUES ($1, $2, $3, $4, $5, true)
RETURNING id, conversation_type, is_active, name, avatar, creator_id, created_at;

-- name: AddConversationMembers :execrows
INSERT INTO conversation_members (conversation_id, user_id)
VALUES ($1, $2);

-- name: GetConversationMembers :many
SELECT conversation_id, user_id, is_muted, muted_until, role, joined_at
FROM conversation_members
WHERE conversation_id = $1;

-- name: GetConversationsByUserID :many
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at
FROM conversations c
INNER JOIN conversation_members cm ON c.id = cm.conversation_id
WHERE cm.user_id = $1
ORDER BY c.created_at DESC;

-- name: GetDirectConversationByMembers :one
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at
FROM conversations c
WHERE c.conversation_type = 'direct'
  AND c.is_active = true
  AND c.id IN (
    SELECT cm1.conversation_id
    FROM conversation_members cm1
    INNER JOIN conversation_members cm2 ON cm1.conversation_id = cm2.conversation_id
    WHERE cm1.user_id = $1 AND cm2.user_id = $2
  )
LIMIT 1;

-- name: AddConversationMemberWithRole :execrows
INSERT INTO conversation_members (conversation_id, user_id, role)
VALUES ($1, $2, $3);

-- name: RemoveConversationMembers :execrows
DELETE FROM conversation_members
WHERE conversation_id = $1 AND user_id = ANY($2::bigint[]);

-- name: UpdateConversationMemberRole :execrows
UPDATE conversation_members
SET role = $3
WHERE conversation_id = $1 AND user_id = $2;

-- name: UpdateConversationCreator :exec
UPDATE conversations
SET creator_id = $2
WHERE id = $1;

-- name: UpdateConversation :exec
UPDATE conversations
SET name = COALESCE(sqlc.narg('name'), name),
    avatar = COALESCE(sqlc.narg('avatar'), avatar)
WHERE id = $1;

-- name: DeactivateConversation :exec
UPDATE conversations SET is_active = false WHERE id = $1;

-- name: GetConversationCreator :one
SELECT creator_id FROM conversations WHERE id = $1;

-- name: IsConversationMember :one
SELECT EXISTS(
    SELECT 1 FROM conversation_members
    WHERE conversation_id = $1 AND user_id = $2
);

-- name: GetConversationMembersDetail :many
SELECT cm.user_id, ui.email, ui.avatar, ui.nickname AS name, cm.role, cm.joined_at
FROM conversation_members cm
JOIN user_info ui ON cm.user_id = ui.id
WHERE cm.conversation_id = $1;

-- name: SearchConversationsByName :many
SELECT c.id, c.conversation_type, c.is_active, c.name, c.avatar, c.creator_id, c.created_at,
       similarity(c.name, sqlc.arg(search)::text) AS rank,
       ts_headline('simple', c.name, plainto_tsquery('simple', sqlc.arg(search)::text), 'StartSel=<mark>, StopSel=</mark>, MaxWords=12, MinWords=1, ShortWord=1') AS snippet
FROM conversations c
JOIN conversation_members cm ON cm.conversation_id = c.id
WHERE cm.user_id = $1
  AND c.is_active = true
  AND c.name <> ''
  AND c.name ILIKE '%' || sqlc.arg(search)::text || '%'
ORDER BY rank DESC, c.created_at DESC, c.id DESC
LIMIT sqlc.arg(max_rows);
