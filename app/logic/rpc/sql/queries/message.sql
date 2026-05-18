-- name: InsertMessage :exec
INSERT INTO messages (id, conversation_id, sender_id, message_type, content, client_msg_id, mentions, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW());