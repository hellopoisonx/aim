package search

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/searchservice"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSearchClient struct {
	searchservice.SearchService

	got  *searchservice.UnifiedSearchReq
	resp *searchservice.UnifiedSearchResp
	err  error
}

func (f *fakeSearchClient) UnifiedSearch(_ context.Context, in *searchservice.UnifiedSearchReq, _ ...grpc.CallOption) (*searchservice.UnifiedSearchResp, error) {
	f.got = in

	return f.resp, f.err
}

var _ searchservice.SearchService = (*fakeSearchClient)(nil)

func TestSearchRequiresIdentity(t *testing.T) {
	t.Parallel()

	client := &fakeSearchClient{}
	_, err := NewSearchLogic(context.Background(), &svc.ServiceContext{LogicSearchClient: client}).
		Search(&types.SearchRequest{Query: "hi"})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeAuth, codeErr.Code)
	require.Nil(t, client.got, "upstream must not be called without an authenticated caller")
}

func TestSearchForwardsScopeAndMapsResponse(t *testing.T) {
	t.Parallel()

	client := &fakeSearchClient{resp: &searchservice.UnifiedSearchResp{
		Conversations: []*logicpb.SearchConversationResult{{
			Conversation: &logicpb.ConversationResponse{Id: 7, Name: "team"},
			Snippet:      "team",
		}},
		Messages: []*logicpb.SearchMessageResult{{
			Message: &logicpb.MessageItem{Id: 99, ConversationId: 7, Content: "hello team"},
			Snippet: "hello <em>team</em>",
		}},
		NextCursorCreatedAt: 1700000000000,
		NextCursorId:        99,
		HasMore:             true,
	}}
	logic := NewSearchLogic(authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 42}), &svc.ServiceContext{LogicSearchClient: client})

	resp, err := logic.Search(&types.SearchRequest{
		Query:           "team",
		Scope:           " conversations , messages ",
		ConversationId:  7,
		CursorCreatedAt: 1699999999999,
		CursorId:        50,
		Limit:           20,
	})
	require.NoError(t, err)

	require.Equal(t, int64(42), client.got.GetUserId())
	require.Equal(t, "team", client.got.GetQuery())
	require.Equal(t, []string{"conversations", "messages"}, client.got.GetScopes(), "scope is split and trimmed")
	require.Equal(t, int64(7), client.got.GetConversationId())
	require.Equal(t, int64(1699999999999), client.got.GetCursorCreatedAt())
	require.Equal(t, int64(50), client.got.GetCursorId())
	require.Equal(t, int32(20), client.got.GetLimit())

	require.Equal(t, []types.SearchConversationResultItem{{
		Conversation: types.ConversationItem{ConversationId: 7, Name: "team"},
		Snippet:      "team",
	}}, resp.Conversations)
	require.Equal(t, []types.SearchMessageResultItem{{
		Message: types.MessageItem{Id: 99, ConversationId: 7, Content: "hello team"},
		Snippet: "hello <em>team</em>",
	}}, resp.Messages)
	require.Equal(t, int64(1700000000000), resp.NextCursorCreatedAt)
	require.Equal(t, int64(99), resp.NextCursorId)
	require.True(t, resp.HasMore)
}

func TestSearchWithoutScopeSendsNilScopes(t *testing.T) {
	t.Parallel()

	client := &fakeSearchClient{}
	logic := NewSearchLogic(authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 42}), &svc.ServiceContext{LogicSearchClient: client})

	resp, err := logic.Search(&types.SearchRequest{Query: "team"})
	require.NoError(t, err)

	require.Nil(t, client.got.GetScopes(), "an empty scope must stay empty rather than become [\"\"]")
	require.Empty(t, resp.Conversations)
	require.Empty(t, resp.Messages)
}

func TestSearchSanitizesUpstreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		upstream error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "infrastructure error is hidden",
			upstream: status.Error(codes.Unavailable, "elasticsearch password=leaked"),
			wantCode: errorx.CodeInternal,
			wantMsg:  "internal error",
		},
		{
			name:     "client-visible grpc error keeps its message",
			upstream: status.Error(codes.InvalidArgument, "query too long"),
			wantCode: errorx.CodeBadInput,
			wantMsg:  "query too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeSearchClient{err: tt.upstream}
			logic := NewSearchLogic(authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 42}), &svc.ServiceContext{LogicSearchClient: client})

			_, err := logic.Search(&types.SearchRequest{Query: "team"})

			var codeErr *errorx.CodeError
			require.ErrorAs(t, err, &codeErr)
			require.Equal(t, tt.wantCode, codeErr.Code)
			require.Equal(t, tt.wantMsg, codeErr.Message)
		})
	}
}

func TestMapSearchConversations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []*logicpb.SearchConversationResult
		want  []types.SearchConversationResultItem
	}{
		{
			name:  "nil input yields an empty non-nil slice",
			items: nil,
			want:  []types.SearchConversationResultItem{},
		},
		{
			name:  "result without a conversation yields zero values",
			items: []*logicpb.SearchConversationResult{{Snippet: "orphan"}},
			want:  []types.SearchConversationResultItem{{Snippet: "orphan"}},
		},
		{
			name: "all fields are mapped and the snippet passes through",
			items: []*logicpb.SearchConversationResult{{
				Conversation: &logicpb.ConversationResponse{
					Id:               7,
					ConversationType: "group",
					IsActive:         true,
					CreatedAt:        1700000000000,
					MemberIds:        []int64{7, 42},
					Name:             "team",
					Avatar:           "avatar",
					CreatorId:        7,
				},
				Snippet: "<em>team</em> standup",
			}},
			want: []types.SearchConversationResultItem{{
				Conversation: types.ConversationItem{
					ConversationId:   7,
					ConversationType: "group",
					IsActive:         true,
					CreatedAt:        1700000000000,
					MemberIds:        []int64{7, 42},
					Name:             "team",
					Avatar:           "avatar",
					CreatorId:        7,
				},
				Snippet: "<em>team</em> standup",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, mapSearchConversations(tt.items))
		})
	}
}

func TestMapSearchMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []*logicpb.SearchMessageResult
		want  []types.SearchMessageResultItem
	}{
		{
			name:  "nil input yields an empty non-nil slice",
			items: nil,
			want:  []types.SearchMessageResultItem{},
		},
		{
			name:  "result without a message yields zero values",
			items: []*logicpb.SearchMessageResult{{Snippet: "orphan"}},
			want:  []types.SearchMessageResultItem{{Snippet: "orphan"}},
		},
		{
			name: "all mapped fields are copied, unexposed ones are dropped, snippet passes through",
			items: []*logicpb.SearchMessageResult{{
				Message: &logicpb.MessageItem{
					Id:             99,
					ConversationId: 7,
					SenderId:       42,
					MessageType:    "text",
					Content:        "hello team",
					ClientMsgId:    "client-1",
					CreatedAt:      1700000000000,
					SenderInfo:     &logicpb.SenderInfo{Name: "alice", Email: "a@example.com"},
					Mentions:       []string{"7"},
					IsSystem:       true,
				},
				Snippet: "hello <em>team</em>",
			}},
			want: []types.SearchMessageResultItem{{
				Message: types.MessageItem{
					Id:             99,
					ConversationId: 7,
					SenderId:       42,
					MessageType:    "text",
					Content:        "hello team",
					ClientMsgId:    "client-1",
					CreatedAt:      1700000000000,
					IsSystem:       true,
				},
				Snippet: "hello <em>team</em>",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, mapSearchMessages(tt.items))
		})
	}
}
