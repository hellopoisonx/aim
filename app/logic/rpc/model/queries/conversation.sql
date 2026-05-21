-- name: GetConversation :one
SELECT id, conversation_type, is_active, created_at
FROM conversations
WHERE id = $1;

-- name: CreateConversation :one
INSERT INTO conversations (id, conversation_type, is_active)
VALUES ($1, $2, true)
RETURNING id, conversation_type, is_active, created_at;

-- name: AddConversationMembers :execrows
INSERT INTO conversation_members (conversation_id, user_id)
VALUES ($1, $2);

-- name: GetConversationMembers :many
SELECT conversation_id, user_id, is_muted, muted_until, joined_at
FROM conversation_members
WHERE conversation_id = $1;

-- name: GetConversationsByUserID :many
SELECT c.id, c.conversation_type, c.is_active, c.created_at
FROM conversations c
INNER JOIN conversation_members cm ON c.id = cm.conversation_id
WHERE cm.user_id = $1
ORDER BY c.created_at DESC;

-- name: GetDirectConversationByMembers :one
SELECT c.id, c.conversation_type, c.is_active, c.created_at
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