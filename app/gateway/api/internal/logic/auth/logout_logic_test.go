package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestLogoutUsesBearerClaims(t *testing.T) {
	manager := jwt.NewManager("test-secret")
	token, _, err := manager.GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	authClient := &recordingAuthClient{}

	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	ctx := authctx.WithAuthorization(context.Background(), "Bearer "+token)

	resp, err := NewLogoutLogic(ctx, svc.NewServiceContextWithAuth(c, authClient)).Logout()
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, int64(42), authClient.logoutUserID)
	require.Equal(t, "desktop-1", authClient.logoutDeviceID)
}

func TestLogoutSanitizesRPCError(t *testing.T) {
	manager := jwt.NewManager("test-secret")
	token, _, err := manager.GenerateAccessToken(42, "desktop-1")
	require.NoError(t, err)

	var c config.Config

	c.Auth.AccessSecret = "test-secret"
	ctx := authctx.WithAuthorization(context.Background(), "Bearer "+token)

	_, err = NewLogoutLogic(ctx, svc.NewServiceContextWithAuth(c, &recordingAuthClient{err: errors.New("redis password leaked")})).Logout()
	requireInternalError(t, err)
}

func TestLogoutRejectsMissingBearerToken(t *testing.T) {
	var c config.Config

	c.Auth.AccessSecret = "test-secret"

	_, err := NewLogoutLogic(context.Background(), svc.NewServiceContextWithAuth(c, &recordingAuthClient{})).Logout()
	require.Error(t, err)
}

type recordingAuthClient struct {
	logoutUserID   int64
	logoutDeviceID string
	err            error
}

func (c *recordingAuthClient) Register(context.Context, *authservice.RegisterReq, ...grpc.CallOption) (*authservice.RegisterResp, error) {
	return nil, nil
}

func (c *recordingAuthClient) Login(context.Context, *authservice.LoginReq, ...grpc.CallOption) (*authservice.LoginResp, error) {
	return nil, nil
}

func (c *recordingAuthClient) RefreshToken(context.Context, *authservice.RefreshTokenReq, ...grpc.CallOption) (*authservice.RefreshTokenResp, error) {
	return nil, nil
}

func (c *recordingAuthClient) Logout(_ context.Context, req *authservice.LogoutReq, _ ...grpc.CallOption) (*authservice.LogoutResp, error) {
	if c.err != nil {
		return nil, c.err
	}

	c.logoutUserID = req.UserId
	c.logoutDeviceID = req.DeviceId

	return &authservice.LogoutResp{Success: true}, nil
}

func (c *recordingAuthClient) CreateBotCredential(context.Context, *authservice.CreateBotCredentialReq, ...grpc.CallOption) (*authservice.CreateBotCredentialResp, error) { return nil, nil }
