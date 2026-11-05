package botservicelogic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeBotQuerier implements the token/webhook DAO slice. Methods left out of
// this fake are meant to be unreachable from the code under test.
type fakeBotQuerier struct {
	service.BotQuerier

	tokenRow    dao.BotTokenWithUser
	tokenErr    error
	tokenActs   []string
	userInfo    orm.UserInfo
	userInfoErr error
	convs       []orm.Conversation
	convsErr    error
	webhook     *orm.BotWebhook
	webhookErr  error
	eventAction map[string]string
	deleteRows  int64
	deleteErr   error
}

func (f *fakeBotQuerier) GetBotTokenByHash(context.Context, string) (dao.BotTokenWithUser, error) {
	return f.tokenRow, f.tokenErr
}

func (f *fakeBotQuerier) ListEnabledActionsByToken(context.Context, int64) ([]string, error) {
	return f.tokenActs, nil
}

func (f *fakeBotQuerier) GetEnabledActionByWebhookEvent(_ context.Context, event string) (string, error) {
	action, ok := f.eventAction[event]
	if !ok {
		return "", dao.ErrNotFound
	}
	return action, nil
}

func (f *fakeBotQuerier) GetUserInfoByID(context.Context, int64) (orm.UserInfo, error) {
	return f.userInfo, f.userInfoErr
}

func (f *fakeBotQuerier) GetConversationsByUserID(context.Context, int64) ([]orm.Conversation, error) {
	return f.convs, f.convsErr
}

func (f *fakeBotQuerier) GetBotWebhook(context.Context, int64) (orm.BotWebhook, error) {
	if f.webhookErr != nil {
		return orm.BotWebhook{}, f.webhookErr
	}
	if f.webhook == nil {
		return orm.BotWebhook{}, dao.ErrNotFound
	}
	return *f.webhook, nil
}

func (f *fakeBotQuerier) DeleteBotWebhook(context.Context, int64) (int64, error) {
	return f.deleteRows, f.deleteErr
}

// fakeBotManagement implements the user-side management slice.
type fakeBotManagement struct {
	service.BotManagementQuerier

	bot    dao.ManagedUserBot
	botErr error
}

func (f *fakeBotManagement) GetManagedUserBot(context.Context, int64, int64) (dao.ManagedUserBot, error) {
	return f.bot, f.botErr
}

func botSvcCtx(queries service.BotQuerier, management service.BotManagementQuerier) *svc.ServiceContext {
	return &svc.ServiceContext{BotService: service.NewBotService(queries, management)}
}

// workingBotSvcCtx returns a context whose BotService dependencies are real
// fakes rather than nil, so a regressed guard fails an assertion instead of
// panicking on a nil interface.
func workingBotSvcCtx() (*svc.ServiceContext, *fakeBotQuerier, *fakeBotManagement) {
	queries := &fakeBotQuerier{}
	management := &fakeBotManagement{}
	return botSvcCtx(queries, management), queries, management
}

func requireBizCode(t *testing.T, err error, code int, message string) {
	t.Helper()
	require.Error(t, err)

	codeErr := errorx.FromGRPCError(err)
	require.NotNil(t, codeErr, "error must carry an AIM biz code: %v", err)
	require.Equal(t, code, codeErr.Code)
	if message != "" {
		// Error() keeps the caller-facing message; FromGRPCError rewrites it
		// to "internal error" for infrastructure gRPC codes.
		require.Contains(t, err.Error(), message)
	}
}

