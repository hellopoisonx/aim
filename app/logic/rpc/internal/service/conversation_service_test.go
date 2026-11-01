package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/servicefakes"

	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"

	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSnowflake is a shared Snowflake instance for tests.
var testSnowflake = func() *tools.Snowflake {
	s, err := tools.NewSnowflake(1)
	if err != nil {
		panic(err)
	}

	return s
}()

// ---------------------------------------------------------------------------
// CreateConversation tests
// ---------------------------------------------------------------------------

func TestConversationService_CreateConversation_ValidDirect(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// direct Conversations: member_ids must contain exactly the peer (creator is auto-added).
	conv, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)
	assert.Equal(t, sharedconversation.TypeDirect, conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify members were added (creator auto-added on top of peer)
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_CreateConversation_ValidGroup(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	conv, err := svc.CreateConversation(context.Background(), sharedconversation.TypeGroup, 1, []int64{1, 2, 3, 4}, "Group Chat", "")
	require.NoError(t, err)
	assert.Equal(t, sharedconversation.TypeGroup, conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify all members were added
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 4)
	var foundOwner bool
	for _, m := range members {
		if m.UserID == 1 {
			assert.Equal(t, sharedconversation.RoleOwner, m.Role)
			foundOwner = true
		}
	}
	assert.True(t, foundOwner)
}

func TestConversationService_CreateConversation_InvalidType(t *testing.T) {
	tests := []struct {
		name             string
		conversationType string
	}{
		{name: "empty_string", conversationType: ""},
		{name: "random_type", conversationType: "channel"},
		{name: "uppercase_direct", conversationType: "Direct"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := servicefakes.NewConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), sharedconversation.Type(tt.conversationType), 1, []int64{1, 2}, "conv", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "conversation_type must be 'direct' or 'group'")
		})
	}
}

func TestConversationService_CreateConversation_MissingGroupName(t *testing.T) {
	tests := []struct {
		name        string
		convName    string
		errContains string
	}{
		{name: "empty_name_group", convName: "", errContains: "name is required"},
		{name: "whitespace_name_group", convName: "   ", errContains: "name is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := servicefakes.NewConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2}, tt.convName, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestConversationService_CreateConversation_DirectAllowsEmptyName(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Direct conversation names are now computed by the logic RPC layer before calling the service.
	// The service layer accepts whatever name is passed, including empty.
	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "", "")
	require.NoError(t, err)
	assert.Empty(t, conv.Name)
}

func TestConversationService_CreateConversation_EmptyMembers(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{}, "Group", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "member_ids must not be empty")
}

