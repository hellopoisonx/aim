package outbox

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// outboxRecord is the shared row model for the outbox_records table.
//
// Every AIM service creates the same table (see the package documentation in
// outbox.go); keeping the model here means services no longer need their own
// copy of the DAO queries.
type outboxRecord struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Topic       string     `gorm:"column:topic;not null"`
	Key         string     `gorm:"column:key;not null;default:''"`
	Payload     []byte     `gorm:"column:payload;type:bytea;not null"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
}

// TableName pins the physical table name.
func (outboxRecord) TableName() string { return "outbox_records" }

// GORMStore is the standard Store implementation over a *gorm.DB.
//
// Services construct one per process, e.g.
//
//	poller := outbox.NewPoller(outbox.NewGORMStore(store.DB()), publish, cfg)
//
// Business writes must still insert outbox rows inside their own transaction
// (via the service DAO); GORMStore only owns the delivery-side operations.
type GORMStore struct {
	db *gorm.DB
}

// NewGORMStore wraps db. A nil handle yields a nil store so callers can wire
// the poller unconditionally.
func NewGORMStore(db *gorm.DB) *GORMStore {
	if db == nil {
		return nil
	}
	return &GORMStore{db: db}
}

// Insert appends a pending outbox row and returns its id.
func (s *GORMStore) Insert(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	record := outboxRecord{Topic: topic, Key: key, Payload: payload}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return 0, err
	}

	return record.ID, nil
}

// FetchPending returns unprocessed rows ordered by created_at.
func (s *GORMStore) FetchPending(ctx context.Context, batchSize int) ([]Record, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("outbox: batch size must be positive")
	}

	var rows []outboxRecord
	if err := s.db.WithContext(ctx).
		Where("processed_at IS NULL").
		Order("created_at ASC").
		Limit(batchSize).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	records := make([]Record, len(rows))
	for i, row := range rows {
		records[i] = Record{
			ID:        row.ID,
			Topic:     row.Topic,
			Key:       row.Key,
			Payload:   row.Payload,
			CreatedAt: row.CreatedAt,
		}
	}

	return records, nil
}

// MarkProcessed stamps processed_at = NOW() for the given ids.
func (s *GORMStore) MarkProcessed(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	result := s.db.WithContext(ctx).
		Model(&outboxRecord{}).
		Where("id IN ?", ids).
		Update("processed_at", gorm.Expr("NOW()"))
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// DeleteBefore removes processed rows older than before.
func (s *GORMStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("processed_at IS NOT NULL AND processed_at < ?", before).
		Delete(&outboxRecord{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// compile-time guard.
var _ Store = (*GORMStore)(nil)
