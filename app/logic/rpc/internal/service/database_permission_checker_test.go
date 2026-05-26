package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fake querier ---

type fakeQuerier struct {
	conversations    map[int64]model.GetConversationRow
	members          map[string]model.GetMemberRow // key: "convID:userID"
	friendships      map[string][]model.GetFriendshipBidirectionalRow
	messageCounts    map[int64]int64
	userTypes        map[int64]string
	getConvErr       error
	getMemberErr     error
	getFriendshipErr error
}

func (f *fakeQuerier) GetConversation(ctx context.Context, id int64) (model.GetConversationRow, error) {
	if f.getConvErr != nil {
		return model.GetConversationRow{}, f.getConvErr
	}

	conv, ok := f.conversations[id]
	if !ok {
		return model.GetConversationRow{}, pgx.ErrNoRows
	}

	return conv, nil
}

func (f *fakeQuerier) GetMember(ctx context.Context, arg model.GetMemberParams) (model.GetMemberRow, error) {
	if f.getMemberErr != nil {
		return model.GetMemberRow{}, f.getMemberErr
	}

	key := convUserKey(arg.ConversationID, arg.UserID)

	member, ok := f.members[key]
	if !ok {
		return model.GetMemberRow{}, pgx.ErrNoRows
	}

	return member, nil
}

func (f *fakeQuerier) GetFriendshipBidirectional(ctx context.Context, arg model.GetFriendshipBidirectionalParams) ([]model.GetFriendshipBidirectionalRow, error) {
	if f.getFriendshipErr != nil {
		return nil, f.getFriendshipErr
	}

	key := friendshipKey(arg.UserID, arg.FriendID)

	return f.friendships[key], nil
}

// Unused methods - implement to satisfy interface
func (f *fakeQuerier) GetFriendship(ctx context.Context, arg model.GetFriendshipParams) (model.GetFriendshipRow, error) {
	return model.GetFriendshipRow{}, nil
}

func (f *fakeQuerier) GetFriendshipByPair(ctx context.Context, arg model.GetFriendshipByPairParams) (model.Friendship, error) {
	return model.Friendship{}, nil
}

func (f *fakeQuerier) UpsertFriendship(ctx context.Context, arg model.UpsertFriendshipParams) (model.Friendship, error) {
	return model.Friendship{}, nil
}

func (f *fakeQuerier) IsMemberMuted(ctx context.Context, arg model.IsMemberMutedParams) (model.IsMemberMutedRow, error) {
	return model.IsMemberMutedRow{}, nil
}

func (f *fakeQuerier) InsertMessage(ctx context.Context, arg model.InsertMessageParams) error {
	return nil
}

func (f *fakeQuerier) CreateUserInfo(ctx context.Context, arg model.CreateUserInfoParams) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) UpdateUserInfoProfile(ctx context.Context, arg model.UpdateUserInfoProfileParams) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) UpdateUserInfoStatus(ctx context.Context, arg model.UpdateUserInfoStatusParams) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeQuerier) SearchUserInfoByNickname(ctx context.Context, arg model.SearchUserInfoByNicknameParams) ([]model.UserInfo, error) {
	return nil, nil
}

func (f *fakeQuerier) CreateConversation(ctx context.Context, arg model.CreateConversationParams) (model.CreateConversationRow, error) {
	return model.CreateConversationRow{}, nil
}

func (f *fakeQuerier) AddConversationMembers(ctx context.Context, arg model.AddConversationMembersParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) GetConversationMembers(ctx context.Context, conversationID int64) ([]model.GetConversationMembersRow, error) {
	rows := make([]model.GetConversationMembersRow, 0)

	for _, member := range f.members {
		if member.ConversationID != conversationID {
			continue
		}

		rows = append(rows, model.GetConversationMembersRow{
			ConversationID: member.ConversationID,
			UserID:         member.UserID,
			IsMuted:        member.IsMuted,
			MutedUntil:     member.MutedUntil,
		})
	}

	return rows, nil
}

func (f *fakeQuerier) GetConversationsByUserID(ctx context.Context, userID int64) ([]model.GetConversationsByUserIDRow, error) {
	return nil, nil
}

func (f *fakeQuerier) ListMessagesByConversation(ctx context.Context, arg model.ListMessagesByConversationParams) ([]model.Message, error) {
	return nil, nil
}

