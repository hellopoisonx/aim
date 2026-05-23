-- name: CreateUserInfo :one
INSERT INTO user_info (id, email, nickname, avatar, status)
VALUES ($1, $2, $3, $4, 1)
RETURNING id, email, status, nickname, avatar, created_at, updated_at, user_type;

-- name: GetUserInfoByID :one
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type
FROM user_info
WHERE id = $1;

-- name: GetUserInfoByEmail :one
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type
FROM user_info
WHERE email = $1;

-- name: GetUserInfoByNickname :one
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type
FROM user_info
WHERE nickname = $1;

-- name: UpdateUserInfoProfile :one
UPDATE user_info
SET nickname = $2, avatar = $3, updated_at = NOW()
WHERE id = $1
RETURNING id, email, status, nickname, avatar, created_at, updated_at, user_type;

-- name: UpdateUserInfoStatus :one
UPDATE user_info
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING id, email, status, nickname, avatar, created_at, updated_at, user_type;

-- name: SearchUserInfoByNickname :many
SELECT id, email, status, nickname, avatar, created_at, updated_at, user_type
FROM user_info
WHERE nickname ILIKE '%' || sqlc.arg(search) || '%'
ORDER BY similarity(nickname, sqlc.arg(search)) DESC, created_at DESC, id ASC
LIMIT sqlc.arg(max_rows);

-- name: GetUserType :one
SELECT user_type FROM user_info WHERE id = $1;

-- name: UpdateUserInfoType :execrows
UPDATE user_info SET user_type = $2, updated_at = NOW() WHERE id = $1;
