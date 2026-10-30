package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/hellopoisonx/aim/app/shared/tracing"
)

// UserRegistrarStore is the registration entry point RPC logic depends on.
// *UserRegistrar implements it; tests can substitute a fake.
type UserRegistrarStore interface {
	CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error)
}

// TxRunner runs a callback inside a database transaction.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(tx *dao.Store) error) error
}

// UserRegistrar creates users with the UserCreated event recorded in the
// outbox, inside the same transaction as the credential row.
//
// It exists so RPC logic does not have to open transactions, build events or
// poke the outbox poller itself: registration is one domain operation.
type UserRegistrar struct {
	txRunner TxRunner
	idGen    *tools.Snowflake
	topic    string
	wake     func()
}

// NewUserRegistrar builds a registrar. All four dependencies are required; the
// service context only constructs one when outbox publishing is configured.
func NewUserRegistrar(txRunner TxRunner, idGen *tools.Snowflake, topic string, wake func()) *UserRegistrar {
	if txRunner == nil || idGen == nil || topic == "" {
		return nil
	}
	if wake == nil {
		wake = func() {}
	}

	return &UserRegistrar{txRunner: txRunner, idGen: idGen, topic: topic, wake: wake}
}

// CreateUser inserts the credential and its outbox event atomically, then wakes
// the outbox poller so the event is delivered without waiting for the next tick.
func (r *UserRegistrar) CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error) {
	var user UserCredential
	err := r.txRunner.WithTx(ctx, func(tx *dao.Store) error {
		created, err := NewDAOUserStoreWithIDGenerator(tx, r.idGen).CreateUser(ctx, email, passwordHash, name)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(events.UserCreatedEvent{
			TraceContextFields: tracing.InjectTraceContext(ctx),
			UserID:             created.ID,
			Email:              email,
			Nickname:           name,
			CreatedAt:          time.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}

		if _, err := tx.InsertOutboxRecord(ctx, r.topic, strconv.FormatInt(created.ID, 10), payload); err != nil {
			return err
		}

		user = created
		return nil
	})
	if err != nil {
		return UserCredential{}, err
	}

	r.wake()

	return user, nil
}
