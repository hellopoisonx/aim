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