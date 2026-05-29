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
FROM (
    SELECT DISTINCT ON (peer_id)
        $1::bigint AS user_id,
        peer_id AS friend_id,
        status,
        created_at,
        updated_at
    FROM (
        SELECT
            CASE WHEN user_id = $1 THEN friend_id ELSE user_id END AS peer_id,
            status,
            created_at,
            updated_at
        FROM friendships
        WHERE status = 'accepted' AND (user_id = $1 OR friend_id = $1)
    ) normalized
    ORDER BY peer_id, updated_at DESC
) deduped
ORDER BY updated_at DESC;

-- name: CreateFriendTag :one
INSERT INTO friend_tags (id, user_id, name, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING id, user_id, name, created_at, updated_at;

-- name: GetFriendTagByID :one
SELECT id, user_id, name, created_at, updated_at
FROM friend_tags
WHERE user_id = $1 AND id = $2;

-- name: GetFriendTagsByIDs :many
SELECT id, user_id, name, created_at, updated_at
FROM friend_tags
WHERE user_id = $1 AND id = ANY($2::bigint[])
ORDER BY name ASC, id ASC;

-- name: ListFriendTags :many
SELECT id, user_id, name, created_at, updated_at
FROM friend_tags
WHERE user_id = $1
ORDER BY updated_at DESC, name ASC, id ASC;

-- name: RenameFriendTag :one
UPDATE friend_tags
SET name = $3, updated_at = NOW()
WHERE user_id = $1 AND id = $2
RETURNING id, user_id, name, created_at, updated_at;

-- name: DeleteFriendTag :execrows
DELETE FROM friend_tags
WHERE user_id = $1 AND id = $2;

-- name: ReplaceFriendTags :exec
WITH deleted AS (
    DELETE FROM friend_tag_assignments fta
    WHERE fta.user_id = $1 AND fta.friend_id = $2
), inserted AS (
    INSERT INTO friend_tag_assignments (user_id, friend_id, tag_id, created_at)
    SELECT $1::bigint, $2::bigint, unnest($3::bigint[]), NOW()
    ON CONFLICT (user_id, friend_id, tag_id) DO NOTHING
    RETURNING 1
)
SELECT 1;

-- name: RemoveFriendTagAssignment :execrows
DELETE FROM friend_tag_assignments
WHERE user_id = $1 AND friend_id = $2 AND tag_id = $3;

-- name: ListFriendTagAssignmentsForUser :many
SELECT fta.friend_id, ft.id, ft.user_id, ft.name, ft.created_at, ft.updated_at
FROM friend_tag_assignments fta
JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
WHERE fta.user_id = $1
ORDER BY fta.friend_id ASC, ft.name ASC, ft.id ASC;

-- name: ListFriendTagsForFriend :many
SELECT ft.id, ft.user_id, ft.name, ft.created_at, ft.updated_at
FROM friend_tag_assignments fta
JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
WHERE fta.user_id = $1 AND fta.friend_id = $2
ORDER BY ft.name ASC, ft.id ASC;

-- name: ListFriendsByTagID :many
SELECT user_id, friend_id, status, created_at, updated_at
FROM (
    SELECT DISTINCT ON (normalized.peer_id)
        $1::bigint AS user_id,
        normalized.peer_id AS friend_id,
        normalized.status,
        normalized.created_at,
        normalized.updated_at
    FROM (
        SELECT
            CASE WHEN fs.user_id = $1 THEN fs.friend_id ELSE fs.user_id END AS peer_id,
            fs.status,
            fs.created_at,
            fs.updated_at
        FROM friendships fs
        WHERE fs.status = 'accepted' AND (fs.user_id = $1 OR fs.friend_id = $1)
    ) normalized
    JOIN friend_tag_assignments fta
      ON fta.user_id = $1 AND fta.friend_id = normalized.peer_id AND fta.tag_id = $2
    ORDER BY normalized.peer_id, normalized.updated_at DESC
) deduped
ORDER BY deduped.updated_at DESC;

-- name: ListFriendsByTagName :many
SELECT user_id, friend_id, status, created_at, updated_at
FROM (
    SELECT DISTINCT ON (normalized.peer_id)
        $1::bigint AS user_id,
        normalized.peer_id AS friend_id,
        normalized.status,
        normalized.created_at,
        normalized.updated_at
    FROM (
        SELECT
            CASE WHEN fs.user_id = $1 THEN fs.friend_id ELSE fs.user_id END AS peer_id,
            fs.status,
            fs.created_at,
            fs.updated_at
        FROM friendships fs
        WHERE fs.status = 'accepted' AND (fs.user_id = $1 OR fs.friend_id = $1)
    ) normalized
    JOIN friend_tag_assignments fta
      ON fta.user_id = $1 AND fta.friend_id = normalized.peer_id
    JOIN friend_tags ft
      ON ft.id = fta.tag_id AND ft.user_id = fta.user_id AND ft.name = $2
    ORDER BY normalized.peer_id, normalized.updated_at DESC
) deduped
ORDER BY deduped.updated_at DESC;

-- name: SearchFriendsByQuery :many
SELECT DISTINCT ON (peer_id)
    $1::bigint AS user_id,
    peer_id AS friend_id,
    ui.email,
    ui.nickname,
    ui.avatar,
    similarity(ui.nickname || ' ' || ui.email, sqlc.arg(search)::text) AS rank,
    ts_headline('simple', ui.nickname || ' ' || ui.email, plainto_tsquery('simple', sqlc.arg(search)::text), 'StartSel=<mark>, StopSel=</mark>, MaxWords=12, MinWords=1, ShortWord=1') AS snippet
FROM (
    SELECT CASE WHEN user_id = $1 THEN friend_id ELSE user_id END AS peer_id
    FROM friendships
    WHERE status = 'accepted' AND (user_id = $1 OR friend_id = $1)
    UNION
    SELECT fta.friend_id AS peer_id
    FROM friend_tag_assignments fta
    JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
    WHERE fta.user_id = $1 AND ft.name ILIKE '%' || sqlc.arg(search)::text || '%'
 ) peers
JOIN user_info ui ON ui.id = peer_id
WHERE ui.nickname ILIKE '%' || sqlc.arg(search)::text || '%'
   OR ui.email ILIKE '%' || sqlc.arg(search)::text || '%'
   OR EXISTS (
       SELECT 1
       FROM friend_tag_assignments fta
       JOIN friend_tags ft ON ft.id = fta.tag_id AND ft.user_id = fta.user_id
       WHERE fta.user_id = $1 AND fta.friend_id = peer_id AND ft.name ILIKE '%' || sqlc.arg(search)::text || '%'
   )
ORDER BY peer_id, rank DESC, ui.id ASC
LIMIT sqlc.arg(max_rows);
