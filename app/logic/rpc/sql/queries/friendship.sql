-- name: GetFriendship :one
SELECT user_id, friend_id, status
FROM friendships
WHERE user_id = $1 AND friend_id = $2;

-- name: GetFriendshipBidirectional :many
SELECT user_id, friend_id, status
FROM friendships
WHERE (user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1);