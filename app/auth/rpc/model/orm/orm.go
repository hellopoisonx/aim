// Package orm holds GORM model definitions for the aim_auth database.
// Structs mirror app/auth/rpc/model/migrations 1:1. Migration SQL remains
// the single source of truth for the schema; never run AutoMigrate.
package orm

import "time"

// UserCredential maps table user_credentials (000_init.sql, 001_require_user_name.sql).
type UserCredential struct {
	ID           int64  `gorm:"column:id;primaryKey" json:"id"`
	Email        string `gorm:"column:email;size:255;not null;uniqueIndex" json:"email"`
	PasswordHash string `gorm:"column:password_hash;size:255;not null" json:"password_hash"`
	// Status is always written explicitly; 0 is a valid disabled/bot credential state.
	Status    int16     `gorm:"column:status;not null" json:"status"`
	Name      string    `gorm:"column:name;size:255;not null" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (UserCredential) TableName() string { return "user_credentials" }

// OutboxRecord maps table outbox_records (002_outbox_records.sql).
type OutboxRecord struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Topic       string     `gorm:"column:topic;not null" json:"topic"`
	Key         string     `gorm:"column:key;not null;default:''" json:"key"`
	Payload     []byte     `gorm:"column:payload;type:bytea;not null" json:"payload"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	ProcessedAt *time.Time `gorm:"column:processed_at" json:"processed_at"`
}

// TableName pins the physical table name.
func (OutboxRecord) TableName() string { return "outbox_records" }
