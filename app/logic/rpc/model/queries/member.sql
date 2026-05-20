-- name: GetMember :one
SELECT conversation_id, user_id, is_muted, muted_until
FROM conversation_members
WHERE conversation_id = $1 AND user_id = $2;

-- name: IsMemberMuted :one
SELECT is_muted, muted_until
FROM conversation_members
WHERE conversation_id = $1 AND user_id = $2;