// Package svc provides the outbox Store adapter for auth service.
package svc

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/outbox"
)

// outboxStoreAdapter adapts the GORM DAO to outbox.Store.
type outboxStoreAdapter struct {
	store *dao.Store
}

// NewOutboxStore creates an outbox.Store backed by the auth DAO.
func NewOutboxStore(store *dao.Store) outbox.Store {
	if store == nil {
		return nil
	}
	return &outboxStoreAdapter{store: store}
}

func (a *outboxStoreAdapter) Insert(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	return a.store.InsertOutboxRecord(ctx, topic, key, payload)
}

func (a *outboxStoreAdapter) FetchPending(ctx context.Context, batchSize int) ([]outbox.Record, error) {
	rows, err := a.store.FetchPendingOutboxRecords(ctx, batchSize)
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
			CreatedAt: r.CreatedAt,
		}
	}
	return records, nil
}

func (a *outboxStoreAdapter) MarkProcessed(ctx context.Context, ids []int64) (int64, error) {
	return a.store.MarkOutboxRecordsProcessed(ctx, ids)
}

func (a *outboxStoreAdapter) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	return a.store.DeleteOutboxRecordsBefore(ctx, before)
}
