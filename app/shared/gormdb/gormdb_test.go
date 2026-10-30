package gormdb

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenRejectsBadDSN(t *testing.T) {
	// An unparsable DSN must fail instead of returning a half-configured handle.
	_, err := Open("this-is-not-a-dsn")
	require.Error(t, err)
}

func TestOpenAppliesConnectionPool(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a PostgreSQL server")
	}

	dsn := os.Getenv("AIM_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AIM_TEST_POSTGRES_DSN is not set")
	}

	db, err := Open(dsn)
	require.NoError(t, err)
	require.True(t, db.Config.TranslateError, "driver errors must map onto GORM sentinels")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	require.Equal(t, 20, sqlDB.Stats().MaxOpenConnections)
}
