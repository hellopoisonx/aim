package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// rowStub is the single-row result set the DAO sees. The service maps the DAO
// row, so the tests pin that mapping without a PostgreSQL server.
type rowStub struct {
	columns []string
	values  []driver.Value
}

func (s rowStub) Open(string) (driver.Conn, error) { return stubConn{stub: s}, nil }

type stubConn struct{ stub rowStub }

func (c stubConn) Prepare(string) (driver.Stmt, error) { return stubStmt{}, nil }
func (c stubConn) Close() error                        { return nil }
func (c stubConn) Begin() (driver.Tx, error)           { return stubTx{}, nil }

func (c stubConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &stubRows{stub: c.stub}, nil
}

func (c stubConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

type stubTx struct{}

func (stubTx) Commit() error   { return nil }
func (stubTx) Rollback() error { return nil }

type stubConnector struct{ stub rowStub }

func (c stubConnector) Connect(context.Context) (driver.Conn, error) {
	return stubConn{stub: c.stub}, nil
}
func (c stubConnector) Driver() driver.Driver { return c.stub }

type stubStmt struct{}

func (stubStmt) Close() error                               { return nil }
func (stubStmt) NumInput() int                              { return -1 }
func (stubStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (stubStmt) Query([]driver.Value) (driver.Rows, error)  { return &stubRows{}, nil }

type stubRows struct {
	stub rowStub
	done bool
}

func (r *stubRows) Columns() []string { return r.stub.columns }
func (r *stubRows) Close() error      { return nil }

func (r *stubRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.stub.values)

	return nil
}

func newStubStore(t *testing.T, fileID uuid.UUID, status, objectKey string) *dao.Store {
	t.Helper()

	sqlDB := sql.OpenDB(stubConnector{stub: rowStub{
		columns: []string{"file_id", "status", "object_key"},
		values:  []driver.Value{fileID.String(), status, objectKey},
	}})
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	require.NoError(t, err)

	return dao.New(db)
}

func TestDownload(t *testing.T) {
	fileID := uuid.New()
	objectKey := "/1/" + fileID.String() + "/original"

	tests := []struct {
		name    string
		status  string
		signer  s3signer.S3Signer
		wantErr error
	}{
		{
			name:   "uploaded row is presigned for the public endpoint",
			status: sharedattachment.UploadStatusUploaded,
			signer: newTestService().signer,
		},
		{
			name:    "pending row is rejected",
			status:  sharedattachment.UploadStatusPending,
			signer:  newTestService().signer,
			wantErr: ErrNotUploaded,
		},
		{
			name:   "signer error is returned unchanged",
			status: sharedattachment.UploadStatusUploaded,
			signer: s3signer.S3Signer{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			svc.store = newStubStore(t, fileID, tc.status, objectKey)
			svc.signer = tc.signer

			before := time.Now()
			resp, err := svc.Download(context.Background(), 1, fileID.String())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)

				return
			}
			if tc.signer.Bucket == "" {
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrNotUploaded)
				require.Nil(t, resp)

				return
			}
			require.NoError(t, err)
			require.Equal(t, "http://localhost:8333/aim-attachments"+objectKey, resp.URL)
			require.NotContains(t, resp.Headers, "host")
			ttl := svc.cfg.Seaweed.DownloadTTL()
			require.GreaterOrEqual(t, resp.ExpiresAt, before.Add(ttl).UnixMilli())
			require.LessOrEqual(t, resp.ExpiresAt, time.Now().Add(ttl).UnixMilli())
		})
	}
}

func TestDownloadSignsWhenCredentialsConfigured(t *testing.T) {
	fileID := uuid.New()
	svc := newTestService()
	svc.store = newStubStore(t, fileID, sharedattachment.UploadStatusUploaded, "k/original")
	svc.signer.AccessKey = "access"
	svc.signer.SecretKey = "secret"

	resp, err := svc.Download(context.Background(), 1, fileID.String())
	require.NoError(t, err)
	require.Contains(t, resp.URL, "X-Amz-Signature=")
}
