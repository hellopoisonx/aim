package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeBotClient struct {
	resp *pb.ValidateBotTokenResp
	err  error
}

var _ botservice.BotService = (*fakeBotClient)(nil)

func (f *fakeBotClient) ValidateBotToken(_ context.Context, _ *pb.ValidateBotTokenReq, _ ...grpc.CallOption) (*pb.ValidateBotTokenResp, error) {
	return f.resp, f.err
}
func (f *fakeBotClient) GetBotProfile(_ context.Context, _ *pb.GetBotProfileReq, _ ...grpc.CallOption) (*pb.GetBotProfileResp, error) {
	return nil, nil
}
func (f *fakeBotClient) ListBotConversations(_ context.Context, _ *pb.ListBotConversationsReq, _ ...grpc.CallOption) (*pb.ListBotConversationsResp, error) {
	return nil, nil
}
func (f *fakeBotClient) GetBotWebhook(_ context.Context, _ *pb.GetBotWebhookReq, _ ...grpc.CallOption) (*pb.GetBotWebhookResp, error) {
	return nil, nil
}
func (f *fakeBotClient) SetBotWebhook(_ context.Context, _ *pb.SetBotWebhookReq, _ ...grpc.CallOption) (*pb.SetBotWebhookResp, error) {
	return nil, nil
}
func (f *fakeBotClient) DeleteBotWebhook(_ context.Context, _ *pb.DeleteBotWebhookReq, _ ...grpc.CallOption) (*pb.DeleteBotWebhookResp, error) {
	return nil, nil
}
func (f *fakeBotClient) ResolveBotWebhookEventActions(_ context.Context, _ *pb.ResolveBotWebhookEventActionsReq, _ ...grpc.CallOption) (*pb.ResolveBotWebhookEventActionsResp, error) {
	return &pb.ResolveBotWebhookEventActionsResp{}, nil
}

func TestExtractBotToken(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		wantTok string
		wantErr int
	}{
		{"missing", "", "", errorx.CodeBotTokenInvalid},
		{"wrong scheme", "Bearer abc", "", errorx.CodeBotTokenInvalid},
		{"empty token", "Bot ", "", errorx.CodeBotTokenInvalid},
		{"ok", "Bot abc123", "abc123", 0},
		{"ok lowercase scheme", "bot abc123", "abc123", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tok, err := extractBotToken(c.header)
			if c.wantErr != 0 {
				require.Error(t, err)
				ce, ok := err.(*errorx.CodeError)
				require.True(t, ok)
				require.Equal(t, c.wantErr, ce.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.wantTok, tok)
		})
	}
}

func TestBotAuthMiddleware_Success(t *testing.T) {
	mw := NewBotAuthMiddleware(&fakeBotClient{
		resp: &pb.ValidateBotTokenResp{
			Identity: &pb.BotIdentity{
				BotUserId: 1001,
				TokenId:   42,
				Scopes:    []string{"bot.message.send"},
				Nickname:  "alice",
			},
		},
	})

	called := false
	handler := mw.Handle(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		identity, ok := botctx.FromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, int64(1001), identity.BotUserID)
		assert.True(t, identity.HasAction("bot.message.send"))
		assert.False(t, identity.HasScope("admin"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/bot/v1/me", nil)
	req.Header.Set("Authorization", "Bot some-token")
	rr := httptest.NewRecorder()
	handler(rr, req)

	require.True(t, called)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestBotAuthMiddleware_RejectsMissingHeader(t *testing.T) {
	mw := NewBotAuthMiddleware(&fakeBotClient{})

	called := false
	handler := mw.Handle(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/api/bot/v1/me", nil))

	require.False(t, called)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	require.Equal(t, errorx.CodeBotTokenInvalid, body.Code)
}

func TestBotAuthMiddleware_PropagatesGRPCError(t *testing.T) {
	revoked := errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token revoked")
	mw := NewBotAuthMiddleware(&fakeBotClient{err: revoked})

	called := false
	handler := mw.Handle(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/bot/v1/me", nil)
	req.Header.Set("Authorization", "Bot doomed")
	rr := httptest.NewRecorder()
	handler(rr, req)

	require.False(t, called)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	body, _ := io.ReadAll(rr.Body)
	require.Contains(t, string(body), "revoked")
}

func TestBotAuthMiddleware_NilIdentityRejected(t *testing.T) {
	mw := NewBotAuthMiddleware(&fakeBotClient{resp: &pb.ValidateBotTokenResp{}})

	handler := mw.Handle(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler must not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/bot/v1/me", nil)
	req.Header.Set("Authorization", "Bot blank-resp")
	rr := httptest.NewRecorder()
	handler(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestBotAuthMiddleware_NilClientFailsClosed(t *testing.T) {
	mw := NewBotAuthMiddleware(nil)

	handler := mw.Handle(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler must not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/bot/v1/me", nil)
	req.Header.Set("Authorization", "Bot anything")
	rr := httptest.NewRecorder()
	handler(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}