func TestBotServiceLogicRequiresConfiguredBotService(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, svcCtx *svc.ServiceContext) error
	}{
		{"AddUserBotToConversation", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewAddUserBotToConversationLogic(ctx, c).AddUserBotToConversation(
				&pb.AddUserBotToConversationReq{BotUserId: 1})
			return err
		}},
		{"CreateUserBotDirectConversation", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewCreateUserBotDirectConversationLogic(ctx, c).CreateUserBotDirectConversation(
				&pb.CreateUserBotDirectConversationReq{BotUserId: 1})
			return err
		}},
		{"CreateUserBot", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewCreateUserBotLogic(ctx, c).CreateUserBot(&pb.CreateUserBotReq{})
			return err
		}},
		{"CreateUserBotToken", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewCreateUserBotTokenLogic(ctx, c).CreateUserBotToken(&pb.CreateUserBotTokenReq{})
			return err
		}},
		{"DeleteBotWebhook", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewDeleteBotWebhookLogic(ctx, c).DeleteBotWebhook(&pb.DeleteBotWebhookReq{})
			return err
		}},
		{"DeleteUserBot", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewDeleteUserBotLogic(ctx, c).DeleteUserBot(&pb.DeleteUserBotReq{})
			return err
		}},
		{"GetBotProfile", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewGetBotProfileLogic(ctx, c).GetBotProfile(&pb.GetBotProfileReq{})
			return err
		}},
		{"GetBotWebhook", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewGetBotWebhookLogic(ctx, c).GetBotWebhook(&pb.GetBotWebhookReq{})
			return err
		}},
		{"GetUserBot", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewGetUserBotLogic(ctx, c).GetUserBot(&pb.GetUserBotReq{})
			return err
		}},
		{"ListBotActions", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewListBotActionsLogic(ctx, c).ListBotActions(&pb.ListBotActionsReq{})
			return err
		}},
		{"ListBotConversations", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewListBotConversationsLogic(ctx, c).ListBotConversations(&pb.ListBotConversationsReq{})
			return err
		}},
		{"ListBotEvents", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewListBotEventsLogic(ctx, c).ListBotEvents(&pb.ListBotEventsReq{})
			return err
		}},
		{"ListUserBotTokens", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewListUserBotTokensLogic(ctx, c).ListUserBotTokens(&pb.ListUserBotTokensReq{})
			return err
		}},
		{"ListUserBots", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewListUserBotsLogic(ctx, c).ListUserBots(&pb.ListUserBotsReq{})
			return err
		}},
		{"ResolveBotWebhookEventActions", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewResolveBotWebhookEventActionsLogic(ctx, c).ResolveBotWebhookEventActions(
				&pb.ResolveBotWebhookEventActionsReq{})
			return err
		}},
		{"RevokeUserBotToken", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewRevokeUserBotTokenLogic(ctx, c).RevokeUserBotToken(&pb.RevokeUserBotTokenReq{})
			return err
		}},
		{"RotateUserBotToken", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewRotateUserBotTokenLogic(ctx, c).RotateUserBotToken(&pb.RotateUserBotTokenReq{})
			return err
		}},
		{"SetBotWebhook", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewSetBotWebhookLogic(ctx, c).SetBotWebhook(&pb.SetBotWebhookReq{})
			return err
		}},
		{"SetUserBotStatus", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewSetUserBotStatusLogic(ctx, c).SetUserBotStatus(&pb.SetUserBotStatusReq{})
			return err
		}},
		{"UpdateUserBotProfile", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewUpdateUserBotProfileLogic(ctx, c).UpdateUserBotProfile(&pb.UpdateUserBotProfileReq{})
			return err
		}},
		{"UpdateUserBotToken", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewUpdateUserBotTokenLogic(ctx, c).UpdateUserBotToken(&pb.UpdateUserBotTokenReq{})
			return err
		}},
		{"ValidateBotToken", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewValidateBotTokenLogic(ctx, c).ValidateBotToken(&pb.ValidateBotTokenReq{})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(context.Background(), &svc.ServiceContext{})
			requireBizCode(t, err, errorx.CodeInternal, "bot service not configured")
			require.Equal(t, codes.Internal, status.Code(err))
		})
	}
}

