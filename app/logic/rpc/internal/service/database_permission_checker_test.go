package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func convUserKey(convID, userID int64) string {
	return fmt.Sprintf("%d:%d", convID, userID)
}

func friendshipKey(userID, friendID int64) string {
	return fmt.Sprintf("%d:%d", userID, friendID)
}

func memberMap(members ...orm.ConversationMember) map[string]orm.ConversationMember {
	result := make(map[string]orm.ConversationMember, len(members))
	for _, member := range members {
		result[convUserKey(member.ConversationID, member.UserID)] = member
	}

	return result
}

func directMembers(conversationID, userID, peerID int64) map[string]orm.ConversationMember {
	return memberMap(
		orm.ConversationMember{ConversationID: conversationID, UserID: userID},
		orm.ConversationMember{ConversationID: conversationID, UserID: peerID},
	)
}

func newPastTime() *time.Time {
	t := time.Now().Add(-time.Hour)
	return &t
}

func newFutureTime() *time.Time {
	t := time.Now().Add(time.Hour)
	return &t
}

// --- Test cases ---

func TestDatabasePermissionChecker_GroupMember(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members:     map[string]orm.ConversationMember{},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: nil}, // permanent mute
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: newPastTime()},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: true, MutedUntil: newFutureTime()},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			300: {ID: 300, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(300, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
				conversations: map[int64]orm.Conversation{
					200: {ID: 200, ConversationType: "direct", IsActive: true},
				},
				members: directMembers(200, tt.senderID, tt.peerID),
				friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{},
		members:       map[string]orm.ConversationMember{},
		friendships:   map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: false},
		},
		members:     map[string]orm.ConversationMember{},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{},
		members:       map[string]orm.ConversationMember{},
		friendships:   map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
			convUserKey(1, 200): {ConversationID: 1, UserID: 200, IsMuted: false},
			convUserKey(1, 300): {ConversationID: 1, UserID: 300, IsMuted: false},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			1: {ID: 1, ConversationType: "group", IsActive: true},
		},
		members: map[string]orm.ConversationMember{
			convUserKey(1, 100): {ConversationID: 1, UserID: 100, IsMuted: false},
		},
		friendships: map[string][]orm.Friendship{},
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
		conversations: map[int64]orm.Conversation{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: directMembers(200, 100, 200),
		friendships: map[string][]orm.Friendship{
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
