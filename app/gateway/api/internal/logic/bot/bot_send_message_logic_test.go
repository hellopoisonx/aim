package bot

import (
	"context"
	"errors"
	"testing"

	corepb "github.com/hellopoisonx/aim/app/core/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeCoreClient struct {
	called bool
	req    *corepb.TransferReq
	resp   *corepb.TransferResp
	err    error
}

var _ corepb.TransferServiceClient = (*fakeCoreClient)(nil)

func (f *fakeCoreClient) Transfer(_ context.Context, in *corepb.TransferReq, _ ...grpc.CallOption) (*corepb.TransferResp, error) {
	f.called = true
	f.req = in
	if f.err != nil {
		return nil, f.err
	}
	if f.resp != nil {
		return f.resp, nil
	}
	return &corepb.TransferResp{
		MessageId:   42,
		ClientMsgId: in.ClientMsgId,
		AcceptedAt:  1700000000000,
	}, nil
}

func ctxWithBot(scopes ...string) context.Context {
	return botctx.WithBotIdentity(context.Background(), botctx.BotIdentity{
		BotUserID: 1001,
		TokenID:   1,
		Scopes:    scopes,
	})
}

func TestBotSendMessage_RequiresScope(t *testing.T) {
	core := &fakeCoreClient{}
	l := NewBotSendMessageLogic(ctxWithBot("messages:receive"), &svc.ServiceContext{CoreClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: 7,
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
	core := &fakeCoreClient{}
	l := NewBotSendMessageLogic(ctxWithBot("messages:send"), &svc.ServiceContext{CoreClient: core})

	resp, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: 7,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgId:    "client-1",
		Mentions:       []string{"42"},
	})

	require.NoError(t, err)
	require.Equal(t, int64(42), resp.MessageId)
	require.Equal(t, "client-1", resp.ClientMsgId)

	require.True(t, core.called)
	require.Equal(t, int64(1001), core.req.SenderId)
	require.Equal(t, "bot-api", core.req.DeviceId)
	require.Equal(t, int64(7), core.req.ConversationId)
	require.Equal(t, "text", core.req.MessageType)
	require.Equal(t, []string{"42"}, core.req.Mentions)
}

func TestBotSendMessage_ValidatesRequest(t *testing.T) {
	core := &fakeCoreClient{}
	l := NewBotSendMessageLogic(ctxWithBot("messages:send"), &svc.ServiceContext{CoreClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{ConversationId: 0, MessageType: "text", ClientMsgId: "x"})
	require.Error(t, err)
	require.False(t, core.called)
}

func TestBotSendMessage_PropagatesCoreError(t *testing.T) {
	rateLimit := errorx.NewCodeError(errorx.CodeRateLimit, "rate limit")
	core := &fakeCoreClient{err: rateLimit}
	l := NewBotSendMessageLogic(ctxWithBot("messages:send"), &svc.ServiceContext{CoreClient: core})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: 7,
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

func TestBotSendMessage_NoCoreClient(t *testing.T) {
	l := NewBotSendMessageLogic(ctxWithBot("messages:send"), &svc.ServiceContext{})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: 7,
		MessageType:    "text",
		Content:        "hi",
		ClientMsgId:    "x",
	})
	require.Error(t, err)
}

func TestBotSendMessage_RequiresIdentity(t *testing.T) {
	l := NewBotSendMessageLogic(context.Background(), &svc.ServiceContext{})

	_, err := l.BotSendMessage(&types.BotSendMessageRequest{
		ConversationId: 7,
		MessageType:    "text",
		Content:        "hi",
		ClientMsgId:    "x",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "")))
}
