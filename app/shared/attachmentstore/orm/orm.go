// Package orm holds the GORM models of the aim_attachment database.
// Structs mirror app/attachment/model/migrations 1:1. Migration SQL remains
// the single source of truth for the schema; never run AutoMigrate.
package orm

import (
	"time"

	"github.com/hellopoisonx/aim/app/shared/pgtypes"

	"github.com/google/uuid"
)

// AttachmentFile maps table attachment_files (000_attachments.sql).
type AttachmentFile struct {
	FileID         uuid.UUID  `gorm:"column:file_id;type:uuid;primaryKey" json:"file_id"`
	OwnerID        int64      `gorm:"column:owner_id;not null" json:"owner_id"`
	ConversationID int64      `gorm:"column:conversation_id;not null" json:"conversation_id"`
	Kind           string     `gorm:"column:kind;size:16;not null" json:"kind"`
	OriginalName   string     `gorm:"column:original_name;not null" json:"original_name"`
	Mime           string     `gorm:"column:mime;not null" json:"mime"`
	SizeBytes      int64      `gorm:"column:size_bytes;not null" json:"size_bytes"`
	Sha256         *string    `gorm:"column:sha256" json:"sha256"`
	Status         string     `gorm:"column:status;size:16;not null;default:pending" json:"status"`
	ParseStatus    string     `gorm:"column:parse_status;size:16;not null;default:pending" json:"parse_status"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UploadedAt     *time.Time `gorm:"column:uploaded_at" json:"uploaded_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (AttachmentFile) TableName() string { return "attachment_files" }

// AttachmentObject maps table attachment_objects (000).
type AttachmentObject struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FileID     uuid.UUID `gorm:"column:file_id;type:uuid;not null;uniqueIndex:attachment_objects_file_id_object_role_key,priority:1" json:"file_id"`
	ObjectRole string    `gorm:"column:object_role;size:32;not null;uniqueIndex:attachment_objects_file_id_object_role_key,priority:2" json:"object_role"`
	Bucket     string    `gorm:"column:bucket;not null;uniqueIndex:attachment_objects_bucket_object_key_key,priority:1" json:"bucket"`
	ObjectKey  string    `gorm:"column:object_key;not null;uniqueIndex:attachment_objects_bucket_object_key_key,priority:2" json:"object_key"`
	Mime       string    `gorm:"column:mime;not null" json:"mime"`
	SizeBytes  *int64    `gorm:"column:size_bytes" json:"size_bytes"`
	Sha256     *string   `gorm:"column:sha256" json:"sha256"`
	Width      *int32    `gorm:"column:width" json:"width"`
	Height     *int32    `gorm:"column:height" json:"height"`
	DurationMs *int64    `gorm:"column:duration_ms" json:"duration_ms"`
	CreatedAt  time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (AttachmentObject) TableName() string { return "attachment_objects" }

// AttachmentReference maps table attachment_references (000).
// UNIQUE(file_id, message_id) allows duplicate NULL message_id in PostgreSQL.
type AttachmentReference struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FileID         uuid.UUID `gorm:"column:file_id;type:uuid;not null;uniqueIndex:attachment_references_file_id_message_id_key,priority:1" json:"file_id"`
	MessageID      *int64    `gorm:"column:message_id;uniqueIndex:attachment_references_file_id_message_id_key,priority:2" json:"message_id"`
	ConversationID int64     `gorm:"column:conversation_id;not null" json:"conversation_id"`
	SenderID       int64     `gorm:"column:sender_id;not null" json:"sender_id"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName pins the physical table name.
func (AttachmentReference) TableName() string { return "attachment_references" }

// AttachmentParseJob maps table attachment_parse_jobs (000).
type AttachmentParseJob struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FileID      uuid.UUID `gorm:"column:file_id;type:uuid;not null" json:"file_id"`
	Status      string    `gorm:"column:status;size:16;not null;default:pending" json:"status"`
	Attempts    int32     `gorm:"column:attempts;not null;default:0" json:"attempts"`
	LastError   *string   `gorm:"column:last_error" json:"last_error"`
	AvailableAt time.Time `gorm:"column:available_at;not null;default:now()" json:"available_at"`
	CreatedAt   time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (AttachmentParseJob) TableName() string { return "attachment_parse_jobs" }

// AttachmentParseResult maps table attachment_parse_results (000).
type AttachmentParseResult struct {
	FileID            uuid.UUID    `gorm:"column:file_id;type:uuid;primaryKey" json:"file_id"`
	ThumbnailObjectID *int64       `gorm:"column:thumbnail_object_id" json:"thumbnail_object_id"`
	DurationMs        *int64       `gorm:"column:duration_ms" json:"duration_ms"`
	Width             *int32       `gorm:"column:width" json:"width"`
	Height            *int32       `gorm:"column:height" json:"height"`
	Metadata          pgtypes.JSON `gorm:"column:metadata;type:jsonb;not null;default:'{}'" json:"metadata"`
	Error             *string      `gorm:"column:error" json:"error"`
	CreatedAt         time.Time    `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt         time.Time    `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName pins the physical table name.
func (AttachmentParseResult) TableName() string { return "attachment_parse_results" }