func TestBotServiceLogicValidatesInput(t *testing.T) {
	svcCtx, _, _ := workingBotSvcCtx()

	tests := []struct {
		name    string
		call    func(ctx context.Context) error
		code    int
		message string
	}{
		{
			name: "GetBotProfile rejects zero bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewGetBotProfileLogic(ctx, svcCtx).GetBotProfile(&pb.GetBotProfileReq{})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "GetBotWebhook rejects negative bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewGetBotWebhookLogic(ctx, svcCtx).GetBotWebhook(&pb.GetBotWebhookReq{BotUserId: -1})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "SetBotWebhook rejects zero bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewSetBotWebhookLogic(ctx, svcCtx).SetBotWebhook(&pb.SetBotWebhookReq{Url: "https://example.com/hook"})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "DeleteBotWebhook rejects zero bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewDeleteBotWebhookLogic(ctx, svcCtx).DeleteBotWebhook(&pb.DeleteBotWebhookReq{})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "ListBotConversations rejects zero bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewListBotConversationsLogic(ctx, svcCtx).ListBotConversations(&pb.ListBotConversationsReq{})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "ValidateBotToken rejects empty plaintext",
			call: func(ctx context.Context) error {
				_, err := NewValidateBotTokenLogic(ctx, svcCtx).ValidateBotToken(&pb.ValidateBotTokenReq{})
				return err
			},
			code: errorx.CodeBotTokenInvalid, message: "token is required",
		},
		{
			name: "CreateUserBot rejects zero owner_user_id",
			call: func(ctx context.Context) error {
				_, err := NewCreateUserBotLogic(ctx, svcCtx).CreateUserBot(&pb.CreateUserBotReq{
					BotUserId: 7, Email: "bot@example.com", Nickname: "bot",
				})
				return err
			},
			code: errorx.CodeBadInput, message: "owner_user_id is required",
		},
		{
			name: "CreateUserBot rejects zero bot_user_id",
			call: func(ctx context.Context) error {
				_, err := NewCreateUserBotLogic(ctx, svcCtx).CreateUserBot(&pb.CreateUserBotReq{
					OwnerUserId: 3, Email: "bot@example.com", Nickname: "bot",
				})
				return err
			},
			code: errorx.CodeBadInput, message: "bot_user_id is required",
		},
		{
			name: "CreateUserBot rejects blank nickname",
			call: func(ctx context.Context) error {
				_, err := NewCreateUserBotLogic(ctx, svcCtx).CreateUserBot(&pb.CreateUserBotReq{
					OwnerUserId: 3, BotUserId: 7, Email: "bot@example.com", Nickname: "  ",
				})
				return err
			},
			code: errorx.CodeBadInput, message: "nickname is required",
		},
		{
			name: "ListUserBots rejects zero owner_user_id",
			call: func(ctx context.Context) error {
				_, err := NewListUserBotsLogic(ctx, svcCtx).ListUserBots(&pb.ListUserBotsReq{})
				return err
			},
			code: errorx.CodeBadInput, message: "owner_user_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBizCode(t, tt.call(context.Background()), tt.code, tt.message)
		})
	}
}

