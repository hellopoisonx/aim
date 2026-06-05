-- name: InsertOutboxRecord :one
INSERT INTO outbox_records (topic, key, payload)
VALUES ($1, $2, $3)
RETURNING id;

-- name: FetchPendingOutboxRecords :many
SELECT id, topic, key, payload, created_at
FROM outbox_records
WHERE processed_at IS NULL
ORDER BY created_at
LIMIT $1;

-- name: MarkOutboxRecordsProcessed :execrows
UPDATE outbox_records
SET processed_at = NOW()
WHERE id = ANY($1::BIGINT[]);

-- name: DeleteOutboxRecordsBefore :execrows
DELETE FROM outbox_records
WHERE processed_at IS NOT NULL AND processed_at < $1;
