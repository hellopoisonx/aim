package conversationservicelogic

import (
	"context"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTS() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

// fakeConversationStore implements service.ConversationStore for testing.
type fakeConversationStore struct {
	conversations map[int64]model.Conversation
	members       map[int64][]model.ConversationMember
	messages      map[int64][]model.Message
}

func newFakeStore() *fakeConversationStore {
	return &fakeConversationStore{
		conversations: make(map[int64]model.Conversation),
		members:       make(map[int64][]model.ConversationMember),
		messages:      make(map[int64][]model.Message),
	}
}

func (f *fakeConversationStore) CreateConversation(_ context.Context, arg model.CreateConversationParams) (model.Conversation, error) {
	conv := model.Conversation{
		ID:               arg.ID,
		ConversationType: arg.ConversationType,
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	f.conversations[arg.ID] = conv
	return conv, nil
}

func (f *fakeConversationStore) GetConversation(_ context.Context, id int64) (model.Conversation, error) {
	conv, ok := f.conversations[id]
	if !ok {
		return model.Conversation{}, pgx.ErrNoRows
	}
	return conv, nil
}

func (f *fakeConversationStore) AddConversationMembers(_ context.Context, arg model.AddConversationMembersParams) (int64, error) {
	f.members[arg.ConversationID] = append(f.members[arg.ConversationID], model.ConversationMember{
		ConversationID: arg.ConversationID,
		UserID:         arg.UserID,
		JoinedAt:       newTS(),
	})
	return 1, nil
}

func (f *fakeConversationStore) GetConversationMembers(_ context.Context, conversationID int64) ([]model.ConversationMember, error) {
	return f.members[conversationID], nil
}

func (f *fakeConversationStore) GetConversationsByUserID(_ context.Context, userID int64) ([]model.Conversation, error) {
	var result []model.Conversation
	for convID, conv := range f.conversations {
		for _, m := range f.members[convID] {
			if m.UserID == userID {
				result = append(result, conv)
				break
			}
		}
	}
	return result, nil
}

func (f *fakeConversationStore) ListMessagesByConversation(_ context.Context, arg model.ListMessagesByConversationParams) ([]model.Message, error) {
	return f.messages[arg.ConversationID], nil
}

func (f *fakeConversationStore) ListMessagesByConversationInitial(_ context.Context, arg model.ListMessagesByConversationInitialParams) ([]model.Message, error) {
	return f.messages[arg.ConversationID], nil
}

func (f *fakeConversationStore) CountMessagesByConversation(_ context.Context, conversationID int64) (int64, error) {
	return int64(len(f.messages[conversationID])), nil
}

func (f *fakeConversationStore) GetDirectConversationByMembers(_ context.Context, arg model.GetDirectConversationByMembersParams) (model.Conversation, error) {
	for convID, conv := range f.conversations {
		if conv.ConversationType != "direct" || !conv.IsActive {
			continue
		}
		memberSet := make(map[int64]bool)
		for _, m := range f.members[convID] {
			memberSet[m.UserID] = true
		}
		if memberSet[arg.UserID] && memberSet[arg.UserID_2] {
			return conv, nil
		}
	}
	return model.Conversation{}, pgx.ErrNoRows
}

// testSnowflake is a shared Snowflake instance for tests.
var testSnowflake = func() *tools.Snowflake {
	s, err := tools.NewSnowflake(1)
	if err != nil {
		panic(err)
	}
	return s
}()

func generateTestConversationID() int64 {
	id, err := testSnowflake.NextID()
	if err != nil {
		panic(err)
	}
	return id
}

func setupTestSvc() *svc.ServiceContext {
	store := newFakeStore()
	convSvc := service.NewConversationService(store, testSnowflake)
	return &svc.ServiceContext{
		ConversationService: convSvc,
	}
}

func TestCreateConversationLogic(t *testing.T) {
	svcCtx := setupTestSvc()

	tests := []struct {
		name    string
		req     *pb.CreateConversationReq
		wantErr bool
	}{
		{
			name: "valid direct conversation",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{1, 2},
			},
			wantErr: false,
		},
		{
			name: "valid group conversation",
			req: &pb.CreateConversationReq{
				ConversationType: "group",
				CreatorId:        1,
				MemberIds:        []int64{1, 2, 3},
			},
			wantErr: false,
		},
		{
			name: "direct with more than 2 members",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{1, 2, 3},
			},
			wantErr: true,
		},
		{
			name: "invalid conversation type",
			req: &pb.CreateConversationReq{
				ConversationType: "channel",
				CreatorId:        1,
				MemberIds:        []int64{1, 2},
			},
			wantErr: true,
		},
		{
			name: "empty member ids",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{},
			},
			wantErr: true,
		},
		{
			name: "creator auto-added",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{2},
			},
			wantErr: false,
		},
		{
			name: "zero creator id",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        0,
				MemberIds:        []int64{1, 2},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewCreateConversationLogic(context.Background(), svcCtx)
			resp, err := l.CreateConversation(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.NotNil(t, resp.Conversation)
			assert.Equal(t, tt.req.GetConversationType(), resp.Conversation.GetConversationType())
			assert.True(t, resp.Conversation.GetIsActive())
			assert.Greater(t, resp.Conversation.GetId(), int64(0))
		})
	}
}

