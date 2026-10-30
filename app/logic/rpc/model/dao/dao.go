// Package dao is the GORM data-access layer for the aim_logic database.
//
// Business services depend on this package instead of hand-written SQL. All
// table/column knowledge stays here; migrations remain the single source of
// truth for the schema.
package dao

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Sentinel errors exposed to business code.
var (
	ErrNotFound     = gorm.ErrRecordNotFound
	ErrDuplicateKey = gorm.ErrDuplicatedKey
)

// Store is the GORM-backed data access object for aim_logic.
type Store struct {
	db *gorm.DB
}

// New wraps an existing GORM handle.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying handle for health checks and advanced callers.
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
		return errors.New("logic dao: database is not configured")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}
