package dao

// Integration tests for the auth GORM DAO.
//
// They run against a real PostgreSQL because the layer they protect is the SQL:
// the unique email index, the NOT NULL name column and transaction scoping
// cannot be validated with an in-memory fake.
//
// Usage:
//
//	AIM_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=aim password=aim dbname=aim_test sslmode=disable' \
//	  go test ./app/auth/rpc/model/dao/ -run Integration
//
// Without the variable the whole file is skipped, so `go test ./...` stays
// hermetic.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

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
	require.NoError(t, db.Exec("TRUNCATE TABLE user_credentials").Error)

	return New(db)
}

func applyMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "migrations"))
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

func TestIntegrationCreateUserRoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	created, err := store.CreateUser(ctx, 101, "ada@example.com", "hash", "Ada")
	require.NoError(t, err)
	require.Equal(t, int64(101), created.ID)
	require.EqualValues(t, 1, created.Status)

	found, err := store.GetUserByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, int64(101), found.ID)
	require.Equal(t, "hash", found.PasswordHash)
	require.Equal(t, "Ada", found.Name)
	require.EqualValues(t, 1, found.Status)
}

func TestIntegrationCreateBotCredentialIsDisabled(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	created, err := store.CreateBotCredential(ctx, 202, "bot@example.com", "hash", "Bot")
	require.NoError(t, err)
	require.EqualValues(t, 0, created.Status)

	found, err := store.GetUserByEmail(ctx, "bot@example.com")
	require.NoError(t, err)
	require.EqualValues(t, 0, found.Status)
}

func TestIntegrationDuplicateEmailAndMissingUser(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	_, err := store.CreateUser(ctx, 301, "dup@example.com", "hash", "Dup")
	require.NoError(t, err)

	_, err = store.CreateUser(ctx, 302, "dup@example.com", "hash", "Dup Two")
	require.ErrorIs(t, err, ErrDuplicateKey)

	_, err = store.GetUserByEmail(ctx, "nobody@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestIntegrationWithTxScopesWrites(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	rollbackErr := errors.New("roll back")
	err := store.WithTx(ctx, func(tx *Store) error {
		_, createErr := tx.CreateUser(ctx, 401, "rolled-back@example.com", "hash", "Rollback")
		require.NoError(t, createErr)

		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)

	_, err = store.GetUserByEmail(ctx, "rolled-back@example.com")
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, store.WithTx(ctx, func(tx *Store) error {
		_, createErr := tx.CreateUser(ctx, 402, "committed@example.com", "hash", "Committed")

		return createErr
	}))

	_, err = store.GetUserByEmail(ctx, "committed@example.com")
	require.NoError(t, err)
}
