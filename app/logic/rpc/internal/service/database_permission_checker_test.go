package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fake querier ---

type fakeQuerier struct {
	conversations        map[int64]model.GetConversationRow
	members             map[string]model.GetMemberRow // key: "convID:userID"
	friendships         map[string][]model.GetFriendshipBidirectionalRow
	getConvErr         error
	getMemberErr       error
	getFriendshipErr   error
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

func (f *fakeQuerier) IsMemberMuted(ctx context.Context, arg model.IsMemberMutedParams) (model.IsMemberMutedRow, error) {
	return model.IsMemberMutedRow{}, nil
}

func (f *fakeQuerier) InsertMessage(ctx context.Context, arg model.InsertMessageParams) error {
	return nil
}

func convUserKey(convID, userID int64) string {
	return fmt.Sprintf("%d:%d", convID, userID)
}

func friendshipKey(userID, friendID int64) string {
	return fmt.Sprintf("%d:%d", userID, friendID)
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
		members:       map[string]model.GetMemberRow{},
		friendships:   map[string][]model.GetFriendshipBidirectionalRow{},
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
		members: map[string]model.GetMemberRow{},
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

func TestDatabasePermissionChecker_DirectNotFriend(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: map[string]model.GetMemberRow{},
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
	assert.False(t, decision.Allowed)
	assert.Equal(t, CodePermissionDenied, decision.Code)
	assert.Contains(t, decision.Reason, "not friends")
}

func TestDatabasePermissionChecker_DirectBlocked(t *testing.T) {
	fq := &fakeQuerier{
		conversations: map[int64]model.GetConversationRow{
			200: {ID: 200, ConversationType: "direct", IsActive: true},
		},
		members: map[string]model.GetMemberRow{},
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
		members:       map[string]model.GetMemberRow{},
		friendships:   map[string][]model.GetFriendshipBidirectionalRow{},
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
		getConvErr:   errors.New("database connection error"),
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
		members: map[string]model.GetMemberRow{},
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

