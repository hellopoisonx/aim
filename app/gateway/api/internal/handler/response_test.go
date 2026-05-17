package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestOkResponseWrapsBody(t *testing.T) {
	t.Parallel()

	got, ok := okResponse(context.Background(), map[string]string{"token": "access"}).(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, 0, got.Code)
	require.Equal(t, "ok", got.Msg)
	require.Equal(t, map[string]string{"token": "access"}, got.Body)
}

func TestErrorResponseWrapsCodeError(t *testing.T) {
	t.Parallel()

	statusCode, got := errorResponse(context.Background(), errorx.NewCodeError(40100, "missing authorization"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Equal(t, 40100, envelope.Code)
	require.Equal(t, "missing authorization", envelope.Msg)
	require.Nil(t, envelope.Body)
}

func TestErrorResponseSanitizesGrpcInfraError(t *testing.T) {
	t.Parallel()

	// codes.Unavailable is an infrastructure code → sanitized.
	statusCode, got := errorResponse(context.Background(), status.Error(codes.Unavailable, "auth database down"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusInternalServerError, statusCode)
	require.Equal(t, 50000, envelope.Code)
	require.Equal(t, "internal error", envelope.Msg)
}

func TestErrorResponsePreservesGrpcBizCode(t *testing.T) {
	t.Parallel()

	// Simulate an auth RPC returning 40100 over gRPC.
	// With GRPCStatus(), CodeError(40100, "invalid credentials") becomes
	// gRPC codes.Unauthenticated. The gateway receives a gRPC status error.
	statusCode, got := errorResponse(context.Background(), status.Error(codes.Unauthenticated, "invalid credentials"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Equal(t, 40100, envelope.Code)
	require.Equal(t, "invalid credentials", envelope.Msg)
}

func TestErrorResponseSanitizesGrpcInternal(t *testing.T) {
	t.Parallel()

	statusCode, got := errorResponse(context.Background(), status.Error(codes.Internal, "postgres password leaked"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusInternalServerError, statusCode)
	require.Equal(t, 50000, envelope.Code)
	require.Equal(t, "internal error", envelope.Msg)
}

func TestErrorResponsePreservesGrpcConflict(t *testing.T) {
	t.Parallel()

	statusCode, got := errorResponse(context.Background(), status.Error(codes.AlreadyExists, "email already registered"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusConflict, statusCode)
	require.Equal(t, 40900, envelope.Code)
	require.Equal(t, "email already registered", envelope.Msg)
}

func TestErrorResponseKeepsBadRequestMessage(t *testing.T) {
	t.Parallel()

	statusCode, got := errorResponse(context.Background(), errors.New("field email is not set"))
	envelope, ok := got.(responseEnvelope)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, 40000, envelope.Code)
	require.Equal(t, "field email is not set", envelope.Msg)
}

func TestHttpStatusFromCodeRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code int
		want int
	}{
		{40000, http.StatusBadRequest},
		{40100, http.StatusUnauthorized},
		{40300, http.StatusForbidden},
		{40400, http.StatusNotFound},
		{40900, http.StatusConflict},
		{42900, http.StatusTooManyRequests},
		{50000, http.StatusInternalServerError},
		{59999, http.StatusInternalServerError}, // 59999/100=599 clamped to 500
		{errorx.CodeInvalidCredentials, http.StatusUnauthorized},
		{errorx.CodeUserNotFound, http.StatusNotFound},
		{errorx.CodeUserExists, http.StatusConflict},
		{errorx.CodeTokenInvalid, http.StatusUnauthorized},
		{errorx.CodeTokenExpired, http.StatusUnauthorized},
		{errorx.CodeUserBanned, http.StatusForbidden},
		{999, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		got := httpStatusFromCode(tt.code)
		require.Equal(t, tt.want, got, "code=%d", tt.code)
	}
}