func TestBotServiceLogicErrorMapping(t *testing.T) {
	infraErr := errors.New("connection reset by peer")

	tests := []struct {
		name     string
		svcCtx   func() *svc.ServiceContext
		call     func(ctx context.Context, c *svc.ServiceContext) error
		code     int
		grpcCode codes.Code
		message  string
	}{
		{
			name: "GetUserBot maps a management miss to not found",
			svcCtx: func() *svc.ServiceContext {
				c, _, m := workingBotSvcCtx()
				m.botErr = dao.ErrNotFound
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetUserBotLogic(ctx, c).GetUserBot(&pb.GetUserBotReq{OwnerUserId: 3, BotUserId: 7})
				return err
			},
			code: errorx.CodeNotFound, grpcCode: codes.NotFound, message: "bot not found",
		},
		{
			name: "GetUserBot forwards infrastructure errors untouched",
			svcCtx: func() *svc.ServiceContext {
				c, _, m := workingBotSvcCtx()
				m.botErr = infraErr
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetUserBotLogic(ctx, c).GetUserBot(&pb.GetUserBotReq{OwnerUserId: 3, BotUserId: 7})
				return err
			},
			code: errorx.CodeInternal, grpcCode: codes.Unknown, message: infraErr.Error(),
		},
		{
			name: "GetBotProfile keeps the user-not-found sentinel",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.userInfoErr = dao.ErrNotFound
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetBotProfileLogic(ctx, c).GetBotProfile(&pb.GetBotProfileReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeUserNotFound, grpcCode: codes.NotFound, message: "user not found",
		},
		{
			name: "GetBotProfile sanitises unclassified errors",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.userInfoErr = infraErr
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetBotProfileLogic(ctx, c).GetBotProfile(&pb.GetBotProfileReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeInternal, grpcCode: codes.Internal, message: "internal error",
		},
		{
			name: "GetBotProfile rejects non-bot identities",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.userInfo = orm.UserInfo{ID: 7, UserType: "human"}
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetBotProfileLogic(ctx, c).GetBotProfile(&pb.GetBotProfileReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeBotDisabled, grpcCode: codes.Unauthenticated, message: "user is not a bot",
		},
		{
			name: "GetBotWebhook wraps query failures as internal",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.webhookErr = infraErr
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewGetBotWebhookLogic(ctx, c).GetBotWebhook(&pb.GetBotWebhookReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeInternal, grpcCode: codes.Internal, message: "get bot webhook failed",
		},
		{
			name: "DeleteBotWebhook wraps delete failures as internal",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.deleteErr = infraErr
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewDeleteBotWebhookLogic(ctx, c).DeleteBotWebhook(&pb.DeleteBotWebhookReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeInternal, grpcCode: codes.Internal, message: "delete bot webhook failed",
		},
		{
			name: "ListBotConversations wraps query failures as internal",
			svcCtx: func() *svc.ServiceContext {
				c, q, _ := workingBotSvcCtx()
				q.convsErr = infraErr
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewListBotConversationsLogic(ctx, c).ListBotConversations(
					&pb.ListBotConversationsReq{BotUserId: 7})
				return err
			},
			code: errorx.CodeInternal, grpcCode: codes.Internal, message: "list bot conversations failed",
		},
		{
			name: "ValidateBotToken rejects malformed plaintext with the bot token code",
			svcCtx: func() *svc.ServiceContext {
				c, _, _ := workingBotSvcCtx()
				return c
			},
			call: func(ctx context.Context, c *svc.ServiceContext) error {
				_, err := NewValidateBotTokenLogic(ctx, c).ValidateBotToken(
					&pb.ValidateBotTokenReq{PlaintextToken: "not-a-bot-token"})
				return err
			},
			code: errorx.CodeBotTokenInvalid, grpcCode: codes.Unauthenticated, message: "invalid bot token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(context.Background(), tt.svcCtx())
			require.Error(t, err)
			require.Equal(t, tt.grpcCode, status.Code(err))
			require.Contains(t, err.Error(), tt.message)
			if tt.grpcCode != codes.Unknown {
				requireBizCode(t, err, tt.code, tt.message)
			}
		})
	}
}

func TestBotServiceLogicRequiresConversationService(t *testing.T) {
	newCtx := func() *svc.ServiceContext {
		c, _, m := workingBotSvcCtx()
		m.bot = dao.ManagedUserBot{ID: 7, OwnerUserID: 3}
		return c
	}

	tests := []struct {
		name string
		call func(ctx context.Context, c *svc.ServiceContext) error
	}{
		{"AddUserBotToConversation", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewAddUserBotToConversationLogic(ctx, c).AddUserBotToConversation(
				&pb.AddUserBotToConversationReq{OwnerUserId: 3, BotUserId: 7, ConversationId: 11, OperatorId: 3})
			return err
		}},
		{"CreateUserBotDirectConversation", func(ctx context.Context, c *svc.ServiceContext) error {
			_, err := NewCreateUserBotDirectConversationLogic(ctx, c).CreateUserBotDirectConversation(
				&pb.CreateUserBotDirectConversationReq{OwnerUserId: 3, BotUserId: 7})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(context.Background(), newCtx())
			requireBizCode(t, err, errorx.CodeInternal, "conversation service not configured")
		})
	}
}

func TestBotServiceLogicMapsResponses(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)

	t.Run("GetBotWebhook reports an absent webhook", func(t *testing.T) {
		c, _, _ := workingBotSvcCtx()

		resp, err := NewGetBotWebhookLogic(context.Background(), c).GetBotWebhook(&pb.GetBotWebhookReq{BotUserId: 7})
		require.NoError(t, err)
		require.Nil(t, resp.GetWebhook())
	})

	t.Run("GetBotWebhook maps a configured webhook", func(t *testing.T) {
		c, q, _ := workingBotSvcCtx()
		q.webhook = &orm.BotWebhook{
			BotUserID: 7,
			Url:       "https://example.com/hook",
			Events:    []string{"message.created"},
			Enabled:   true,
			UpdatedAt: created,
		}

		resp, err := NewGetBotWebhookLogic(context.Background(), c).GetBotWebhook(&pb.GetBotWebhookReq{BotUserId: 7})
		require.NoError(t, err)
		require.Equal(t, &pb.BotWebhookConfig{
			BotUserId: 7,
			Url:       "https://example.com/hook",
			Events:    []string{"message.created"},
			Enabled:   true,
			UpdatedAt: created.UnixMilli(),
		}, resp.GetWebhook())
	})

	t.Run("ListBotConversations maps rows", func(t *testing.T) {
		c, q, _ := workingBotSvcCtx()
		q.convs = []orm.Conversation{
			{ID: 11, ConversationType: "group", Name: "team", Avatar: "a.png", CreatedAt: created},
			{ID: 12, ConversationType: "direct", Name: "", Avatar: "", CreatedAt: created},
		}

		resp, err := NewListBotConversationsLogic(context.Background(), c).ListBotConversations(
			&pb.ListBotConversationsReq{BotUserId: 7})
		require.NoError(t, err)
		require.Equal(t, []*pb.BotConversationItem{
			{ConversationId: 11, ConversationType: "group", Name: "team", Avatar: "a.png", CreatedAt: created.UnixMilli()},
			{ConversationId: 12, ConversationType: "direct", CreatedAt: created.UnixMilli()},
		}, resp.GetConversations())
	})

	t.Run("ListBotConversations returns an empty slice when the bot has none", func(t *testing.T) {
		c, _, _ := workingBotSvcCtx()

		resp, err := NewListBotConversationsLogic(context.Background(), c).ListBotConversations(
			&pb.ListBotConversationsReq{BotUserId: 7})
		require.NoError(t, err)
		require.NotNil(t, resp.GetConversations())
		require.Empty(t, resp.GetConversations())
	})

	t.Run("ResolveBotWebhookEventActions sorts the map into a stable list", func(t *testing.T) {
		c, q, _ := workingBotSvcCtx()
		q.eventAction = map[string]string{
			"message.created":         "bot.webhook.subscribe.message_created",
			"conversation.member_add": "bot.conversation.list",
		}

		resp, err := NewResolveBotWebhookEventActionsLogic(context.Background(), c).ResolveBotWebhookEventActions(
			&pb.ResolveBotWebhookEventActionsReq{Events: []string{"message.created", "conversation.member_add"}})
		require.NoError(t, err)
		require.Equal(t, []*pb.WebhookEventAction{
			{Event: "conversation.member_add", Action: "bot.conversation.list"},
			{Event: "message.created", Action: "bot.webhook.subscribe.message_created"},
		}, resp.GetEventActions())
	})

	t.Run("ResolveBotWebhookEventActions rejects an unknown event", func(t *testing.T) {
		c, _, _ := workingBotSvcCtx()

		_, err := NewResolveBotWebhookEventActionsLogic(context.Background(), c).ResolveBotWebhookEventActions(
			&pb.ResolveBotWebhookEventActionsReq{Events: []string{"nope"}})
		requireBizCode(t, err, errorx.CodeBotWebhookInvalid, "unsupported webhook event: nope")
	})

	t.Run("ValidateBotToken maps the identity", func(t *testing.T) {
		plaintext, err := bottoken.Generate()
		require.NoError(t, err)

		c, q, _ := workingBotSvcCtx()
		q.tokenRow = dao.BotTokenWithUser{
			BotToken:   orm.BotToken{ID: 21, BotUserID: 7},
			UserType:   service.UserTypeBot,
			UserStatus: 1,
			Nickname:   "bot",
			Avatar:     "a.png",
		}
		q.tokenActs = []string{"bot.message.send"}

		resp, err := NewValidateBotTokenLogic(context.Background(), c).ValidateBotToken(
			&pb.ValidateBotTokenReq{PlaintextToken: plaintext})
		require.NoError(t, err)
		require.Equal(t, &pb.BotIdentity{
			BotUserId:  7,
			TokenId:    21,
			Scopes:     []string{"bot.message.send"},
			Nickname:   "bot",
			Avatar:     "a.png",
			UserStatus: 1,
		}, resp.GetIdentity())
	})

	t.Run("GetBotProfile maps the profile", func(t *testing.T) {
		c, q, _ := workingBotSvcCtx()
		q.userInfo = orm.UserInfo{
			ID:        7,
			Email:     "bot@example.com",
			Status:    1,
			Nickname:  "bot",
			Avatar:    "a.png",
			UserType:  service.UserTypeBot,
			CreatedAt: created,
			UpdatedAt: created,
		}

		resp, err := NewGetBotProfileLogic(context.Background(), c).GetBotProfile(&pb.GetBotProfileReq{BotUserId: 7})
		require.NoError(t, err)
		require.Equal(t, &pb.UserInfoResponse{
			Id:        7,
			Email:     "bot@example.com",
			Status:    1,
			Nickname:  "bot",
			Avatar:    "a.png",
			CreatedAt: created.UnixMilli(),
			UpdatedAt: created.UnixMilli(),
		}, resp.GetProfile())
	})

	t.Run("GetUserBot maps the managed bot", func(t *testing.T) {
		c, _, m := workingBotSvcCtx()
		m.bot = dao.ManagedUserBot{
			ID:          7,
			Email:       "bot@example.com",
			Status:      1,
			Nickname:    "bot",
			Avatar:      "a.png",
			OwnerUserID: 3,
			CreatedAt:   created,
			UpdatedAt:   created,
		}

		resp, err := NewGetUserBotLogic(context.Background(), c).GetUserBot(
			&pb.GetUserBotReq{OwnerUserId: 3, BotUserId: 7})
		require.NoError(t, err)
		require.Equal(t, &pb.UserBotInfo{
			BotUserId:   7,
			OwnerUserId: 3,
			Email:       "bot@example.com",
			Nickname:    "bot",
			Avatar:      "a.png",
			Status:      1,
			CreatedAt:   created.UnixMilli(),
			UpdatedAt:   created.UnixMilli(),
		}, resp.GetBot())
	})
}
