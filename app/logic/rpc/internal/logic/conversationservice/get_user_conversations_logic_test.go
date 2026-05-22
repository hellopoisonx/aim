package conversationservicelogic

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserConversationsLogic(t *testing.T) {
	store := newFakeStore()
	convID1 := generateTestConversationID()
	store.conversations[convID1] = newConv(convID1, "direct")
	store.members[convID1] = newMember(convID1, 1)

	convID2 := generateTestConversationID()
	store.conversations[convID2] = newConv(convID2, "group")
	store.members[convID2] = newMember(convID2, 1, 3)

	// convID3: user 2 only (not user 1)
	convID3 := generateTestConversationID()
	store.conversations[convID3] = newConv(convID3, "direct")
	store.members[convID3] = newMember(convID3, 2, 4)

	convSvc := service.NewConversationService(store, testSnowflake)
	svcCtx := &svc.ServiceContext{
		ConversationService: convSvc,
	}

	tests := []struct {
		name    string
		req     *pb.GetUserConversationsReq
		wantErr bool
		wantLen int
	}{
		{
			name: "user with 2 conversations",
			req: &pb.GetUserConversationsReq{
				UserId: 1,
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "user with 1 conversation",
			req: &pb.GetUserConversationsReq{
				UserId: 2,
			},
			wantErr: false,
			wantLen: 1,
		},
		{
			name: "user with no conversations",
			req: &pb.GetUserConversationsReq{
				UserId: 999,
			},
			wantErr: false,
			wantLen: 0,
		},
		{
			name: "zero user id",
			req: &pb.GetUserConversationsReq{
				UserId: 0,
			},
			wantErr: true,
		},
		{
			name: "negative user id",
			req: &pb.GetUserConversationsReq{
				UserId: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewGetUserConversationsLogic(context.Background(), svcCtx)
			resp, err := l.GetUserConversations(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.GetConversations(), tt.wantLen)
		})
	}
}

func TestGetUserConversationsLogic_NilService(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ConversationService: nil,
	}
	l := NewGetUserConversationsLogic(context.Background(), svcCtx)
	_, err := l.GetUserConversations(&pb.GetUserConversationsReq{
		UserId: 1,
	})
	require.Error(t, err)
}

// Helper functions updated for member creation

func newConv(id int64, convType string) model.Conversation {
	return model.Conversation{
		ID:               id,
		ConversationType: convType,
		IsActive:         true,
		CreatedAt:        newTS(),
	}
}

func newMember(convID int64, userIDs ...int64) []model.ConversationMember {
	members := make([]model.ConversationMember, len(userIDs))
	for i, uid := range userIDs {
		members[i] = model.ConversationMember{
			ConversationID: convID,
			UserID:         uid,
			JoinedAt:       newTS(),
		}
	}
	return members
}
