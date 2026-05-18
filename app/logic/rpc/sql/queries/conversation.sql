-- name: GetConversation :one
SELECT id, conversation_type, is_active
FROM conversations
WHERE id = $1;