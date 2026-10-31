package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"gorm.io/gorm"
)

// UserRegistrarStore is the registration entry point RPC logic depends on.
// *UserRegistrar implements it; tests can substitute a fake.
type UserRegistrarStore interface {
	CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error)
}

// TransactionalSender publishes an event atomically with its local database
// transaction. *rocketmq.Producer implements it; tests substitute a fake.
type TransactionalSender interface {
	SendTransactional(ctx context.Context, db *gorm.DB, message rocketmq.TransactionalMessage, localTx func(ctx context.Context, tx *gorm.DB) error) error
}

// UserRegistrar creates users and publishes user.created as a RocketMQ
// transactional message, so the event becomes visible exactly when the
// credential row commits.
//
// It exists so RPC logic does not have to open transactions or build events:
// registration is one domain operation.
type UserRegistrar struct {
	db     *gorm.DB
	idGen  *tools.Snowflake
	sender TransactionalSender
	topic  string
}

// NewUserRegistrar builds a registrar. All dependencies are required; the
// service context only constructs one when transactional publishing is
// configured.
func NewUserRegistrar(db *gorm.DB, idGen *tools.Snowflake, sender TransactionalSender, topic string) *UserRegistrar {
	if db == nil || idGen == nil || sender == nil || topic == "" {
		return nil
	}
	return &UserRegistrar{db: db, idGen: idGen, sender: sender, topic: topic}
}

// CreateUser inserts the credential and resolves the user.created half message
// with the same commit.
func (r *UserRegistrar) CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error) {
	// The half message body is fixed before the transaction opens, so the id and
	// the payload are built up front.
	id, err := r.idGen.NextID()
	if err != nil {
		return UserCredential{}, err
	}

	payload, err := json.Marshal(events.UserCreatedEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		UserID:             id,
		Email:              email,
		Nickname:           name,
		CreatedAt:          time.Now().UnixMilli(),
	})
	if err != nil {
		return UserCredential{}, err
	}

	eventUID := UserCreatedEventUID(id)
	var user UserCredential
	err = r.sender.SendTransactional(ctx, r.db, rocketmq.TransactionalMessage{
		Topic:    r.topic,
		EventUID: eventUID,
		Key:      strconv.FormatInt(id, 10),
		Body:     payload,
	}, func(ctx context.Context, tx *gorm.DB) error {
		created, err := dao.New(tx).CreateUser(ctx, id, email, passwordHash, name)
		if err != nil {
			return err
		}

		user = userCredentialFromORM(created)
		return nil
	})
	if err != nil {
		return UserCredential{}, err
	}

	return user, nil
}

// UserCreatedEventUID is the deterministic event identity of user.created. It is
// the event_journal primary key and the message key used for transaction
// check-back.
func UserCreatedEventUID(userID int64) string {
	return "user.created:" + strconv.FormatInt(userID, 10)
}
