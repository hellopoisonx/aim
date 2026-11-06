package bot

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeMessageClient struct {
	called bool
	req    *logicpb.IngestMessageReq
	resp   *logicpb.IngestMessageResp
	err    error
}

func (f *fakeMessageClient) IngestMessage(_ context.Context, in *logicpb.IngestMessageReq, _ ...grpc.CallOption) (*logicpb.IngestMessageResp, error) {
	f.called = true
	f.req = in
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &logicpb.IngestMessageResp{
		MessageId:   42,
		ClientMsgId: in.ClientMsgId,
		AcceptedAt:  1700000000000,
	}, nil
}

func ctxWithBot(scopes ...string) context.Context {
	return botctx.WithIdentity(context.Background(), botctx.Identity{
		BotUserID: 1001,
		TokenID:   1,
		Scopes:    scopes,
	})
}

func TestBotSendMessage_RequiresScope(t *testing.T) {
	core := &fakeMessageClient{}
	l := NewBotSendMessageLogic(ctxWithBot("bot.conversation.list"), &svc.ServiceContext{MessageClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: "7",
		MessageType:    "text",
		Content:        "hi",
		ClientMsgId:    "client-1",
	})

	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotScopeDenied, ce.Code)
	require.False(t, core.called)
}

func TestBotSendMessage_ForwardsToCore(t *testing.T) {
	core := &fakeMessageClient{}
	l := NewBotSendMessageLogic(ctxWithBot("bot.message.send"), &svc.ServiceContext{MessageClient: core})

	resp, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: "7",
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
		Mentions:       []string{"42"},
	})

	require.NoError(t, err)
	require.Equal(t, "42", resp.MessageId)
	require.Equal(t, "client-1", resp.ClientMsgId)

	require.True(t, core.called)
	require.Equal(t, int64(1001), core.req.SenderId)
	require.Equal(t, "bot-api", core.req.DeviceId)
	require.Equal(t, int64(7), core.req.ConversationId)
	require.Equal(t, "text", core.req.MessageType)
	require.Equal(t, []string{"42"}, core.req.Mentions)
}

func TestBotSendMessage_ValidatesRequest(t *testing.T) {
	core := &fakeMessageClient{}
	l := NewBotSendMessageLogic(ctxWithBot("bot.message.send"), &svc.ServiceContext{MessageClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{ConversationId: "0", MessageType: "text", ClientMsgId: "x"})
	require.Error(t, err)
	require.False(t, core.called)
}

func TestBotSendMessage_PropagatesCoreError(t *testing.T) {
	rateLimit := errorx.NewCodeError(errorx.CodeRateLimit, "rate limit")
	core := &fakeMessageClient{err: rateLimit}
	l := NewBotSendMessageLogic(ctxWithBot("bot.message.send"), &svc.ServiceContext{MessageClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: "7",
		MessageType:    "text",
		Content:        "hi",
		ClientMsgId:    "x",
	})
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	// Round-trip through gRPC collapses the precise code into the category.
	require.Equal(t, errorx.CodeRateLimit, ce.Code)
}

func TestBotSendMessage_RequiresIdentity(t *testing.T) {
	l := NewBotSendMessageLogic(context.Background(), &svc.ServiceContext{})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: "7",
		MessageType:    "text",
		Content:        "hi",
		ClientMsgId:    "x",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, errorx.NewCodeError(errorx.CodeBotTokenInvalid, ""))
}
