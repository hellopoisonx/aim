package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"gorm.io/gorm"
)

// InsertOutboxRecord appends a pending outbox row and returns its id.
func (s *Store) InsertOutboxRecord(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	record := orm.OutboxRecord{Topic: topic, Key: key, Payload: payload}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return 0, translate(err)
	}

	return record.ID, nil
}

// FetchPendingOutboxRecords returns unprocessed rows ordered by created_at.
func (s *Store) FetchPendingOutboxRecords(ctx context.Context, limit int) ([]orm.OutboxRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("logic dao: limit must be positive")
	}

	var records []orm.OutboxRecord
	if err := s.db.WithContext(ctx).
		Where("processed_at IS NULL").
		Order("created_at ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, translate(err)
	}

	return records, nil
}

// MarkOutboxRecordsProcessed stamps processed_at = NOW() for the given ids.
func (s *Store) MarkOutboxRecordsProcessed(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	result := s.db.WithContext(ctx).
		Model(&orm.OutboxRecord{}).
		Where("id IN ?", ids).
		Update("processed_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return 0, translate(result.Error)
	}

	return result.RowsAffected, nil
}

// DeleteOutboxRecordsBefore removes processed rows older than before.
func (s *Store) DeleteOutboxRecordsBefore(ctx context.Context, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("processed_at IS NOT NULL AND processed_at < ?", before).
		Delete(&orm.OutboxRecord{})
	if result.Error != nil {
		return 0, translate(result.Error)
	}

	return result.RowsAffected, nil
}
