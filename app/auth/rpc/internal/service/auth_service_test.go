package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

var errQuery = errors.New("query failed")

func TestPasswordAndEmailHelpers(t *testing.T) {
	require.Equal(t, "ada@example.com", NormalizeEmail(" Ada@Example.COM "))

	hash, err := HashPassword("password123")
	require.NoError(t, err)
	require.NotEqual(t, "password123", hash)
	require.True(t, CheckPassword(hash, "password123"))
	require.False(t, CheckPassword(hash, "wrong-password"))
}

func TestJWTIssuerIssuesValidToken(t *testing.T) {
	issuer := NewJWTIssuer("test-secret", 5*time.Minute)
	token, expiresAt, err := issuer.Issue(context.Background(), 9, "device-1")
	require.NoError(t, err)
	require.NotZero(t, expiresAt)

	claims, err := jwt.NewManager("test-secret").ValidateAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(9), claims.UserID)
	require.Equal(t, "device-1", claims.DeviceID)
}

func TestParseSessionValueRejectsInvalidData(t *testing.T) {
	_, _, err := parseSessionValue("broken")
	require.Error(t, err)

	_, _, err = parseSessionValue("x:device")
	require.Error(t, err)
}

func TestSQLUserStore(t *testing.T) {
	db := &stubDB{row: stubRow{values: []any{
		int64(11),
		"ada@example.com",
		"hash",
		"Ada",
		int16(StatusNormal),
		pgtype.Timestamptz{},
		pgtype.Timestamptz{},
	}}}
	store := NewSQLUserStore(model.New(db))

	created, err := store.CreateUser(context.Background(), "ada@example.com", "hash", "Ada")
	require.NoError(t, err)
	require.Len(t, db.args, 4)
	require.IsType(t, int64(0), db.args[0])
	require.Positive(t, db.args[0].(int64))
	require.Equal(t, "ada@example.com", db.args[1])
	require.Equal(t, "hash", db.args[2])
	require.Equal(t, "Ada", db.args[3])
	require.Equal(t, int64(11), created.ID)
	require.Equal(t, "ada@example.com", created.Email)
	require.Equal(t, "Ada", created.Name)

	found, err := store.GetUserByEmail(context.Background(), "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, int64(11), found.ID)
}

func TestSQLUserStorePropagatesQueryErrors(t *testing.T) {
	store := NewSQLUserStore(model.New(&stubDB{row: stubRow{err: errQuery}}))

	_, err := store.CreateUser(context.Background(), "ada@example.com", "hash", "Ada")
	require.ErrorIs(t, err, errQuery)

	_, err = store.GetUserByEmail(context.Background(), "ada@example.com")
	require.ErrorIs(t, err, errQuery)
}

func TestDatabaseErrorHelpers(t *testing.T) {
	require.True(t, IsDuplicateEmail(&pgconn.PgError{Code: "23505"}))
	require.False(t, IsDuplicateEmail(errors.New("other")))
	require.True(t, IsNotFound(pgx.ErrNoRows))
	require.False(t, IsNotFound(errors.New("other")))
}

type stubDB struct {
	row  stubRow
	args []any
}

func (db *stubDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (db *stubDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *stubDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	db.args = args
	return db.row
}

type stubRow struct {
	values []any
	err    error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	for i := range dest {
		switch target := dest[i].(type) {
		case *int64:
			*target = r.values[i].(int64)
		case *string:
			*target = r.values[i].(string)
		case *int16:
			*target = r.values[i].(int16)
		case *pgtype.Timestamptz:
			*target = r.values[i].(pgtype.Timestamptz)
		}
	}

	return nil
}
