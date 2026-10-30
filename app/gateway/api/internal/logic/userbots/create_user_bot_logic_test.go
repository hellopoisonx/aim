package userbots

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	authpb "github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	botservice "github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAuthClient implements the goctl AuthService interface; only the
// CreateBotCredential RPC is exercised by this flow.
type fakeAuthClient struct {
	authservice.AuthService
	resp *authpb.CreateBotCredentialResp
	err  error
}

func (f *fakeAuthClient) CreateBotCredential(_ context.Context, _ *authpb.CreateBotCredentialReq, _ ...grpc.CallOption) (*authpb.CreateBotCredentialResp, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.resp, nil
}

// fakeBotClient embeds the full client interface so only the called RPC needs
// an implementation.
type fakeBotClient struct {
	botservice.BotService
	got  *logicpb.CreateUserBotReq
	resp *logicpb.CreateUserBotResp
	err  error
}

func (f *fakeBotClient) CreateUserBot(_ context.Context, req *logicpb.CreateUserBotReq, _ ...grpc.CallOption) (*logicpb.CreateUserBotResp, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}

	return f.resp, nil
}

var (
	_ authpb.AuthServiceClient = (*fakeAuthClient)(nil)
	_ botservice.BotService    = (*fakeBotClient)(nil)
)

func TestCreateUserBotRequiresIdentity(t *testing.T) {
	_, err := NewCreateUserBotLogic(context.Background(), &svc.ServiceContext{}).CreateUserBot(&types.CreateUserBotRequest{})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeAuth, codeErr.Code)
}

func TestCreateUserBotForwardsCredentialAndOwner(t *testing.T) {
	authClient := &fakeAuthClient{resp: &authpb.CreateBotCredentialResp{UserId: 900, Email: "bot@example.com"}}
	botClient := &fakeBotClient{resp: &logicpb.CreateUserBotResp{Bot: &logicpb.UserBotInfo{
		BotUserId: 900, OwnerUserId: 1, Email: "bot@example.com", Nickname: "helper",
	}}}
	svcCtx := &svc.ServiceContext{AuthClient: authClient, LogicBotClient: botClient}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1})
	resp, err := NewCreateUserBotLogic(ctx, svcCtx).CreateUserBot(&types.CreateUserBotRequest{
		Email: "bot@example.com", Nickname: "helper",
	})
	require.NoError(t, err)
	require.Equal(t, int64(900), resp.Bot.BotUserId)
	require.Equal(t, "helper", resp.Bot.Nickname)

	require.NotNil(t, botClient.got)
	require.Equal(t, int64(1), botClient.got.GetOwnerUserId(), "the caller owns the bot")
	require.Equal(t, int64(900), botClient.got.GetBotUserId())
}

func TestCreateUserBotSanitizesUpstreamError(t *testing.T) {
	authClient := &fakeAuthClient{err: status.Error(codes.Unavailable, "postgres password leaked")}
	svcCtx := &svc.ServiceContext{AuthClient: authClient}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1})
	_, err := NewCreateUserBotLogic(ctx, svcCtx).CreateUserBot(&types.CreateUserBotRequest{})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}
