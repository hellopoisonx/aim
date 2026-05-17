package errorx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCodeErrorError(t *testing.T) {
	t.Parallel()

	ce := NewCodeError(40100, "unauthorized")
	require.Equal(t, "unauthorized", ce.Error())
}

func TestCodeErrorIs(t *testing.T) {
	t.Parallel()

	ce := NewCodeError(40100, "unauthorized")

	require.True(t, ce.Is(NewCodeError(40100, "other message")))
	require.False(t, ce.Is(NewCodeError(40900, "unauthorized")))
	require.False(t, ce.Is(errors.New("unauthorized")))
}

func TestNewCodeErrorf(t *testing.T) {
	t.Parallel()

	ce := NewCodeErrorf(40000, "field %s is required", "email")
	require.Equal(t, 40000, ce.Code)
	require.Equal(t, "field email is required", ce.Message)
}

func TestGRPCStatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bizCode  int
		wantGRPC codes.Code
	}{
		{40000, codes.InvalidArgument},
		{40100, codes.Unauthenticated},
		{40300, codes.PermissionDenied},
		{40400, codes.NotFound},
		{40900, codes.AlreadyExists},
		{42900, codes.ResourceExhausted},
		{50000, codes.Internal},
		{59999, codes.Internal},
		{CodeInvalidCredentials, codes.Unauthenticated},
		{CodeUserNotFound, codes.Unauthenticated},
		{CodeUserBanned, codes.Unauthenticated},
		{999, codes.Unauthenticated},
	}

	for _, tt := range tests {
		ce := NewCodeError(tt.bizCode, "test")
		st := ce.GRPCStatus()
		require.Equal(t, tt.wantGRPC, st.Code(), "bizCode=%d", tt.bizCode)
		require.Equal(t, "test", st.Message())
	}
}

func TestFromGRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantNil  bool
		wantCode int
		wantMsg  string
	}{
		{
			name:     "unauthorized",
			err:      status.Error(codes.Unauthenticated, "invalid credentials"),
			wantCode: 40100,
			wantMsg:  "invalid credentials",
		},
		{
			name:     "invalid argument",
			err:      status.Error(codes.InvalidArgument, "missing required field"),
			wantCode: 40000,
			wantMsg:  "missing required field",
		},
		{
			name:     "already exists",
			err:      status.Error(codes.AlreadyExists, "email already registered"),
			wantCode: 40900,
			wantMsg:  "email already registered",
		},
		{
			name:     "not found",
			err:      status.Error(codes.NotFound, "user not found"),
			wantCode: 40400,
			wantMsg:  "user not found",
		},
		{
			name:     "internal sanitized",
			err:      status.Error(codes.Internal, "postgres password leaked"),
			wantCode: 50000,
			wantMsg:  "internal error",
		},
		{
			name:     "unknown sanitized",
			err:      status.Error(codes.Unknown, "something broke"),
			wantCode: 50000,
			wantMsg:  "internal error",
		},
		{
			name:    "plain error returns nil",
			err:     errors.New("plain error"),
			wantNil: true,
		},
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := FromGRPCError(tt.err)
			if tt.wantNil {
				require.Nil(t, ce)
				return
			}

			require.NotNil(t, ce)
			require.Equal(t, tt.wantCode, ce.Code)
			require.Equal(t, tt.wantMsg, ce.Message)
		})
	}
}

func TestGRPCStatusRoundTrip(t *testing.T) {
	t.Parallel()

	// An RPC server returns CodeError(40100, "invalid credentials").
	original := NewCodeError(40100, "invalid credentials")

	// gRPC framework calls GRPCStatus() → wire status.
	st := original.GRPCStatus()
	require.Equal(t, codes.Unauthenticated, st.Code())
	require.Equal(t, "invalid credentials", st.Message())

	// Gateway receives gRPC status error, extracts CodeError.
	gwErr := status.Error(st.Code(), st.Message())
	recovered := FromGRPCError(gwErr)
	require.NotNil(t, recovered)
	require.Equal(t, 40100, recovered.Code)
	require.Equal(t, "invalid credentials", recovered.Message)
}

func TestGRPCStatusRoundTripInternal(t *testing.T) {
	t.Parallel()

	// Internal errors get sanitized on the gateway side.
	original := NewCodeError(50000, "database connection pool exhausted")
	st := original.GRPCStatus()
	require.Equal(t, codes.Internal, st.Code())

	gwErr := status.Error(st.Code(), st.Message())
	recovered := FromGRPCError(gwErr)
	require.NotNil(t, recovered)
	require.Equal(t, 50000, recovered.Code)
	require.Equal(t, "internal error", recovered.Message)
}

func TestBizCodeFromGRPCCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		grpcCode codes.Code
		want     int
	}{
		{codes.InvalidArgument, 40000},
		{codes.Unauthenticated, 40100},
		{codes.PermissionDenied, 40300},
		{codes.NotFound, 40400},
		{codes.AlreadyExists, 40900},
		{codes.ResourceExhausted, 42900},
		{codes.Internal, 50000},
		{codes.Unavailable, 50000},
		{codes.Unknown, 50000},
	}

	for _, tt := range tests {
		got := bizCodeFromGRPCCode(tt.grpcCode)
		require.Equal(t, tt.want, got, "grpcCode=%s", tt.grpcCode)
	}
}

func TestSentinelErrorVariables(t *testing.T) {
	t.Parallel()

	require.Equal(t, CodeInvalidCredentials, ErrInvalidCredentials.Code)
	require.Equal(t, CodeUserNotFound, ErrUserNotFound.Code)
	require.Equal(t, CodeUserExists, ErrUserExists.Code)
	require.Equal(t, CodeTokenInvalid, ErrTokenInvalid.Code)
	require.Equal(t, CodeTokenExpired, ErrTokenExpired.Code)
	require.Equal(t, CodeUserBanned, ErrUserBanned.Code)
}
