package rocketmq

import (
	"context"
	"errors"
	"fmt"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// TransactionalMessage is an event that must be published only if the local
// database transaction commits.
type TransactionalMessage struct {
	// Topic is the destination topic; it must be declared as a TRANSACTION
	// topic.
	Topic string
	// EventUID is the deterministic event identity (domain:aggregate-id). It is
	// both the message key used for check-back and the event_journal primary
	// key, which makes retries idempotent.
	EventUID string
	// Key additionally lands in the message keys so consumers see the domain
	// key (user id, conversation id, file id) as keys[1].
	Key string
	// ShardingKey is the FIFO message group. A transactional topic cannot be
	// FIFO, so this is normally empty.
	ShardingKey string
	// Body is the JSON payload.
	Body []byte
}

// journalTx runs localTx together with the event_journal insert inside one
// database transaction. It is a field on Producer so tests can drive the
// commit/rollback decision without a live database.
type journalTx func(ctx context.Context, db *gorm.DB, journal EventJournal, localTx func(ctx context.Context, tx *gorm.DB) error) error

// gormJournalTx is the production journalTx: business rows and the journal row
// commit or roll back together.
func gormJournalTx(ctx context.Context, db *gorm.DB, journal EventJournal, localTx func(ctx context.Context, tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := InsertJournal(ctx, tx, journal); err != nil {
			return fmt.Errorf("rocketmq: record event journal %s: %w", journal.EventUID, err)
		}
		return localTx(ctx, tx)
	})
}

// SendTransactional publishes m atomically with localTx.
//
// Order of operations (the broker protocol):
//
//  1. send the half message — invisible to consumers until resolved;
//  2. run localTx (plus the event_journal row) in one database transaction;
//  3. commit the half message, or roll it back when localTx failed.
//
// If step 3 is lost (network, crash) the broker later asks this producer, which
// answers from event_journal. If step 1 fails the local transaction never runs,
// so callers see the error and no business row is written.
func (p *Producer) SendTransactional(ctx context.Context, db *gorm.DB, m TransactionalMessage, localTx func(ctx context.Context, tx *gorm.DB) error) error {
	if p == nil {
		return errors.New("rocketmq: transactional publish requires a producer")
	}
	if db == nil {
		return errors.New("rocketmq: transactional publish requires a database handle")
	}
	if m.Topic == "" || m.EventUID == "" {
		return errors.New("rocketmq: transactional publish requires topic and event_uid")
	}
	if localTx == nil {
		return errors.New("rocketmq: transactional publish requires a local transaction")
	}

	keys := []string{m.EventUID}
	if m.Key != "" && m.Key != m.EventUID {
		keys = append(keys, m.Key)
	}
	msg := newSDKMessage(m.Topic, m.Body, "", m.ShardingKey)
	msg.SetKeys(keys...)

	transaction := p.producer.BeginTransaction()
	if _, err := p.producer.SendWithTransaction(ctx, msg, transaction); err != nil {
		return fmt.Errorf("rocketmq: send half message %s to %s: %w", m.EventUID, m.Topic, err)
	}

	err := p.journalTx(ctx, db, EventJournal{EventUID: m.EventUID, Topic: m.Topic, MsgKey: m.Key}, localTx)
	if err != nil {
		if rollbackErr := transaction.RollBack(); rollbackErr != nil {
			// The unresolved half message is discarded by the broker's check-back
			// (no journal row exists), so this is logged rather than surfaced.
			logx.WithContext(ctx).Errorf("rocketmq: rollback half message %s failed: %v", m.EventUID, rollbackErr)
		}
		return err
	}

	if commitErr := transaction.Commit(); commitErr != nil {
		// The rows are committed; the broker's check-back finds the journal row and
		// commits the message on its own.
		logx.WithContext(ctx).Errorf("rocketmq: commit half message %s failed, awaiting check-back: %v", m.EventUID, commitErr)
	}
	return nil
}
