package httperr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClassifyCodeError(t *testing.T) {
	statusCode, envelope := Classify(errorx.NewCodeError(errorx.CodeAuth, "missing authorization"))
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Equal(t, errorx.CodeAuth, envelope.Code)
	require.Equal(t, "missing authorization", envelope.Msg)
}

func TestClassifyPreservesBizCodeAcrossGRPC(t *testing.T) {
	// A service error keeps its exact biz code through the ErrorInfo detail.
	codeErr := errorx.NewCodeError(errorx.CodeBadInput, "id must be positive")
	statusCode, envelope := Classify(status.ErrorProto(codeErr.GRPCStatus().Proto()))
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, errorx.CodeBadInput, envelope.Code)
	require.Equal(t, "id must be positive", envelope.Msg)
}

func TestClassifySanitizesInfraGRPCError(t *testing.T) {
	statusCode, envelope := Classify(status.Error(codes.Unavailable, "redis pool exhausted"))
	require.Equal(t, http.StatusInternalServerError, statusCode)
	require.Equal(t, CodeInternal, envelope.Code)
	require.Equal(t, "internal error", envelope.Msg)
}

func TestClassifyPlainErrorIsBadInput(t *testing.T) {
	// go-zero's binding layer reports parse/validation failures as plain errors.
	statusCode, envelope := Classify(errors.New("field email is not set"))
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, CodeBadInput, envelope.Code)
	require.Equal(t, "field email is not set", envelope.Msg)
}

func TestWriteSetsStatusAndEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, errorx.NewCodeError(errorx.CodeTokenExpired, "token expired"))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.JSONEq(t, `{"code":1005,"msg":"token expired","body":null}`, rec.Body.String())
}

func TestOkResponseWrapsBody(t *testing.T) {
	got, ok := OkResponse(context.Background(), map[string]string{"token": "access"}).(Envelope)
	require.True(t, ok)
	require.Equal(t, CodeOK, got.Code)
	require.Equal(t, "ok", got.Msg)
	require.Equal(t, map[string]string{"token": "access"}, got.Body)
}
