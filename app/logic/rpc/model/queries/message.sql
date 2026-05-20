-- name: InsertMessage :exec
INSERT INTO messages (id, conversation_id, sender_id, message_type, content, client_msg_id, mentions, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW());

-- name: ListMessagesByConversation :many
SELECT id, conversation_id, sender_id, message_type, content, client_msg_id, mentions, created_at
FROM messages
WHERE conversation_id = $1
  AND (created_at < $2 OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: ListMessagesByConversationInitial :many
SELECT id, conversation_id, sender_id, message_type, content, client_msg_id, mentions, created_at
FROM messages
WHERE conversation_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: CountMessagesByConversation :one
SELECT COUNT(*) FROM messages WHERE conversation_id = $1;