func TestConversationService_CreateConversation_DirectInvalidMemberCount(t *testing.T) {
	tests := []struct {
		name        string
		memberIDs   []int64
		errContains string
	}{
		{
			name:        "only_creator",
			memberIDs:   []int64{1},
			errContains: "direct conversation must have exactly 2 members",
		},
		{
			name:        "two_peers",
			memberIDs:   []int64{2, 3},
			errContains: "direct conversation member_ids must contain exactly one peer user id",
		},
		{
			name:        "three_members",
			memberIDs:   []int64{1, 2, 3},
			errContains: "direct conversation member_ids must contain exactly one peer user id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := servicefakes.NewConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), "direct", 1, tt.memberIDs, "direct", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestConversationService_CreateConversation_DirectWithCreatorNotInMembers(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Creator (1) is not in the member list; the service should auto-add it.
	conv, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Creator should be automatically added
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)

	// Verify creator is in members
	var hasCreator bool
	for _, m := range members {
		if m.UserID == 1 {
			hasCreator = true
			break
		}
	}
	assert.True(t, hasCreator, "creator should be auto-added to members")
	var foundOwner bool
	for _, m := range members {
		if m.UserID == 1 {
			assert.Equal(t, sharedconversation.RoleOwner, m.Role)
			foundOwner = true
		}
	}
	assert.True(t, foundOwner)
}

func TestConversationService_RoleGuards(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.NoError(t, err)

	_, err = svc.AddGroupMembers(context.Background(), conv.ID, 2, "u2", []int64{4})
	require.ErrorIs(t, err, ErrNotAdmin)

	err = svc.RemoveGroupMembers(context.Background(), conv.ID, 2, "u2", []int64{3})
	require.ErrorIs(t, err, ErrNotAdmin)

	name := "n"
	_, err = svc.UpdateGroupInfo(context.Background(), conv.ID, 2, "u2", &name, nil)
	require.ErrorIs(t, err, ErrNotAdmin)

	err = svc.DismissGroup(context.Background(), conv.ID, 2)
	require.ErrorIs(t, err, ErrNotOwner)

	err = svc.LeaveGroup(context.Background(), conv.ID, 1, "u1")
	require.ErrorIs(t, err, ErrNotOwner)
}

func testMemberRole(t *testing.T, store *servicefakes.ConversationStore, conversationID, userID int64) string {
	t.Helper()
	for _, m := range store.Members[conversationID] {
		if m.UserID == userID {
			return string(m.Role)
		}
	}
	t.Fatalf("member %d not found in conversation %d", userID, conversationID)
	return ""
}

func TestConversationService_GrantAndRevokeGroupAdmin(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.NoError(t, err)

	err = svc.GrantGroupAdmin(context.Background(), conv.ID, 2, 3)
	require.ErrorIs(t, err, ErrNotOwner)

	err = svc.GrantGroupAdmin(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "admin", testMemberRole(t, store, conv.ID, 2))

	// Idempotent for an already-admin target.
	err = svc.GrantGroupAdmin(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "admin", testMemberRole(t, store, conv.ID, 2))

	err = svc.RevokeGroupAdmin(context.Background(), conv.ID, 2, 3)
	require.ErrorIs(t, err, ErrNotOwner)

	err = svc.RevokeGroupAdmin(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "member", testMemberRole(t, store, conv.ID, 2))

	// Idempotent for an already-member target.
	err = svc.RevokeGroupAdmin(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "member", testMemberRole(t, store, conv.ID, 2))
}

func TestConversationService_AdminRemovePermissionMatrix(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3, 4}, "Group", "")
	require.NoError(t, err)
	require.NoError(t, svc.GrantGroupAdmin(context.Background(), conv.ID, 1, 2))
	require.NoError(t, svc.GrantGroupAdmin(context.Background(), conv.ID, 1, 3))

	err = svc.RemoveGroupMembers(context.Background(), conv.ID, 2, "admin", []int64{1})
	require.ErrorIs(t, err, ErrNotOwner)

	err = svc.RemoveGroupMembers(context.Background(), conv.ID, 2, "admin", []int64{3})
	require.ErrorIs(t, err, ErrNotAdmin)

	err = svc.RemoveGroupMembers(context.Background(), conv.ID, 2, "admin", []int64{2})
	require.ErrorIs(t, err, ErrNotOwner)
}

func TestConversationService_TransferGroupOwner(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.NoError(t, err)

	_, err = svc.TransferGroupOwner(context.Background(), conv.ID, 2, 3)
	require.ErrorIs(t, err, ErrNotOwner)

	updated, err := svc.TransferGroupOwner(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.CreatorID)
	assert.Equal(t, int64(2), store.Conversations[conv.ID].CreatorID)
	assert.Equal(t, "admin", testMemberRole(t, store, conv.ID, 1))
	assert.Equal(t, "owner", testMemberRole(t, store, conv.ID, 2))

	// Old owner is now admin, not owner, and cannot transfer again.
	_, err = svc.TransferGroupOwner(context.Background(), conv.ID, 1, 3)
	require.ErrorIs(t, err, ErrNotOwner)
}

