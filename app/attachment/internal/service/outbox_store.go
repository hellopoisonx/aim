package service

import (
	"context"
	"fmt"
	"time"

	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rawOutboxStore implements outbox.Store using raw SQL (no sqlc).
// Used by services that don't use sqlc, such as attachment.
type rawOutboxStore struct {
	pool *pgxpool.Pool
}

func newRawOutboxStore(pool *pgxpool.Pool) *rawOutboxStore {
	return &rawOutboxStore{pool: pool}
}

func (s *rawOutboxStore) Insert(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO outbox_records (topic, key, payload) VALUES ($1, $2, $3) RETURNING id`,
		topic, key, payload,
	).Scan(&id)
	return id, err
}

func (s *rawOutboxStore) FetchPending(ctx context.Context, batchSize int) ([]outbox.Record, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, topic, key, payload, created_at
		 FROM outbox_records
		 WHERE processed_at IS NULL
		 ORDER BY created_at
		 LIMIT $1`, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []outbox.Record
	for rows.Next() {
		var r outbox.Record
		if err := rows.Scan(&r.ID, &r.Topic, &r.Key, &r.Payload, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *rawOutboxStore) MarkProcessed(ctx context.Context, ids []int64) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE outbox_records SET processed_at = NOW() WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *rawOutboxStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM outbox_records WHERE processed_at IS NOT NULL AND processed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
