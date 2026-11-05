package dao

// Integration tests for the shared attachment GORM DAO.
//
// They run against a real PostgreSQL because the SQL is what they protect: the
// status transition in MarkUploaded, the owner/status visibility rule in GetFile
// and the pending-only filter of GetPendingOriginal.
//
// Usage:
//
//	AIM_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=aim password=aim dbname=aim_test sslmode=disable' \
//	  go test ./app/shared/attachmentstore/dao/ -run Integration
//
// Without the variable the whole file is skipped, so `go test ./...` stays
// hermetic.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/orm"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testDSNEnv = "AIM_TEST_POSTGRES_DSN"

func openTestStore(t *testing.T) *Store {
	t.Helper()

	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", testDSNEnv)
	}

	db, err := gormdb.Open(dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	applyMigrations(t, db)
	require.NoError(t, db.Exec("TRUNCATE TABLE attachment_files CASCADE").Error)

	return New(db)
}

func applyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	// The attachment migrations live with the service that owns the schema.
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "attachment", "rpc", "model", "migrations"))
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	require.NotEmpty(t, names)

	for _, name := range names {
		// Path comes from listing the repo's own migrations directory.
		sqlBytes, readErr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // path comes from os.ReadDir of the repo's migrations dir.
		require.NoError(t, readErr)
		require.NoError(t, db.Exec(string(sqlBytes)).Error, "migration %s", name)
	}
}

func seedPendingFile(t *testing.T, store *Store, fileID uuid.UUID, ownerID int64, kind string) {
	t.Helper()

	ctx := context.Background()
	size := int64(10)
	require.NoError(t, store.CreateAttachmentFile(ctx, orm.AttachmentFile{
		FileID:         fileID,
		OwnerID:        ownerID,
		ConversationID: 7,
		Kind:           kind,
		OriginalName:   "report.pdf",
		Mime:           "application/pdf",
		SizeBytes:      size,
		Status:         sharedattachment.UploadStatusPending,
		ParseStatus:    sharedattachment.ParseStatusPending,
	}))
	require.NoError(t, store.CreateAttachmentObject(ctx, orm.AttachmentObject{
		FileID:     fileID,
		ObjectRole: "original",
		Bucket:     "aim-attachments",
		ObjectKey:  "7/" + fileID.String() + "/original",
		Mime:       "application/pdf",
		SizeBytes:  &size,
	}))
}

func TestIntegrationMarkUploaded(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	fileID := uuid.New()
	seedPendingFile(t, store, fileID, 1, sharedattachment.KindFile)

	rows, err := store.MarkUploaded(ctx, fileID, 2, "deadbeef", sharedattachment.KindFile)
	require.NoError(t, err)
	require.Zero(t, rows, "a different owner must not upload the file")

	rows, err = store.MarkUploaded(ctx, fileID, 1, "deadbeef", sharedattachment.KindFile)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	row, err := store.GetFile(ctx, fileID, 1)
	require.NoError(t, err)
	require.Equal(t, sharedattachment.UploadStatusUploaded, row.Status)
	require.Equal(t, sharedattachment.ParseStatusReady, row.ParseStatus)
	require.Equal(t, "deadbeef", row.Sha256)

	rows, err = store.MarkUploaded(ctx, fileID, 1, "cafebabe", sharedattachment.KindFile)
	require.NoError(t, err)
	require.Zero(t, rows, "an uploaded file is no longer pending")
}

func TestIntegrationGetPendingOriginal(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	fileID := uuid.New()
	seedPendingFile(t, store, fileID, 3, sharedattachment.KindImage)

	row, err := store.GetPendingOriginal(ctx, fileID, 3)
	require.NoError(t, err)
	require.Equal(t, "7/"+fileID.String()+"/original", row.ObjectKey)
	require.Equal(t, int64(10), row.SizeBytes)

	_, err = store.GetPendingOriginal(ctx, fileID, 4)
	require.ErrorIs(t, err, ErrNotFound, "owner is part of the pending filter")

	_, err = store.MarkUploaded(ctx, fileID, 3, "", sharedattachment.KindFile)
	require.NoError(t, err)

	_, err = store.GetPendingOriginal(ctx, fileID, 3)
	require.ErrorIs(t, err, ErrNotFound, "uploaded files are not pending")
}

func TestIntegrationGetFileVisibility(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	fileID := uuid.New()
	seedPendingFile(t, store, fileID, 5, sharedattachment.KindFile)

	_, err := store.GetFile(ctx, fileID, 6)
	require.ErrorIs(t, err, ErrNotFound, "a pending file is only visible to its owner")

	_, err = store.MarkUploaded(ctx, fileID, 5, "", sharedattachment.KindFile)
	require.NoError(t, err)

	row, err := store.GetFile(ctx, fileID, 6)
	require.NoError(t, err)
	require.Equal(t, sharedattachment.UploadStatusUploaded, row.Status)
	require.Equal(t, int64(5), row.OwnerID)
}

func TestIntegrationSetParseStatusAndResultError(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	fileID := uuid.New()
	seedPendingFile(t, store, fileID, 7, sharedattachment.KindFile)

	rows, err := store.SetParseStatus(ctx, fileID, sharedattachment.ParseStatusFailed)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	require.NoError(t, store.SetParseResultError(ctx, fileID, "ocr failed"))
	require.NoError(t, store.SetParseResultError(ctx, fileID, "ocr failed again"))

	row, err := store.GetFile(ctx, fileID, 7)
	require.NoError(t, err)
	require.Equal(t, sharedattachment.ParseStatusFailed, row.ParseStatus)
}