func TestConversationService_OwnerOnlyRoleTargets(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2}, "Group", "")
	require.NoError(t, err)

	err = svc.GrantGroupAdmin(context.Background(), conv.ID, 1, 1)
	require.Error(t, err)

	err = svc.RevokeGroupAdmin(context.Background(), conv.ID, 1, 1)
	require.Error(t, err)

	_, err = svc.TransferGroupOwner(context.Background(), conv.ID, 1, 1)
	require.Error(t, err)
}

func TestConversationService_CreateConversation_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.CreateErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2}, "Group", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

func TestConversationService_CreateConversation_AddMemberError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.AddMemberErr = errors.New("failed to add member")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add member")
}

func TestConversationService_CreateConversation_DirectDedup_Existing(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// First call: create a direct conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Second call: same two users should return the existing conversation
	conv2, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Should return the SAME conversation ID
	assert.Equal(t, conv1.ID, conv2.ID)
	assert.Equal(t, sharedconversation.TypeDirect, conv2.ConversationType)
	assert.True(t, conv2.IsActive)
}

func TestConversationService_CreateConversation_DirectDedup_NewMembers(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Create conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Create conversation between users 3 and 4 (different pair)
	conv2, err := svc.CreateConversation(context.Background(), "direct", 3, []int64{4}, "direct-3-4", "")
	require.NoError(t, err)

	// Should be DIFFERENT conversation IDs
	assert.NotEqual(t, conv1.ID, conv2.ID)
}

func TestConversationService_CreateConversation_DirectDedup_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.GetErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), sharedconversation.TypeDirect, 1, []int64{2}, "direct-1-2", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

// ---------------------------------------------------------------------------
// GetConversationByID tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationByID_Found(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[100] = orm.Conversation{
		ID:               100,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	conv, err := svc.GetConversationByID(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(100), conv.ID)
	assert.Equal(t, sharedconversation.TypeDirect, conv.ConversationType)
}

func TestConversationService_GetConversationByID_NotFound(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationByID(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationByID_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.GetErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationByID(context.Background(), 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

// ---------------------------------------------------------------------------
// GetConversationHistory tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationHistory_WithCursor(t *testing.T) {
	store := servicefakes.NewConversationStore()
	// Create conversation
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	// Add messages
	now := time.Now()
	store.Messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg1"), CreatedAt: now},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("msg2"), CreatedAt: now.Add(time.Second)},
		{ID: 3, ConversationID: 1, SenderID: 1, Content: []byte("msg3"), CreatedAt: now.Add(2 * time.Second)},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Get messages with cursor at message ID 2
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 2, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, int64(1), messages[0].ID)
}

func TestConversationService_GetConversationHistory_InitialPage(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.Messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("first"), CreatedAt: now},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("second"), CreatedAt: now.Add(time.Second)},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// When cursor is 0,0 it should call GetConversationHistoryInitial
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestConversationService_GetConversationHistory_ConversationNotFound(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistory(context.Background(), 999, 0, 1, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistory_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.ListMessagesErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistory(context.Background(), 1, 0, 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistory_LimitNormalization(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	// Create more than 100 messages
	now := time.Now()
	var msgs []orm.Message
	for i := int64(1); i <= 150; i++ {
		msgs = append(msgs, orm.Message{
			ID:             i,
			ConversationID: 1,
			SenderID:       1,
			Content:        []byte("msg"),
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
		})
	}
	store.Messages[1] = msgs
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// When limit > 100, it should be normalized to 50
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 200)
	require.NoError(t, err)
	assert.Len(t, messages, 50)
}

func TestConversationService_GetConversationHistory_ZeroLimit(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.Messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg"), CreatedAt: now},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// When limit <= 0, it should default to 50
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 0)
	require.NoError(t, err)
	assert.NotNil(t, messages)
}

// ---------------------------------------------------------------------------
// GetConversationHistoryInitial tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationHistoryInitial_Found(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.Messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("first"), CreatedAt: now},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("second"), CreatedAt: now.Add(time.Second)},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	messages, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestConversationService_GetConversationHistoryInitial_NotFound(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 999, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistoryInitial_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.ListMessagesInitErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistoryInitial_LimitNormalization(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	// Create more than 100 messages
	var msgs []orm.Message
	for i := int64(1); i <= 150; i++ {
		msgs = append(msgs, orm.Message{
			ID:             i,
			ConversationID: 1,
			SenderID:       1,
			Content:        []byte("msg"),
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
		})
	}
	store.Messages[1] = msgs
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// When limit > 100, it should be normalized to 50
	messages, err := svc.GetConversationHistoryInitial(context.Background(), 1, 200)
	require.NoError(t, err)
	assert.Len(t, messages, 50)
}

