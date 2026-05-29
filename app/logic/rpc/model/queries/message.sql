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

-- name: SearchMessagesGlobal :many
SELECT m.id, m.conversation_id, m.sender_id, m.message_type, m.content, m.client_msg_id, m.mentions, m.created_at,
       ts_headline('simple', m.content #>> '{}', plainto_tsquery('simple', sqlc.arg(search)::text), 'StartSel=<mark>, StopSel=</mark>, MaxWords=18, MinWords=3, ShortWord=1') AS snippet
FROM messages m
JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = $1
WHERE (
        to_tsvector('simple', m.content #>> '{}') @@ plainto_tsquery('simple', sqlc.arg(search)::text)
        OR (m.content #>> '{}') ILIKE '%' || sqlc.arg(search)::text || '%'
      )
  AND (
        sqlc.arg(cursor_created_at)::timestamptz IS NULL
        OR m.created_at < sqlc.arg(cursor_created_at)::timestamptz
        OR (m.created_at = sqlc.arg(cursor_created_at)::timestamptz AND m.id < sqlc.arg(cursor_id)::bigint)
      )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_rows);

-- name: SearchMessagesInConversation :many
SELECT m.id, m.conversation_id, m.sender_id, m.message_type, m.content, m.client_msg_id, m.mentions, m.created_at,
       ts_headline('simple', m.content #>> '{}', plainto_tsquery('simple', sqlc.arg(search)::text), 'StartSel=<mark>, StopSel=</mark>, MaxWords=18, MinWords=3, ShortWord=1') AS snippet
FROM messages m
JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = $1
WHERE m.conversation_id = $2
  AND (
        to_tsvector('simple', m.content #>> '{}') @@ plainto_tsquery('simple', sqlc.arg(search)::text)
        OR (m.content #>> '{}') ILIKE '%' || sqlc.arg(search)::text || '%'
      )
  AND (
        sqlc.arg(cursor_created_at)::timestamptz IS NULL
        OR m.created_at < sqlc.arg(cursor_created_at)::timestamptz
        OR (m.created_at = sqlc.arg(cursor_created_at)::timestamptz AND m.id < sqlc.arg(cursor_id)::bigint)
      )
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_rows);