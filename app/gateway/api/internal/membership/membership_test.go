package membership

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeConversationService struct {
	conversationservice.ConversationService

	got    *conversationservice.GetConversationMembersReq
	resp   *conversationservice.GetConversationMembersResp
	err    error
	called bool
}

func (f *fakeConversationService) GetConversationMembers(_ context.Context, in *conversationservice.GetConversationMembersReq, _ ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	f.called = true
	f.got = in

	return f.resp, f.err
}

var _ conversationservice.ConversationService = (*fakeConversationService)(nil)

func TestRequireMember(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		conversationID int64
		callerID       int64
		actor          string
		resp           *conversationservice.GetConversationMembersResp
		clientErr      error
		wantLookup     bool
		wantCode       int
		wantMsg        string
	}{
		{
			name:           "zero conversation id is rejected without a lookup",
			conversationID: 0,
			callerID:       42,
			actor:          ActorUser,
			wantCode:       errorx.CodeBadInput,
			wantMsg:        "conversation id must be positive",
		},
		{
			name:           "negative conversation id is rejected without a lookup",
			conversationID: -3,
			callerID:       42,
			actor:          ActorUser,
			wantCode:       errorx.CodeBadInput,
			wantMsg:        "conversation id must be positive",
		},
		{
			name:           "infrastructure lookup error is sanitized",
			conversationID: 7,
			callerID:       42,
			actor:          ActorUser,
			clientErr:      status.Error(codes.Unavailable, "postgres password=leaked"),
			wantLookup:     true,
			wantCode:       errorx.CodeInternal,
			wantMsg:        "internal error",
		},
		{
			name:           "empty member list forbids the caller",
			conversationID: 7,
			callerID:       42,
			actor:          ActorUser,
			wantLookup:     true,
			wantCode:       errorx.CodeForbidden,
			wantMsg:        "user is not a member of this conversation",
		},
		{
			name:           "caller absent from member ids is forbidden",
			conversationID: 7,
			callerID:       42,
			actor:          ActorUser,
			resp:           &conversationservice.GetConversationMembersResp{MemberIds: []int64{1, 2}},
			wantLookup:     true,
			wantCode:       errorx.CodeForbidden,
			wantMsg:        "user is not a member of this conversation",
		},
		{
			name:           "bot actor is named in the forbidden message",
			conversationID: 7,
			callerID:       900,
			actor:          ActorBot,
			resp:           &conversationservice.GetConversationMembersResp{MemberIds: []int64{1, 2}},
			wantLookup:     true,
			wantCode:       errorx.CodeForbidden,
			wantMsg:        "bot is not a member of this conversation",
		},
		{
			name:           "member passes",
			conversationID: 7,
			callerID:       42,
			actor:          ActorUser,
			resp:           &conversationservice.GetConversationMembersResp{MemberIds: []int64{1, 42}},
			wantLookup:     true,
		},
	}

	logger := logx.WithContext(context.Background())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeConversationService{resp: tt.resp, err: tt.clientErr}

			err := RequireMember(context.Background(), logger, client, tt.conversationID, tt.callerID, tt.actor)

			require.Equal(t, tt.wantLookup, client.called)
			if tt.wantLookup {
				require.Equal(t, tt.conversationID, client.got.GetConversationId())
			}

			if tt.wantCode == 0 {
				require.NoError(t, err)

				return
			}

			var codeErr *errorx.CodeError
			require.ErrorAs(t, err, &codeErr)
			require.Equal(t, tt.wantCode, codeErr.Code)
			require.Equal(t, tt.wantMsg, codeErr.Message)
		})
	}
}
