package bot

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeLogicBotClient struct {
	resolveResp *pb.ResolveBotWebhookEventActionsResp
	resolveErr  error

	listConversationsCalled bool
	getWebhookCalled        bool
	setWebhookCalled        bool
	deleteWebhookCalled     bool
	setWebhookReq           *pb.SetBotWebhookReq
}

var _ botservice.BotService = (*fakeLogicBotClient)(nil)

func (f *fakeLogicBotClient) ValidateBotToken(context.Context, *pb.ValidateBotTokenReq, ...grpc.CallOption) (*pb.ValidateBotTokenResp, error) {
	return nil, nil
}

func (f *fakeLogicBotClient) GetBotProfile(context.Context, *pb.GetBotProfileReq, ...grpc.CallOption) (*pb.GetBotProfileResp, error) {
	return nil, nil
}

func (f *fakeLogicBotClient) ListBotConversations(context.Context, *pb.ListBotConversationsReq, ...grpc.CallOption) (*pb.ListBotConversationsResp, error) {
	f.listConversationsCalled = true

	return &pb.ListBotConversationsResp{}, nil
}

func (f *fakeLogicBotClient) GetBotWebhook(context.Context, *pb.GetBotWebhookReq, ...grpc.CallOption) (*pb.GetBotWebhookResp, error) {
	f.getWebhookCalled = true

	return &pb.GetBotWebhookResp{}, nil
}

func (f *fakeLogicBotClient) SetBotWebhook(_ context.Context, in *pb.SetBotWebhookReq, _ ...grpc.CallOption) (*pb.SetBotWebhookResp, error) {
	f.setWebhookCalled = true
	f.setWebhookReq = in

	return &pb.SetBotWebhookResp{Webhook: &pb.BotWebhookConfig{
		BotUserId: in.GetBotUserId(),
		Url:       in.GetUrl(),
		Events:    in.GetEvents(),
		Enabled:   in.GetEnabled(),
		UpdatedAt: 123,
	}}, nil
}

func (f *fakeLogicBotClient) DeleteBotWebhook(context.Context, *pb.DeleteBotWebhookReq, ...grpc.CallOption) (*pb.DeleteBotWebhookResp, error) {
	f.deleteWebhookCalled = true

	return &pb.DeleteBotWebhookResp{Deleted: true}, nil
}

func (f *fakeLogicBotClient) ResolveBotWebhookEventActions(context.Context, *pb.ResolveBotWebhookEventActionsReq, ...grpc.CallOption) (*pb.ResolveBotWebhookEventActionsResp, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}

	if f.resolveResp != nil {
		return f.resolveResp, nil
	}

	return &pb.ResolveBotWebhookEventActionsResp{EventActions: []*pb.WebhookEventAction{{
		Event:  botperm.WebhookEventMessageCreated,
		Action: botperm.ActionWebhookSubscribeMessageCreated,
	}}}, nil
}

func TestBotGetMe_RequiresSelfReadAction(t *testing.T) {
	l := NewBotGetMeLogic(ctxWithBot(botperm.ActionMessageSend), &svc.ServiceContext{})

	_, err := l.BotGetMe()

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
}

func TestBotGetMe_WithSelfReadAction(t *testing.T) {
	resp, err := NewBotGetMeLogic(ctxWithBot(botperm.ActionSelfRead), &svc.ServiceContext{}).BotGetMe()

	require.NoError(t, err)
	require.Equal(t, int64(1001), resp.Bot.BotUserId)
	require.Equal(t, []string{botperm.ActionSelfRead}, resp.Bot.Scopes)
}

func TestBotListConversations_RequiresListAction(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotListConversationsLogic(ctxWithBot(botperm.ActionSelfRead), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotListConversations()

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
	require.False(t, client.listConversationsCalled)
}

func TestBotGetWebhook_RequiresReadAction(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotGetWebhookLogic(ctxWithBot(botperm.ActionWebhookWrite), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotGetWebhook()

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
	require.False(t, client.getWebhookCalled)
}

func TestBotDeleteWebhook_RequiresDeleteAction(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotDeleteWebhookLogic(ctxWithBot(botperm.ActionWebhookRead), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotDeleteWebhook()

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
	require.False(t, client.deleteWebhookCalled)
}

func TestBotSetWebhook_RequiresWriteAction(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotSetWebhookLogic(ctxWithBot(botperm.ActionWebhookSubscribeMessageCreated), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotSetWebhook(&types.BotSetWebhookRequest{Url: "https://bot.test/hook", Secret: "secret"})

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
	require.False(t, client.setWebhookCalled)
}

func TestBotSetWebhook_RequiresEventAction(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotSetWebhookLogic(ctxWithBot(botperm.ActionWebhookWrite), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotSetWebhook(&types.BotSetWebhookRequest{Url: "https://bot.test/hook", Secret: "secret"})

	assertCodeError(t, err, errorx.CodeBotScopeDenied)
	require.False(t, client.setWebhookCalled)
}

func TestBotSetWebhook_PropagatesUnknownWebhookEvent(t *testing.T) {
	client := &fakeLogicBotClient{resolveErr: errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "unsupported webhook event: unknown.event")}
	l := NewBotSetWebhookLogic(ctxWithBot(botperm.ActionWebhookWrite), &svc.ServiceContext{LogicBotClient: client})

	_, err := l.BotSetWebhook(&types.BotSetWebhookRequest{
		Url:    "https://bot.test/hook",
		Secret: "secret",
		Events: []string{"unknown.event"},
	})

	assertCodeError(t, err, errorx.CodeBotWebhookInvalid)
	require.False(t, client.setWebhookCalled)
}

func TestBotSetWebhook_WithActions(t *testing.T) {
	client := &fakeLogicBotClient{}
	l := NewBotSetWebhookLogic(ctxWithBot(botperm.ActionWebhookWrite, botperm.ActionWebhookSubscribeMessageCreated), &svc.ServiceContext{LogicBotClient: client})

	resp, err := l.BotSetWebhook(&types.BotSetWebhookRequest{Url: "https://bot.test/hook", Secret: "secret", Events: []string{botperm.WebhookEventMessageCreated}})

	require.NoError(t, err)
	require.True(t, client.setWebhookCalled)
	require.Equal(t, "https://bot.test/hook", client.setWebhookReq.GetUrl())
	require.Equal(t, int64(1001), client.setWebhookReq.GetBotUserId())
	require.Equal(t, int64(123), resp.Webhook.UpdatedAt)
}

func assertCodeError(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, code, ce.Code)
}
