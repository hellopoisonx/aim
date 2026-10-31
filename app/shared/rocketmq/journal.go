package rocketmq

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EventJournal is the row a transactional publish writes next to the business
// data. It is the transaction check-back authority: the broker asks whether a
// half message may be committed, and the answer is "does this row exist".
//
// Each service owning transactional events creates the table with
//
//	CREATE TABLE IF NOT EXISTS event_journal (
//	    event_uid  TEXT PRIMARY KEY,
//	    topic      TEXT NOT NULL,
//	    msg_key    TEXT NOT NULL DEFAULT '',
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//
// It is a journal, not a queue: no processed marker, no cleanup loop.
type EventJournal struct {
	EventUID  string    `gorm:"column:event_uid;primaryKey"`
	Topic     string    `gorm:"column:topic;not null"`
	MsgKey    string    `gorm:"column:msg_key;not null;default:''"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

// TableName pins the physical table name.
func (EventJournal) TableName() string { return "event_journal" }

// InsertJournal records a published event inside tx. The insert is idempotent
// on the primary key: a retried transaction that already wrote the row is a
// no-op rather than a duplicate-key failure.
func InsertJournal(ctx context.Context, tx *gorm.DB, journal EventJournal) error {
	if tx == nil {
		return errors.New("rocketmq: nil transaction handle")
	}
	if journal.EventUID == "" {
		return errors.New("rocketmq: event_uid is required")
	}
	if journal.CreatedAt.IsZero() {
		journal.CreatedAt = time.Now()
	}
	return tx.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_uid"}}, DoNothing: true}).
		Create(&journal).Error
}

// JournalExists reports whether eventUID has been committed. It is the check
// executed by the transaction check-back.
func JournalExists(ctx context.Context, db *gorm.DB, eventUID string) (bool, error) {
	if db == nil {
		return false, errors.New("rocketmq: nil database handle")
	}
	if eventUID == "" {
		return false, errors.New("rocketmq: event_uid is required")
	}
	var row EventJournal
	err := db.WithContext(ctx).Select("event_uid").Where("event_uid = ?", eventUID).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
