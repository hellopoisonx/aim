-- name: GetFriendship :one
SELECT user_id, friend_id, status
FROM friendships
WHERE user_id = $1 AND friend_id = $2;

-- name: GetFriendshipBidirectional :many
SELECT user_id, friend_id, status
FROM friendships
WHERE (user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1);

-- name: UpsertFriendship :one
INSERT INTO friendships (user_id, friend_id, status, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
ON CONFLICT (user_id, friend_id) DO UPDATE SET
    status = EXCLUDED.status,
    updated_at = NOW()
RETURNING user_id, friend_id, status, created_at, updated_at;

-- name: GetFriendshipByPair :one
SELECT user_id, friend_id, status, created_at, updated_at
FROM friendships
WHERE user_id = $1 AND friend_id = $2;

-- name: ListPendingFriendApplications :many
SELECT user_id, friend_id, status, created_at, updated_at
FROM friendships
WHERE friend_id = $1 AND status = 'pending'
ORDER BY created_at DESC;

-- name: ListFriends :many
SELECT user_id, friend_id, status, created_at, updated_at
FROM friendships
WHERE user_id = $1 AND status = 'accepted'
ORDER BY created_at DESC;