func (f *fakeQuerier) ListMessagesByConversationInitial(ctx context.Context, arg model.ListMessagesByConversationInitialParams) ([]model.Message, error) {
	return nil, nil
}

func (f *fakeQuerier) ListPendingFriendApplications(ctx context.Context, friendID int64) ([]model.Friendship, error) {
	return nil, nil
}

func (f *fakeQuerier) ListFriends(ctx context.Context, userID int64) ([]model.Friendship, error) {
	return nil, nil
}

func (f *fakeQuerier) CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error) {
	return f.messageCounts[conversationID], nil
}

func (f *fakeQuerier) AddConversationMemberWithRole(ctx context.Context, arg model.AddConversationMemberWithRoleParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) RemoveConversationMembers(ctx context.Context, arg model.RemoveConversationMembersParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) UpdateConversation(ctx context.Context, arg model.UpdateConversationParams) error {
	return nil
}

func (f *fakeQuerier) UpdateConversationCreator(ctx context.Context, arg model.UpdateConversationCreatorParams) error {
	return nil
}

func (f *fakeQuerier) UpdateConversationMemberRole(ctx context.Context, arg model.UpdateConversationMemberRoleParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) DeactivateConversation(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeQuerier) GetConversationCreator(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) IsConversationMember(ctx context.Context, arg model.IsConversationMemberParams) (bool, error) {
	return false, nil
}

func (f *fakeQuerier) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]model.GetConversationMembersDetailRow, error) {
	return nil, nil
}

func (f *fakeQuerier) UpsertConversationReadState(ctx context.Context, arg model.UpsertConversationReadStateParams) (model.ConversationReadState, error) {
	return model.ConversationReadState{}, nil
}

func (f *fakeQuerier) GetConversationReadState(ctx context.Context, arg model.GetConversationReadStateParams) (model.ConversationReadState, error) {
	return model.ConversationReadState{}, nil
}

func (f *fakeQuerier) ListConversationReadStates(ctx context.Context, conversationID int64) ([]model.ConversationReadState, error) {
	return nil, nil
}

func (f *fakeQuerier) GetUserType(ctx context.Context, id int64) (string, error) {
	if userType, ok := f.userTypes[id]; ok {
		return userType, nil
	}

	return "human", nil
}

func (f *fakeQuerier) UpdateUserInfoType(ctx context.Context, arg model.UpdateUserInfoTypeParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) CreateBotToken(ctx context.Context, arg model.CreateBotTokenParams) (model.BotToken, error) {
	return model.BotToken{}, nil
}

func (f *fakeQuerier) GetBotTokenByHash(ctx context.Context, tokenHash string) (model.GetBotTokenByHashRow, error) {
	return model.GetBotTokenByHashRow{}, pgx.ErrNoRows
}

func (f *fakeQuerier) ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error) {
	return nil, nil
}

func (f *fakeQuerier) GetBotActionByName(ctx context.Context, action string) (model.BotAction, error) {
	return model.BotAction{}, pgx.ErrNoRows
}

func (f *fakeQuerier) GrantBotTokenAction(ctx context.Context, arg model.GrantBotTokenActionParams) error {
	return nil
}

func (f *fakeQuerier) GetEnabledActionByWebhookEvent(ctx context.Context, event string) (string, error) {
	return "", pgx.ErrNoRows
}

func (f *fakeQuerier) ListBotTokensByBot(ctx context.Context, botUserID int64) ([]model.BotToken, error) {
	return nil, nil
}

func (f *fakeQuerier) RevokeBotToken(ctx context.Context, arg model.RevokeBotTokenParams) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) UpsertBotWebhook(ctx context.Context, arg model.UpsertBotWebhookParams) (model.BotWebhook, error) {
	return model.BotWebhook{}, nil
}

func (f *fakeQuerier) GetBotWebhook(ctx context.Context, botUserID int64) (model.BotWebhook, error) {
	return model.BotWebhook{}, pgx.ErrNoRows
}

func (f *fakeQuerier) DeleteBotWebhook(ctx context.Context, botUserID int64) (int64, error) {
	return 0, nil
}

func (f *fakeQuerier) ListActiveBotWebhooksForConversation(ctx context.Context, conversationID int64) ([]model.BotWebhook, error) {
	return nil, nil
}

