package service

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/attachment/model/dao"
	"github.com/hellopoisonx/aim/app/shared/outbox"
)

// daoOutboxStore adapts the attachment DAO to outbox.Store.
type daoOutboxStore struct {
	store *dao.Store
}

func newDAOOutboxStore(store *dao.Store) *daoOutboxStore {
	return &daoOutboxStore{store: store}
}

func (s *daoOutboxStore) Insert(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	return s.store.InsertOutboxRecord(ctx, topic, key, payload)
}

func (s *daoOutboxStore) FetchPending(ctx context.Context, batchSize int) ([]outbox.Record, error) {
	rows, err := s.store.FetchPendingOutboxRecords(ctx, batchSize)
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

func (s *daoOutboxStore) MarkProcessed(ctx context.Context, ids []int64) (int64, error) {
	return s.store.MarkOutboxRecordsProcessed(ctx, ids)
}

func (s *daoOutboxStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	return s.store.DeleteOutboxRecordsBefore(ctx, before)
}
