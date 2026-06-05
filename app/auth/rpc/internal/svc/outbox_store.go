// Package svc provides the outbox Store adapter for auth service.
package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// outboxStoreAdapter adapts sqlc-generated Queries to outbox.Store.
type outboxStoreAdapter struct {
	queries model.Querier
}

// NewOutboxStore creates an outbox.Store backed by sqlc queries.
func NewOutboxStore(queries model.Querier) outbox.Store {
	if queries == nil {
		return nil
	}
	return &outboxStoreAdapter{queries: queries}
}

func (a *outboxStoreAdapter) Insert(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	return a.queries.InsertOutboxRecord(ctx, model.InsertOutboxRecordParams{
		Topic:   topic,
		Key:     key,
		Payload: payload,
	})
}

func (a *outboxStoreAdapter) FetchPending(ctx context.Context, batchSize int) ([]outbox.Record, error) {
	limit := int32(batchSize) //nolint:gosec // batchSize is config-controlled, max ~1000
	rows, err := a.queries.FetchPendingOutboxRecords(ctx, limit)
	if err != nil {
		return nil, err
	}
	records := make([]outbox.Record, len(rows))
	for i, r := range rows {
		records[i] = outbox.Record{
			ID:        r.ID,
			Topic:     r.Topic,
			Key:       r.Key,
			Payload:   r.Payload,
			CreatedAt: r.CreatedAt.Time,
		}
	}
	return records, nil
}

func (a *outboxStoreAdapter) MarkProcessed(ctx context.Context, ids []int64) (int64, error) {
	return a.queries.MarkOutboxRecordsProcessed(ctx, ids)
}

func (a *outboxStoreAdapter) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	return a.queries.DeleteOutboxRecordsBefore(ctx, pgtype.Timestamptz{Time: before, Valid: true})
}
