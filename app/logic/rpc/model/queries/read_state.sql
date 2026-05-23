-- name: UpsertConversationReadState :one
INSERT INTO conversation_read_states (conversation_id, user_id, last_read_message_id, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (conversation_id, user_id)
DO UPDATE SET
    last_read_message_id = GREATEST(conversation_read_states.last_read_message_id, EXCLUDED.last_read_message_id),
    updated_at = CASE
        WHEN EXCLUDED.last_read_message_id > conversation_read_states.last_read_message_id THEN NOW()
        ELSE conversation_read_states.updated_at
    END
RETURNING conversation_id, user_id, last_read_message_id, updated_at;

-- name: GetConversationReadState :one
SELECT conversation_id, user_id, last_read_message_id, updated_at
FROM conversation_read_states
WHERE conversation_id = $1 AND user_id = $2;

-- name: ListConversationReadStates :many
SELECT conversation_id, user_id, last_read_message_id, updated_at
FROM conversation_read_states
WHERE conversation_id = $1
ORDER BY user_id;
