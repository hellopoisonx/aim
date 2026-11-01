package conversationservicelogic

import (
	"context"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/shared/tools"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/servicefakes"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSnowflake = func() *tools.Snowflake {
	gen, err := tools.NewSnowflake(1)
	if err != nil {
		panic(err)
	}

	return gen
}()

func newTS() time.Time {
	return time.Now().UTC()
}

// generateTestConversationID returns a unique id for the fake store.
func generateTestConversationID() int64 {
	id, err := testSnowflake.NextID()
	if err != nil {
		panic(err)
	}

	return id
}

func setupTestSvc() *svc.ServiceContext {
	store := servicefakes.NewConversationStore()
	convSvc := service.NewConversationService(store, testSnowflake, nil)

	return &svc.ServiceContext{ConversationService: convSvc}
}

func TestCreateConversationLogic(t *testing.T) {
	svcCtx := setupTestSvc()

	tests := []struct {
		name    string
		req     *pb.CreateConversationReq
		wantErr bool
	}{
		{
			name: "valid direct conversation with empty name",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{2},
				Name:             "",
			},
			wantErr: false,
		},
		{
			name: "valid group conversation",
			req: &pb.CreateConversationReq{
				ConversationType: "group",
				CreatorId:        1,
				MemberIds:        []int64{1, 2, 3},
				Name:             "Group Chat",
			},
			wantErr: false,
		},
		{
			name: "direct with more than one peer in request",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{2, 3},
				Name:             "Invalid Direct",
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
			name: "missing group name",
			req: &pb.CreateConversationReq{
				ConversationType: "group",
				CreatorId:        1,
				MemberIds:        []int64{2},
				Name:             "   ",
			},
			wantErr: true,
		},
		{
			name: "creator auto-added",
			req: &pb.CreateConversationReq{
				ConversationType: "direct",
				CreatorId:        1,
				MemberIds:        []int64{2},
				Name:             "Auto Add",
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
			assert.Positive(t, resp.Conversation.GetId())
		})
	}
}

func TestGetConversationHistoryLogic(t *testing.T) {
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 1, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 2, JoinedAt: newTS()},
	}
	clientMsg := "client-1"
	store.Messages[convID] = []orm.Message{
		{ID: 1, ConversationID: convID, SenderID: 1, MessageType: "text", Content: []byte(`"hello"`), ClientMsgID: &clientMsg, Mentions: []byte(`["2"]`), CreatedAt: newTS()},
		{ID: 2, ConversationID: convID, SenderID: 2, MessageType: "text", Content: []byte(`"world"`), CreatedAt: newTS()},
	}

	convSvc := service.NewConversationService(store, testSnowflake, nil, nil)
	svcCtx := &svc.ServiceContext{
		ConversationService: convSvc,
		UserInfoService: &fakeUserInfoService{users: map[int64]orm.UserInfo{
			1: {ID: 1, Email: "user1@example.com", Nickname: "User 1"},
			2: {ID: 2, Email: "user2@example.com", Nickname: "User 2"},
		}},
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
			if tt.name == "initial page" && len(resp.GetMessages()) > 0 {
				assert.Equal(t, "hello", resp.GetMessages()[0].GetContent())
				assert.Equal(t, "world", resp.GetMessages()[1].GetContent())
				assert.Equal(t, "client-1", resp.GetMessages()[0].GetClientMsgId())
				assert.Equal(t, []string{"2"}, resp.GetMessages()[0].GetMentions())
				// Verify per-message read details exist and exclude the sender.
				details := resp.GetMessages()[0].GetReadDetails()
				assert.NotEmpty(t, details, "expected read_details on each message")
				for _, rd := range details {
					assert.NotEqual(t, int64(1), rd.GetUserId(), "sender should be excluded from read_details")
				}
			}
		})
	}
}

func TestMessageContentFromJSONB(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "json string", raw: []byte(`"hello"`), want: "hello"},
		{name: "json string with escaped quote", raw: []byte(`"say \"hi\""`), want: `say "hi"`},
		{name: "json object", raw: []byte(`{"event":"member_joined"}`), want: `{"event":"member_joined"}`},
		{name: "plain fallback", raw: []byte(`plain text`), want: `plain text`},
		{name: "empty", raw: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, messageContentFromJSONB(tt.raw))
		})
	}
}

