// Package dao is the GORM data-access layer for the aim_auth database.
//
// Every method is the only place that knows SQL/table details for its
// aggregate. Business code depends on *Store and the orm structs; migrations
// remain the single source of truth for the schema.
package dao

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/orm"
	"gorm.io/gorm"
)

// Sentinel errors exposed to business code.
var (
	ErrNotFound     = gorm.ErrRecordNotFound
	ErrDuplicateKey = gorm.ErrDuplicatedKey
)

// Store is the GORM-backed data access object for aim_auth.
type Store struct {
	db *gorm.DB
}

// New wraps an existing GORM handle.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying handle for health checks and transactions.
func (s *Store) DB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// WithTx runs fn inside a transaction. The callback receives a Store bound to
// the transaction; nested calls to WithTx on that Store reuse the same tx.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s == nil || s.db == nil {
		return errors.New("auth dao: database is not configured")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

// CreateUser inserts a normal (enabled) credential.
func (s *Store) CreateUser(ctx context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error) {
	return s.createCredential(ctx, id, email, passwordHash, name, 1)
}

// CreateBotCredential inserts a disabled credential used as a Bot identity.
func (s *Store) CreateBotCredential(ctx context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error) {
	return s.createCredential(ctx, id, email, passwordHash, name, 0)
}

func (s *Store) createCredential(ctx context.Context, id int64, email, passwordHash, name string, status int16) (orm.UserCredential, error) {
	user := orm.UserCredential{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		Status:       status,
	}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		return orm.UserCredential{}, translate(err)
	}

	return user, nil
}

// GetUserByEmail loads a credential by email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (orm.UserCredential, error) {
	var user orm.UserCredential
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return orm.UserCredential{}, translate(err)
	}

	return user, nil
}

// InsertOutboxRecord appends a pending outbox row and returns its id.
func (s *Store) InsertOutboxRecord(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	record := orm.OutboxRecord{Topic: topic, Key: key, Payload: payload}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return 0, translate(err)
	}

	return record.ID, nil
}

func translate(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return ErrDuplicateKey
	default:
		return err
	}
}
