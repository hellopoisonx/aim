// Package dao is the GORM data-access layer for the aim_attachment database.
//
// It lives under app/shared because two processes touch these tables: the
// attachment service and the data_parsing worker. Keeping one store here avoids
// one service importing the other's internal package tree.
package dao

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/shared/pgtypes"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/orm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sentinel errors exposed to business code.
var (
	ErrNotFound     = gorm.ErrRecordNotFound
	ErrDuplicateKey = gorm.ErrDuplicatedKey
)

// Store is the GORM-backed data access object for aim_attachment.
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

// WithTx runs fn inside a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s == nil || s.db == nil {
		return errors.New("attachment dao: database is not configured")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

// PendingFile is the file row joined with its original object.
type PendingFile struct {
	FileID         uuid.UUID
	OwnerID        int64
	ConversationID int64
	Kind           string
	OriginalName   string
	Mime           string
	SizeBytes      int64
	Sha256         string
	Status         string
	ParseStatus    string
	Bucket         string
	ObjectKey      string
}

// FileRow is the full file view used by GetFile.
type FileRow struct {
	FileID             uuid.UUID
	OwnerID            int64
	ConversationID     int64
	Kind               string
	OriginalName       string
	Mime               string
	SizeBytes          int64
	Sha256             string
	Status             string
	ParseStatus        string
	Bucket             string
	ObjectKey          string
	ThumbnailObjectKey string
	DurationMS         int64
	Width              int32
	Height             int32
	Metadata           pgtypes.JSON
}

// CreateAttachmentFile inserts the attachment_files row.
func (s *Store) CreateAttachmentFile(ctx context.Context, file orm.AttachmentFile) error {
	if err := s.db.WithContext(ctx).Create(&file).Error; err != nil {
		return err
	}

	return nil
}

// CreateAttachmentObject inserts an attachment_objects row.
func (s *Store) CreateAttachmentObject(ctx context.Context, object orm.AttachmentObject) error {
	if err := s.db.WithContext(ctx).Create(&object).Error; err != nil {
		return err
	}

	return nil
}

// GetPendingOriginal loads a pending file joined with its original object.
func (s *Store) GetPendingOriginal(ctx context.Context, fileID uuid.UUID, ownerID int64) (PendingFile, error) {
	const query = `
SELECT f.file_id, f.owner_id, f.conversation_id, f.kind, f.original_name, f.mime, f.size_bytes,
       COALESCE(f.sha256, '') AS sha256, f.status, f.parse_status, o.bucket, o.object_key
FROM attachment_files f
JOIN attachment_objects o ON o.file_id = f.file_id AND o.object_role = 'original'
WHERE f.file_id = ? AND f.owner_id = ? AND f.status = 'pending'`

	var row PendingFile
	if err := s.db.WithContext(ctx).Raw(query, fileID, ownerID).Scan(&row).Error; err != nil {
		return PendingFile{}, err
	}
	if row.FileID == uuid.Nil {
		return PendingFile{}, ErrNotFound
	}

	return row, nil
}

// MarkUploaded finalizes a pending upload and returns affected rows.
func (s *Store) MarkUploaded(ctx context.Context, fileID uuid.UUID, ownerID int64, sha256, kindFile string) (int64, error) {
	const query = `
UPDATE attachment_files
SET status = 'uploaded', uploaded_at = NOW(), updated_at = NOW(),
    sha256 = COALESCE(NULLIF(?, ''), sha256),
    parse_status = CASE WHEN kind = ? THEN 'ready' ELSE parse_status END
WHERE file_id = ? AND owner_id = ? AND status = 'pending'`

	result := s.db.WithContext(ctx).Exec(query, sha256, kindFile, fileID, ownerID)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GetFile loads the file with its original/thumbnail objects and parse result.
func (s *Store) GetFile(ctx context.Context, fileID uuid.UUID, userID int64) (FileRow, error) {
	const query = `
SELECT f.file_id, f.owner_id, f.conversation_id, f.kind, f.original_name, f.mime, f.size_bytes,
       COALESCE(f.sha256, '') AS sha256, f.status, f.parse_status, o.bucket, o.object_key,
       COALESCE(t.object_key, '') AS thumbnail_object_key,
       COALESCE(r.duration_ms, 0) AS duration_ms,
       COALESCE(r.width, 0) AS width,
       COALESCE(r.height, 0) AS height,
       COALESCE(r.metadata, '{}'::jsonb) AS metadata
FROM attachment_files f
JOIN attachment_objects o ON o.file_id = f.file_id AND o.object_role = 'original'
LEFT JOIN attachment_objects t ON t.file_id = f.file_id AND t.object_role = 'thumbnail'
LEFT JOIN attachment_parse_results r ON r.file_id = f.file_id
WHERE f.file_id = ? AND (f.owner_id = ? OR f.status = 'uploaded')`

	var row FileRow
	if err := s.db.WithContext(ctx).Raw(query, fileID, userID).Scan(&row).Error; err != nil {
		return FileRow{}, err
	}
	if row.FileID == uuid.Nil {
		return FileRow{}, ErrNotFound
	}

	return row, nil
}

// UpsertAttachmentObject inserts or updates a derived object row.
func (s *Store) UpsertAttachmentObject(ctx context.Context, object orm.AttachmentObject) error {
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "file_id"}, {Name: "object_role"}},
		DoUpdates: clause.Assignments(map[string]any{
			"object_key":  object.ObjectKey,
			"mime":        object.Mime,
			"size_bytes":  object.SizeBytes,
			"width":       object.Width,
			"height":      object.Height,
			"duration_ms": object.DurationMs,
		}),
	}).Create(&object).Error
	if err != nil {
		return err
	}

	return nil
}

// UpsertAttachmentParseResult inserts or updates a parse result row.
func (s *Store) UpsertAttachmentParseResult(ctx context.Context, result orm.AttachmentParseResult) error {
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "file_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"duration_ms": result.DurationMs,
			"width":       result.Width,
			"height":      result.Height,
			"metadata":    result.Metadata,
			"error":       nil,
			"updated_at":  gorm.Expr("NOW()"),
		}),
	}).Create(&result).Error
	if err != nil {
		return err
	}

	return nil
}

// SetParseStatus updates attachment_files.parse_status.
func (s *Store) SetParseStatus(ctx context.Context, fileID uuid.UUID, status string) (int64, error) {
	result := s.db.WithContext(ctx).
		Model(&orm.AttachmentFile{}).
		Where("file_id = ?", fileID).
		Updates(map[string]any{"parse_status": status, "updated_at": gorm.Expr("NOW()")})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// SetParseResultError records a parse failure.
func (s *Store) SetParseResultError(ctx context.Context, fileID uuid.UUID, message string) error {
	const query = `
INSERT INTO attachment_parse_results (file_id, error, metadata)
VALUES (?, ?, '{}')
ON CONFLICT (file_id) DO UPDATE SET error = EXCLUDED.error, updated_at = NOW()`

	if err := s.db.WithContext(ctx).Exec(query, fileID, message).Error; err != nil {
		return err
	}

	return nil
}

// InsertOutboxRecord appends a pending outbox row and returns its id.
func (s *Store) InsertOutboxRecord(ctx context.Context, topic, key string, payload []byte) (int64, error) {
	record := orm.OutboxRecord{Topic: topic, Key: key, Payload: payload}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return 0, err
	}

	return record.ID, nil
}
