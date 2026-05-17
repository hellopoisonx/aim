package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGatewayAuthProxyLogic(t *testing.T) {
	client := &proxyAuthClient{}
	svcCtx := svc.NewServiceContextWithAuth(config.Config{}, client)
	ctx := context.Background()

	registered, err := NewRegisterLogic(ctx, svcCtx).Register(&types.RegisterRequest{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)
	require.Equal(t, int64(7), registered.UserId)
	require.Equal(t, "ada@example.com", client.register.Email)

	loggedIn, err := NewLoginLogic(ctx, svcCtx).Login(&types.LoginRequest{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)
	require.Equal(t, "access", loggedIn.AccessToken)
	require.Equal(t, "refresh", loggedIn.RefreshToken)

	refreshed, err := NewRefreshLogic(ctx, svcCtx).Refresh(&types.RefreshRequest{RefreshToken: "refresh"})
	require.NoError(t, err)
	require.Equal(t, "next-access", refreshed.AccessToken)
	require.Equal(t, "next-refresh", refreshed.RefreshToken)
}

func TestGatewayAuthProxyLogicSanitizesPlainError(t *testing.T) {
	t.Parallel()

	svcCtx := svc.NewServiceContextWithAuth(config.Config{}, &failingAuthClient{err: errors.New("postgres password leaked")})
	ctx := context.Background()

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&types.RegisterRequest{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	requireInternalError(t, err)

	_, err = NewLoginLogic(ctx, svcCtx).Login(&types.LoginRequest{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	requireInternalError(t, err)

	_, err = NewRefreshLogic(ctx, svcCtx).Refresh(&types.RefreshRequest{RefreshToken: "refresh"})
	requireInternalError(t, err)
}

func TestGatewayAuthProxyLogicPreservesGrpcBizCode(t *testing.T) {
	t.Parallel()

	// gRPC status error from auth service: invalid credentials → 40100.
	grpcErr := status.Error(codes.Unauthenticated, "invalid credentials")
	client := &failingAuthClient{err: grpcErr}
	svcCtx := svc.NewServiceContextWithAuth(config.Config{}, client)
	ctx := context.Background()

	_, err := NewLoginLogic(ctx, svcCtx).Login(&types.LoginRequest{Email: "ada@example.com", Password: "wrong", DeviceId: "desktop-1"})
	requireCodeError(t, err, 40100, "invalid credentials")
}

func TestGatewayAuthProxyLogicSanitizesGrpcInternal(t *testing.T) {
	t.Parallel()

	// gRPC Internal errors should still sanitize the message.
	grpcErr := status.Error(codes.Internal, "postgres password leaked")
	client := &failingAuthClient{err: grpcErr}
	svcCtx := svc.NewServiceContextWithAuth(config.Config{}, client)
	ctx := context.Background()

	_, err := NewLoginLogic(ctx, svcCtx).Login(&types.LoginRequest{Email: "ada@example.com", Password: "wrong", DeviceId: "desktop-1"})
	requireCodeError(t, err, 50000, "internal error")
}

func TestGatewayAuthProxyPreservesGrpcConflict(t *testing.T) {
	t.Parallel()

	grpcErr := status.Error(codes.AlreadyExists, "email already registered")
	client := &failingAuthClient{err: grpcErr}
	svcCtx := svc.NewServiceContextWithAuth(config.Config{}, client)
	ctx := context.Background()

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&types.RegisterRequest{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	requireCodeError(t, err, 40900, "email already registered")
}

func requireInternalError(t *testing.T, err error) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, 50000, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}

func requireCodeError(t *testing.T, err error, wantCode int, wantMsg string) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, wantCode, codeErr.Code)
	require.Equal(t, wantMsg, codeErr.Message)
}

type proxyAuthClient struct {
	register authservice.RegisterReq
}

func (c *proxyAuthClient) Register(_ context.Context, req *authservice.RegisterReq, _ ...grpc.CallOption) (*authservice.RegisterResp, error) {
	c.register.Email = req.Email
	return &authservice.RegisterResp{UserId: 7}, nil
}

func (c *proxyAuthClient) Login(context.Context, *authservice.LoginReq, ...grpc.CallOption) (*authservice.LoginResp, error) {
	return &authservice.LoginResp{UserId: 7, AccessToken: "access", RefreshToken: "refresh", ExpiresAt: 123}, nil
}

func (c *proxyAuthClient) RefreshToken(context.Context, *authservice.RefreshTokenReq, ...grpc.CallOption) (*authservice.RefreshTokenResp, error) {
	return &authservice.RefreshTokenResp{AccessToken: "next-access", RefreshToken: "next-refresh", ExpiresAt: 456}, nil
}

func (c *proxyAuthClient) Logout(context.Context, *authservice.LogoutReq, ...grpc.CallOption) (*authservice.LogoutResp, error) {
	return &authservice.LogoutResp{Success: true}, nil
}

type failingAuthClient struct {
	err error
}

func (c *failingAuthClient) Register(context.Context, *authservice.RegisterReq, ...grpc.CallOption) (*authservice.RegisterResp, error) {
	return nil, c.err
}

func (c *failingAuthClient) Login(context.Context, *authservice.LoginReq, ...grpc.CallOption) (*authservice.LoginResp, error) {
	return nil, c.err
}

func (c *failingAuthClient) RefreshToken(context.Context, *authservice.RefreshTokenReq, ...grpc.CallOption) (*authservice.RefreshTokenResp, error) {
	return nil, c.err
}

func (c *failingAuthClient) Logout(context.Context, *authservice.LogoutReq, ...grpc.CallOption) (*authservice.LogoutResp, error) {
	return nil, c.err
}