func (f *fakeQuerier) GetDirectConversationByMembers(ctx context.Context, arg model.GetDirectConversationByMembersParams) (model.GetDirectConversationByMembersRow, error) {
	if f.getConvErr != nil {
		return model.GetDirectConversationByMembersRow{}, f.getConvErr
	}

	for _, conv := range f.conversations {
		if conv.ConversationType != "direct" || !conv.IsActive {
			continue
		}

		key1 := convUserKey(conv.ID, arg.UserID)
		key2 := convUserKey(conv.ID, arg.UserID_2)
		_, ok1 := f.members[key1]
		_, ok2 := f.members[key2]

		if ok1 && ok2 {
			return model.GetDirectConversationByMembersRow(conv), nil
		}
	}

	return model.GetDirectConversationByMembersRow{}, pgx.ErrNoRows
}

func convUserKey(convID, userID int64) string {
	return fmt.Sprintf("%d:%d", convID, userID)
}

func friendshipKey(userID, friendID int64) string {
	return fmt.Sprintf("%d:%d", userID, friendID)
}

func memberMap(members ...model.GetMemberRow) map[string]model.GetMemberRow {
	result := make(map[string]model.GetMemberRow, len(members))
	for _, member := range members {
		result[convUserKey(member.ConversationID, member.UserID)] = member
	}

	return result
}

func directMembers(conversationID, userID, peerID int64) map[string]model.GetMemberRow {
	return memberMap(
		model.GetMemberRow{ConversationID: conversationID, UserID: userID},
		model.GetMemberRow{ConversationID: conversationID, UserID: peerID},
	)
}

func newPastTime() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(-1 * time.Hour), Valid: true}
}

func newFutureTime() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(1 * time.Hour), Valid: true}
}

// --- Test cases ---

func TestDatabasePermissionChecker_GroupMember(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
}

func TestDatabasePermissionChecker_GroupNotMember(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members:     map[string]model.GetMemberRow{},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "not a member")
}

func TestDatabasePermissionChecker_GroupMuted(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: pgtype.Timestamptz{Valid: false}}, // permanent mute
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "muted")
}

func TestDatabasePermissionChecker_GroupMuteExpired(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: newPastTime()},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
}

func TestDatabasePermissionChecker_GroupMuteNotExpired(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: newFutureTime()},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "muted")
}

func TestDatabasePermissionChecker_DirectFriend(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "accepted"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
}

func TestDatabasePermissionChecker_DirectFriendUsesPeerMemberNotConversationID(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			300: {ID: 300, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(300, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "accepted"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 300,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
}

func TestDatabasePermissionChecker_DirectNotFriendUsesTemporaryConversation(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "pending"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
	assert.Contains(t, decision.Reason, "temporary conversation")
}

func TestDatabasePermissionChecker_DirectTemporaryConversationLimitReached(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "pending"},
			},
		},
		messageCounts: map[int64]int64{200: temporaryConversationMessageLimit},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "temporary conversation message limit reached")
}

func TestDatabasePermissionChecker_DirectBotConversationSkipsTemporaryLimit(t *testing.T) {
	tests := []struct {
		name     string
		senderID int64
		peerID   int64
	}{
		{
			name:     "human sends to bot",
			senderID: 100,
			peerID:   9001,
		},
		{
			name:     "bot sends to human",
			senderID: 9001,
			peerID:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fq := &fakeQuerier{
				conversations: map[int64]model.GetConversationRow{
					200: {ID: 200, ConversationType: "direct", IsActive: true},
				},
				members: directMembers(200, tt.senderID, tt.peerID),
				friendships: map[string][]model.GetFriendshipBidirectionalRow{
					friendshipKey(tt.senderID, tt.peerID): {
						{UserID: tt.senderID, FriendID: tt.peerID, Status: "pending"},
					},
				},
				messageCounts: map[int64]int64{200: temporaryConversationMessageLimit},
				userTypes:     map[int64]string{9001: UserTypeBot},
			}
			checker := NewDatabasePermissionChecker(fq)

			decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
				SenderID:       tt.senderID,
				ConversationID: 200,
			})

			require.NoError(t, err)
			assert.True(t, decision.Allowed)
			assert.Equal(t, CodeOK, decision.Code)
			assert.Equal(t, "bot direct conversation", decision.Reason)
		})
	}
}