// ---------------------------------------------------------------------------
// GetConversationMembers tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationMembers_Success(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.Members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	members, err := svc.GetConversationMembers(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_GetConversationMembers_ConversationNotFound(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationMembers(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationMembers_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.GetMembersErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationMembers(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationMembers_EmptyMembers(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	// No members added
	svc := NewConversationService(store, testSnowflake, nil, nil)

	members, err := svc.GetConversationMembers(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, members)
}

// ---------------------------------------------------------------------------
// IsMember tests
// ---------------------------------------------------------------------------

func TestConversationService_IsMember_True(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.Members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	isMember, err := svc.IsMember(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.True(t, isMember)
}

func TestConversationService_IsMember_False(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.Members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	isMember, err := svc.IsMember(context.Background(), 1, 999)
	require.NoError(t, err)
	assert.False(t, isMember)
}

func TestConversationService_IsMember_ConversationNotFound(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.IsMember(context.Background(), 999, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_IsMember_StoreError(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.GetMembersErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.IsMember(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

// ---------------------------------------------------------------------------
// Domain error contract
// ---------------------------------------------------------------------------

// Domain sentinels must carry their biz code themselves: RPC handlers return
// them through errorx.ToGRPCError, which passes *CodeError values through
// untouched. If a sentinel lost its code, clients would see 50000 instead.
func TestConversationDomainErrorsCarryBizCodes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"not found", ErrConversationNotFound, CodeConversationNotFound},
		{"not member", ErrNotMember, CodeNotMember},
		{"exists", ErrConversationExists, CodeConversationExists},
		{"not group", ErrNotGroupConversation, CodeNotGroupConversation},
		{"not owner", ErrNotOwner, CodeNotOwner},
		{"not admin", ErrNotAdmin, CodeNotAdmin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var codeErr *errorx.CodeError
			require.ErrorAs(t, tt.err, &codeErr)
			assert.Equal(t, tt.wantCode, codeErr.Code)
			// Code-based matching keeps working for callers using errors.Is.
			assert.ErrorIs(t, tt.err, errorx.NewCodeError(tt.wantCode, "different message"))
		})
	}
}

// A validation error raised by the service must survive the RPC boundary with
// its own code instead of being flattened into 50000.
func TestConversationDomainErrorSurvivesRPCConversion(t *testing.T) {
	validation := errorx.NewCodeError(errorx.CodeBadInput, "id must be positive")
	var codeErr *errorx.CodeError
	require.ErrorAs(t, errorx.ToGRPCError(validation), &codeErr)
	assert.Equal(t, errorx.CodeBadInput, codeErr.Code)
	assert.Equal(t, "id must be positive", codeErr.Message)
}

// ---------------------------------------------------------------------------
// Table-driven tests for edge cases
// ---------------------------------------------------------------------------

func TestConversationService_CreateConversation_TableDriven(t *testing.T) {
	tests := []struct {
		name            string
		convType        string
		creatorID       int64
		memberIDs       []int64
		convName        string
		wantErr         bool
		errContains     string
		expectedMembers int
	}{
		{
			name:            "valid_direct_peer_only",
			convType:        "direct",
			creatorID:       1,
			memberIDs:       []int64{2},
			convName:        "direct-1-2",
			wantErr:         false,
			expectedMembers: 2,
		},
		{
			name:            "valid_group_multiple_members",
			convType:        "group",
			creatorID:       1,
			memberIDs:       []int64{1, 2, 3},
			convName:        "Group",
			wantErr:         false,
			expectedMembers: 3,
		},
		{
			name:        "invalid_type_empty",
			convType:    "",
			creatorID:   1,
			memberIDs:   []int64{1, 2},
			convName:    "conv",
			wantErr:     true,
			errContains: "conversation_type must be 'direct' or 'group'",
		},
		{
			name:        "invalid_type_random",
			convType:    "channel",
			creatorID:   1,
			memberIDs:   []int64{1, 2},
			convName:    "conv",
			wantErr:     true,
			errContains: "conversation_type must be 'direct' or 'group'",
		},
		{
			name:        "missing_name",
			convType:    "group",
			creatorID:   1,
			memberIDs:   []int64{1, 2},
			convName:    "",
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name:        "empty_members",
			convType:    "group",
			creatorID:   1,
			memberIDs:   []int64{},
			convName:    "Group",
			wantErr:     true,
			errContains: "member_ids must not be empty",
		},
		{
			name:        "direct_only_creator",
			convType:    "direct",
			creatorID:   1,
			memberIDs:   []int64{1},
			convName:    "direct",
			wantErr:     true,
			errContains: "direct conversation must have exactly 2 members",
		},
		{
			name:        "direct_three_members",
			convType:    "direct",
			creatorID:   1,
			memberIDs:   []int64{1, 2, 3},
			convName:    "direct",
			wantErr:     true,
			errContains: "direct conversation member_ids must contain exactly one peer user id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := servicefakes.NewConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			conv, err := svc.CreateConversation(context.Background(), sharedconversation.Type(tt.convType), tt.creatorID, tt.memberIDs, tt.convName, "")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, sharedconversation.Type(tt.convType), conv.ConversationType)

				members, err := store.GetConversationMembers(context.Background(), conv.ID)
				require.NoError(t, err)
				assert.Len(t, members, tt.expectedMembers)
			}
		})
	}
}