func TestGetConversationHistoryLogic(t *testing.T) {
	store := newFakeStore()
	convID := generateTestConversationID()
	store.conversations[convID] = model.Conversation{
		ID:               convID,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.members[convID] = []model.ConversationMember{
		{ConversationID: convID, UserID: 1, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 2, JoinedAt: newTS()},
	}
	store.messages[convID] = []model.Message{
		{ID: 1, ConversationID: convID, SenderID: 1, MessageType: "text", Content: []byte(`"hello"`), CreatedAt: newTS()},
		{ID: 2, ConversationID: convID, SenderID: 2, MessageType: "text", Content: []byte(`"world"`), CreatedAt: newTS()},
	}

	convSvc := service.NewConversationService(store, testSnowflake)
	svcCtx := &svc.ServiceContext{
		ConversationService: convSvc,
	}

	tests := []struct {
		name    string
		req     *pb.GetConversationHistoryReq
		wantErr bool
		wantLen int
	}{
		{
			name: "initial page",
			req: &pb.GetConversationHistoryReq{
				ConversationId: convID,
				Limit:          50,
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "conversation not found",
			req: &pb.GetConversationHistoryReq{
				ConversationId: 99999,
				Limit:          50,
			},
			wantErr: true,
		},
		{
			name: "default limit applied",
			req: &pb.GetConversationHistoryReq{
				ConversationId: convID,
				Limit:          0,
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "empty conversation id",
			req: &pb.GetConversationHistoryReq{
				ConversationId: 0,
				Limit:          50,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewGetConversationHistoryLogic(context.Background(), svcCtx)
			resp, err := l.GetConversationHistory(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.GetMessages(), tt.wantLen)
		})
	}
}

func TestGetConversationHistoryLogic_NilService(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ConversationService: nil,
	}
	l := NewGetConversationHistoryLogic(context.Background(), svcCtx)
	_, err := l.GetConversationHistory(&pb.GetConversationHistoryReq{
		ConversationId: 1,
		Limit:          50,
	})
	require.Error(t, err)
}

func TestCreateConversationLogic_NilService(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		ConversationService: nil,
	}
	l := NewCreateConversationLogic(context.Background(), svcCtx)
	_, err := l.CreateConversation(&pb.CreateConversationReq{
		ConversationType: "direct",
		CreatorId:        1,
		MemberIds:        []int64{1, 2},
	})
	require.Error(t, err)
}
func TestGetConversationMembersLogic(t *testing.T) {
	store := newFakeStore()
	convID := generateTestConversationID()
	store.conversations[convID] = model.Conversation{
		ID:               convID,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.members[convID] = []model.ConversationMember{
		{ConversationID: convID, UserID: 1, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 2, JoinedAt: newTS()},
	}

	svcCtx := &svc.ServiceContext{ConversationService: service.NewConversationService(store, testSnowflake)}
	resp, err := NewGetConversationMembersLogic(context.Background(), svcCtx).GetConversationMembers(&pb.GetConversationMembersReq{ConversationId: convID})
	require.NoError(t, err)
	require.Equal(t, convID, resp.GetConversationId())
	require.Equal(t, []int64{1, 2}, resp.GetMemberIds())
}

func TestGetConversationMembersLogic_InvalidRequest(t *testing.T) {
	_, err := NewGetConversationMembersLogic(context.Background(), setupTestSvc()).GetConversationMembers(&pb.GetConversationMembersReq{})
	require.Error(t, err)
}
