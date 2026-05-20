-- name: CreateUser :one
INSERT INTO user_credentials (id, email, password_hash, status)
VALUES ($1, $2, $3, 1)
RETURNING id, email, password_hash, status, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, status, created_at, updated_at
FROM user_credentials
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, status, created_at, updated_at
FROM user_credentials
WHERE id = $1;

-- name: UpdatePassword :one
UPDATE user_credentials
SET password_hash = $2, updated_at = NOW()
WHERE id = $1
RETURNING id, email, password_hash, status, created_at, updated_at;

-- name: UpdateStatus :one
UPDATE user_credentials
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING id, email, password_hash, status, created_at, updated_at;