func TestConversationService_GetConversationHistory_TableDriven(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.Messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg1"), CreatedAt: now},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("msg2"), CreatedAt: now.Add(time.Second)},
		{ID: 3, ConversationID: 1, SenderID: 1, Content: []byte("msg3"), CreatedAt: now.Add(2 * time.Second)},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	tests := []struct {
		name            string
		conversationID  int64
		cursorCreatedAt int64
		cursorID        int64
		limit           int32
		wantErr         bool
		errContains     string
		expectedCount   int
	}{
		{
			name:            "initial_page",
			conversationID:  1,
			cursorCreatedAt: 0,
			cursorID:        0,
			limit:           10,
			wantErr:         false,
			expectedCount:   3,
		},
		{
			name:            "with_cursor",
			conversationID:  1,
			cursorCreatedAt: 0,
			cursorID:        2,
			limit:           10,
			wantErr:         false,
			expectedCount:   1,
		},
		{
			name:            "conversation_not_found",
			conversationID:  999,
			cursorCreatedAt: 0,
			cursorID:        0,
			limit:           10,
			wantErr:         true,
			errContains:     "conversation not found",
		},
		{
			name:            "limit_exceeds_max",
			conversationID:  1,
			cursorCreatedAt: 0,
			cursorID:        0,
			limit:           200,
			wantErr:         false,
			expectedCount:   3, // normalized to 50, but store only has 3 messages
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := svc.GetConversationHistory(context.Background(), tt.conversationID, tt.cursorCreatedAt, tt.cursorID, tt.limit)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Len(t, messages, tt.expectedCount)
			}
		})
	}
}

