package dao

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

// InsertOutboxRecord appends a pending outbox row and returns its id.
func (s *Store) InsertOutboxRecord(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	record := orm.OutboxRecord{Topic: topic, Key: key, Payload: payload}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return 0, err
	}

	return record.ID, nil
}