func TestGetConversationHistoryLogic_ReadDetails(t *testing.T) {
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 1, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 2, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 3, JoinedAt: newTS()},
	}
	store.Messages[convID] = []orm.Message{
		{ID: 10, ConversationID: convID, SenderID: 1, MessageType: "text", Content: []byte(`"hi"`), CreatedAt: newTS()},
		{ID: 20, ConversationID: convID, SenderID: 2, MessageType: "text", Content: []byte(`"hello"`), CreatedAt: newTS()},
	}

	// User 2 has read up to message 20 (both messages), User 3 only up to message 10.
	store.ReadStates[convID] = map[int64]orm.ConversationReadState{
		2: {ConversationID: convID, UserID: 2, LastReadMessageID: 20, UpdatedAt: newTS()},
		3: {ConversationID: convID, UserID: 3, LastReadMessageID: 10, UpdatedAt: newTS()},
	}

	convSvc := service.NewConversationService(store, testSnowflake, nil, nil)
	svcCtx := &svc.ServiceContext{
		ConversationService: convSvc,
		UserInfoService: &fakeUserInfoService{users: map[int64]orm.UserInfo{
			1: {ID: 1, Email: "u1@x.com", Nickname: "U1"},
			2: {ID: 2, Email: "u2@x.com", Nickname: "U2"},
			3: {ID: 3, Email: "u3@x.com", Nickname: "U3"},
		}},
	}

	l := NewGetConversationHistoryLogic(context.Background(), svcCtx)
	resp, err := l.GetConversationHistory(&pb.GetConversationHistoryReq{ConversationId: convID, Limit: 50})
	require.NoError(t, err)
	require.Len(t, resp.GetMessages(), 2)

	// Message 10 (sender=1): both 2 and 3 are non-senders.
	// 2 has last=20 >= 10 → is_read=true. 3 has last=10 >= 10 → is_read=true.
	msg10 := resp.GetMessages()[0]
	require.Len(t, msg10.GetReadDetails(), 2, "msg 10 should have 2 non-sender read details")
	for _, rd := range msg10.GetReadDetails() {
		assert.True(t, rd.GetIsRead(), "both members should have read msg 10")
	}

	// Message 20 (sender=2): non-senders are 1 and 3.
	// 1 has no read state → default is_read=false. 3 has last=10 < 20 → is_read=false.
	msg20 := resp.GetMessages()[1]
	require.Len(t, msg20.GetReadDetails(), 2, "msg 20 should have 2 non-sender read details")
	for _, rd := range msg20.GetReadDetails() {
		assert.False(t, rd.GetIsRead(), "neither member should have read msg 20")
	}

	// Verify read_states global cursor is still returned.
	require.Len(t, resp.GetReadStates(), 2)
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
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 1, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 2, JoinedAt: newTS()},
	}

	svcCtx := &svc.ServiceContext{ConversationService: service.NewConversationService(store, testSnowflake, nil, nil)}
	resp, err := NewGetConversationMembersLogic(context.Background(), svcCtx).GetConversationMembers(&pb.GetConversationMembersReq{ConversationId: convID})
	require.NoError(t, err)
	require.Equal(t, convID, resp.GetConversationId())
	require.Equal(t, []int64{1, 2}, resp.GetMemberIds())
}

func TestGetConversationMembersLogic_InvalidRequest(t *testing.T) {
	_, err := NewGetConversationMembersLogic(context.Background(), setupTestSvc()).GetConversationMembers(&pb.GetConversationMembersReq{})
	require.Error(t, err)
}

func TestUpdateReadReceiptLogic_HappyPath(t *testing.T) {
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 11, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 22, JoinedAt: newTS()},
	}

	svcCtx := &svc.ServiceContext{ConversationService: service.NewConversationService(store, testSnowflake, nil, nil)}

	logic := NewUpdateReadReceiptLogic(context.Background(), svcCtx)
	resp, err := logic.UpdateReadReceipt(&pb.UpdateReadReceiptReq{
		ConversationId:    convID,
		UserId:            11,
		LastReadMessageId: 500,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetReadState())
	require.Equal(t, int64(11), resp.GetReadState().GetUserId())
	require.Equal(t, int64(500), resp.GetReadState().GetLastReadMessageId())
}

func TestUpdateReadReceiptLogic_NotMemberRejected(t *testing.T) {
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 11, JoinedAt: newTS()},
	}

	svcCtx := &svc.ServiceContext{ConversationService: service.NewConversationService(store, testSnowflake, nil, nil)}

	logic := NewUpdateReadReceiptLogic(context.Background(), svcCtx)
	_, err := logic.UpdateReadReceipt(&pb.UpdateReadReceiptReq{
		ConversationId:    convID,
		UserId:            999,
		LastReadMessageId: 500,
	})
	require.Error(t, err)
}

func TestListConversationReadStatesLogic_HappyPath(t *testing.T) {
	store := servicefakes.NewConversationStore()
	convID := generateTestConversationID()
	store.Conversations[convID] = orm.Conversation{
		ID:               convID,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        newTS(),
	}
	store.Members[convID] = []orm.ConversationMember{
		{ConversationID: convID, UserID: 11, JoinedAt: newTS()},
		{ConversationID: convID, UserID: 22, JoinedAt: newTS()},
	}

	svcCtx := &svc.ServiceContext{ConversationService: service.NewConversationService(store, testSnowflake, nil, nil)}

	// Seed two cursors.
	_, err := NewUpdateReadReceiptLogic(context.Background(), svcCtx).UpdateReadReceipt(&pb.UpdateReadReceiptReq{
		ConversationId: convID, UserId: 11, LastReadMessageId: 500,
	})
	require.NoError(t, err)
	_, err = NewUpdateReadReceiptLogic(context.Background(), svcCtx).UpdateReadReceipt(&pb.UpdateReadReceiptReq{
		ConversationId: convID, UserId: 22, LastReadMessageId: 700,
	})
	require.NoError(t, err)

	logic := NewListConversationReadStatesLogic(context.Background(), svcCtx)
	resp, err := logic.ListConversationReadStates(&pb.ListConversationReadStatesReq{ConversationId: convID})
	require.NoError(t, err)
	require.Len(t, resp.GetReadStates(), 2)
}