func TestConversationService_IsMember_TableDriven(t *testing.T) {
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.Members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	tests := []struct {
		name           string
		conversationID int64
		userID         int64
		expectedMember bool
		wantErr        bool
	}{
		{
			name:           "member_exists_first",
			conversationID: 1,
			userID:         1,
			expectedMember: true,
			wantErr:        false,
		},
		{
			name:           "member_exists_second",
			conversationID: 1,
			userID:         2,
			expectedMember: true,
			wantErr:        false,
		},
		{
			name:           "member_not_exists",
			conversationID: 1,
			userID:         999,
			expectedMember: false,
			wantErr:        false,
		},
		{
			name:           "conversation_not_found",
			conversationID: 999,
			userID:         1,
			expectedMember: false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMember, err := svc.IsMember(context.Background(), tt.conversationID, tt.userID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMember, isMember)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateReadReceipt / ListConversationReadStates tests
// ---------------------------------------------------------------------------

func newReadReceiptFixture(t *testing.T) (*ConversationService, *servicefakes.ConversationStore) {
	t.Helper()
	store := servicefakes.NewConversationStore()
	store.Conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.Members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 11, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 22, IsMuted: false, JoinedAt: time.Now()},
	}
	return NewConversationService(store, testSnowflake, nil, nil), store
}

func TestConversationService_UpdateReadReceipt_ValidUpserts(t *testing.T) {
	svc, store := newReadReceiptFixture(t)

	state, err := svc.UpdateReadReceipt(context.Background(), 1, 11, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(11), state.UserID)
	assert.Equal(t, int64(1000), state.LastReadMessageID)

	// Advance the cursor.
	state, err = svc.UpdateReadReceipt(context.Background(), 1, 11, 2000)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), state.LastReadMessageID)

	// Stale receipt is a no-op but still returns the latest stored value.
	state, err = svc.UpdateReadReceipt(context.Background(), 1, 11, 500)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), state.LastReadMessageID)

	// Different member tracked independently.
	state, err = svc.UpdateReadReceipt(context.Background(), 1, 22, 1500)
	require.NoError(t, err)
	assert.Equal(t, int64(22), state.UserID)
	assert.Equal(t, int64(1500), state.LastReadMessageID)

	// Verify list returns both cursors.
	states, err := svc.ListConversationReadStates(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, states, 2)

	byUser := make(map[int64]int64, len(states))
	for _, s := range states {
		byUser[s.UserID] = s.LastReadMessageID
	}
	assert.Equal(t, int64(2000), byUser[11])
	assert.Equal(t, int64(1500), byUser[22])

	_ = store
}

func TestConversationService_UpdateReadReceipt_ValidationErrors(t *testing.T) {
	svc, _ := newReadReceiptFixture(t)

	tests := []struct {
		name              string
		conversationID    int64
		userID            int64
		lastReadMessageID int64
		errContains       string
	}{
		{"zero conversation id", 0, 11, 100, "conversation_id is required"},
		{"zero user id", 1, 0, 100, "user_id is required"},
		{"zero last_read_message_id", 1, 11, 0, "last_read_message_id is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateReadReceipt(context.Background(), tt.conversationID, tt.userID, tt.lastReadMessageID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestConversationService_UpdateReadReceipt_NotMember(t *testing.T) {
	svc, _ := newReadReceiptFixture(t)

	_, err := svc.UpdateReadReceipt(context.Background(), 1, 99, 1000)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotMember)
}

func TestConversationService_UpdateReadReceipt_ConversationNotFound(t *testing.T) {
	svc, _ := newReadReceiptFixture(t)

	_, err := svc.UpdateReadReceipt(context.Background(), 999, 11, 1000)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_ListConversationReadStates_Empty(t *testing.T) {
	svc, _ := newReadReceiptFixture(t)

	states, err := svc.ListConversationReadStates(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, states)
}
