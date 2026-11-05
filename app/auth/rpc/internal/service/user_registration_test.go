package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var errSender = errors.New("broker unavailable")

// dbStub stands in for the database connection: registration only needs the
// INSERT to succeed or fail, and reports the generated id through RETURNING.
type dbStub struct {
	execErr error
}

func (d dbStub) Open(string) (driver.Conn, error) { return connStub{d}, nil }

type connStub struct{ stub dbStub }

func (c connStub) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported")
}
func (c connStub) Close() error              { return nil }
func (c connStub) Begin() (driver.Tx, error) { return txStub{}, nil }

type txStub struct{}

func (txStub) Commit() error   { return nil }
func (txStub) Rollback() error { return nil }

func (c connStub) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.stub.execErr != nil {
		return nil, c.stub.execErr
	}

	return driver.RowsAffected(1), nil
}

func (c connStub) QueryContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Rows, error) {
	if c.stub.execErr != nil {
		return nil, c.stub.execErr
	}
	// RETURNING "id": echo the last bound id so the credential keeps the id the
	// registrar generated.
	var id int64
	for _, arg := range args {
		if v, ok := arg.Value.(int64); ok {
			id = v
		}
	}

	return &rowsStub{columns: []string{"id"}, values: []driver.Value{id}}, nil
}

type rowsStub struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *rowsStub) Columns() []string { return r.columns }
func (r *rowsStub) Close() error      { return nil }

func (r *rowsStub) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)

	return nil
}

type connectorStub struct{ stub dbStub }

func (c connectorStub) Connect(context.Context) (driver.Conn, error) { return connStub{c.stub}, nil }
func (c connectorStub) Driver() driver.Driver                        { return c.stub }

func newStubDB(t *testing.T, execErr error) *gorm.DB {
	t.Helper()

	sqlDB := sql.OpenDB(connectorStub{stub: dbStub{execErr: execErr}})
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	require.NoError(t, err)

	return db
}

type fakeSender struct {
	err     error
	message rocketmq.TransactionalMessage
	calls   int
}

func (s *fakeSender) SendTransactional(ctx context.Context, db *gorm.DB, message rocketmq.TransactionalMessage, localTx func(context.Context, *gorm.DB) error) error {
	s.calls++
	s.message = message
	if s.err != nil {
		return s.err
	}

	return localTx(ctx, db)
}

func TestUserRegistrarCreateUser(t *testing.T) {
	idGen, err := tools.NewSnowflake(1)
	require.NoError(t, err)

	tests := []struct {
		name    string
		execErr error
		sendErr error
		wantErr error
	}{
		{name: "commits the credential and the event"},
		{name: "duplicate email surfaces unchanged", execErr: dao.ErrDuplicateKey, wantErr: dao.ErrDuplicateKey},
		{name: "sender failure is returned unchanged", sendErr: errSender, wantErr: errSender},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeSender{err: tc.sendErr}
			registrar := NewUserRegistrar(newStubDB(t, tc.execErr), idGen, sender, "aim-user-created")
			require.NotNil(t, registrar)

			user, err := registrar.CreateUser(context.Background(), "ada@example.com", "hash", "Ada")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Zero(t, user)
				require.Equal(t, 1, sender.calls)

				return
			}

			require.NoError(t, err)
			require.Positive(t, user.ID)
			require.Equal(t, "ada@example.com", user.Email)
			require.Equal(t, "hash", user.PasswordHash)
			require.Equal(t, "Ada", user.Name)
			require.EqualValues(t, StatusNormal, user.Status)

			require.Equal(t, "aim-user-created", sender.message.Topic)
			require.Equal(t, "user.created:"+strconv.FormatInt(user.ID, 10), sender.message.EventUID)
			require.Equal(t, strconv.FormatInt(user.ID, 10), sender.message.Key)

			var event events.UserCreatedEvent
			require.NoError(t, json.Unmarshal(sender.message.Body, &event))
			require.Equal(t, user.ID, event.UserID)
			require.Equal(t, "ada@example.com", event.Email)
			require.Equal(t, "Ada", event.Nickname)
		})
	}
}

func TestNewUserRegistrarRequiresDependencies(t *testing.T) {
	idGen, err := tools.NewSnowflake(1)
	require.NoError(t, err)
	db := newStubDB(t, nil)
	sender := &fakeSender{}

	require.Nil(t, NewUserRegistrar(nil, idGen, sender, "topic"))
	require.Nil(t, NewUserRegistrar(db, nil, sender, "topic"))
	require.Nil(t, NewUserRegistrar(db, idGen, nil, "topic"))
	require.Nil(t, NewUserRegistrar(db, idGen, sender, ""))
}