func TestDatabasePermissionChecker_DirectTemporaryConversationCustomLimit(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "pending"},
			},
		},
		messageCounts: map[int64]int64{200: 50},
	}

	// limit=100 时汇总 50 条仍在限额内，应放行
	checker := NewDatabasePermissionCheckerWithLimit(fq, 100)
	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID: 100, ConversationID: 200,
	})
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)

	// limit=0 表示不限制（压测场景）
	checkerUnlimited := NewDatabasePermissionCheckerWithLimit(fq, 0)
	decision, err = checkerUnlimited.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID: 100, ConversationID: 200,
	})
	require.NoError(t, err)
	assert.True(t, decision.Allowed)

	// limit=5 且计数 50，应拒绝
	checkerStrict := NewDatabasePermissionCheckerWithLimit(fq, 5)
	decision, err = checkerStrict.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID: 100, ConversationID: 200,
	})
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
}

func TestDatabasePermissionChecker_DirectBlocked(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "blocked"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "blocked")
}

func TestDatabasePermissionChecker_ConversationNotFound(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{},
		members:       map[string]model.GetMemberRow{},
		friendships:   map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 999,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodeNotFound, decision.Code)
	assert.Contains(t, decision.Reason, "not found")
}

func TestDatabasePermissionChecker_ConversationInactive(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: false},
		},
		members:     map[string]model.GetMemberRow{},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodeNotFound, decision.Code)
	assert.Contains(t, decision.Reason, "not active")
}

func TestDatabasePermissionChecker_DBError(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{},
		members:       map[string]model.GetMemberRow{},
		friendships:   map[string][]model.GetFriendshipBidirectionalRow{},
		getConvErr:    errors.New("database connection error"),
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
	assert.False(t, decision.Allowed)
}

func TestDatabasePermissionChecker_DirectBidirectionalFriendship(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "accepted"},
				{UserID: 200, FriendID: 100, Status: "accepted"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
}

func TestDatabasePermissionChecker_GroupMentionsFilteredToMembers(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
			convUserKey(1, 200): {ConversationID: 1, UserID: 200, IsMuted: false},
			convUserKey(1, 300): {ConversationID: 1, UserID: 300, IsMuted: false},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	// Mention member 200, member 300, and non-member 999
	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
		Mentions:       []int64{200, 300, 999},
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
	// Only member IDs 200 and 300 should remain; 999 is filtered out
	assert.Equal(t, []int64{200, 300}, decision.FilteredMentions)
}

func TestDatabasePermissionChecker_GroupAllMentionsNonMember(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	// Mention only non-members
	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
		Mentions:       []int64{888, 999},
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, CodeOK, decision.Code)
	// All mentions filtered out — message still allowed
	assert.Empty(t, decision.FilteredMentions)
}

func TestDatabasePermissionChecker_GroupNoMentions(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]model.GetMemberRow{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]model.GetFriendshipBidirectionalRow{},
	}
	checker := NewDatabasePermissionChecker(fq)

	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 1,
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Nil(t, decision.FilteredMentions)
}

func TestDatabasePermissionChecker_DirectMentionsFilteredToMembers(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]model.GetFriendshipBidirectionalRow{
			friendshipKey(100, 200): {
				{UserID: 100, FriendID: 200, Status: "accepted"},
			},
		},
	}
	checker := NewDatabasePermissionChecker(fq)

	// In a direct conversation, only the peer (200) is a valid mention; 999 is not a member
	decision, err := checker.CheckMessagePermission(context.Background(), PermissionCheck{
		SenderID:       100,
		ConversationID: 200,
		Mentions:       []int64{200, 999},
	})

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Equal(t, []int64{200}, decision.FilteredMentions)
}

func TestFilterMentionsByMemberIDs(t *testing.T) {
	t.Parallel()

	assert.Nil(t, filterMentionsByMemberIDs(nil, nil))
	assert.Empty(t, filterMentionsByMemberIDs([]int64{1, 2}, nil))
	assert.Nil(t, filterMentionsByMemberIDs(nil, []int64{1, 2}))

	// All members
	assert.Equal(t, []int64{1, 2}, filterMentionsByMemberIDs([]int64{1, 2}, []int64{1, 2, 3}))

	// Partial members
	assert.Equal(t, []int64{2}, filterMentionsByMemberIDs([]int64{1, 2, 4}, []int64{2, 3}))

	// No overlap
	assert.Empty(t, filterMentionsByMemberIDs([]int64{5, 6}, []int64{1, 2}))
}
