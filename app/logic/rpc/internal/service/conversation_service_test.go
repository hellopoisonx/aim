package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConversationStore implements ConversationStore for testing.
type fakeConversationStore struct {
	conversations       map[int64]orm.Conversation
	members             map[int64][]orm.ConversationMember // key: conversationID
	messages            map[int64][]orm.Message            // key: conversationID
	readStates          map[int64]map[int64]orm.ConversationReadState
	createErr           error
	getErr              error
	addMemberErr        error
	getMembersErr       error
	listMessagesErr     error
	listMessagesInitErr error
	countErr            error
	upsertReadErr       error
	listReadStatesErr   error
}

func (f *fakeConversationStore) CreateConversation(ctx context.Context, id int64, conversationType, name, avatar string, creatorID int64) (orm.Conversation, error) {
	if f.createErr != nil {
		return orm.Conversation{}, f.createErr
	}

	conv := orm.Conversation{
		ID:               id,
		ConversationType: conversationType,
		IsActive:         true,
		CreatedAt:        time.Now(),
		Name:             name,
		Avatar:           avatar,
		CreatorID:        creatorID,
	}
	f.conversations[id] = conv
	return orm.Conversation(conv), nil
}

func (f *fakeConversationStore) GetConversation(ctx context.Context, id int64) (orm.Conversation, error) {
	if f.getErr != nil {
		return orm.Conversation{}, f.getErr
	}

	conv, ok := f.conversations[id]
	if !ok {
		return orm.Conversation{}, dao.ErrNotFound
	}
	return conv, nil
}

func (f *fakeConversationStore) AddConversationMembers(ctx context.Context, conversationID, userID int64) (int64, error) {
	if f.addMemberErr != nil {
		return 0, f.addMemberErr
	}

	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		IsMuted:        false,
		JoinedAt:       time.Now(),
	}
	f.members[conversationID] = append(f.members[conversationID], member)
	return 1, nil
}

func (f *fakeConversationStore) AddConversationMemberWithRole(ctx context.Context, conversationID, userID int64, role string) (int64, error) {
	if f.addMemberErr != nil {
		return 0, f.addMemberErr
	}

	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
	}
	f.members[conversationID] = append(f.members[conversationID], member)
	return 1, nil
}

func (f *fakeConversationStore) RemoveConversationMembers(ctx context.Context, conversationID int64, userIDs []int64) (int64, error) {
	removeSet := make(map[int64]bool)
	for _, uid := range userIDs {
		removeSet[uid] = true
	}
	kept := make([]orm.ConversationMember, 0, len(f.members[conversationID]))
	removed := int64(0)
	for _, m := range f.members[conversationID] {
		if removeSet[m.UserID] {
			removed++
		} else {
			kept = append(kept, m)
		}
	}
	f.members[conversationID] = kept
	return removed, nil
}

func (f *fakeConversationStore) UpdateConversation(ctx context.Context, id int64, name, avatar *string) error {
	conv, ok := f.conversations[id]
	if !ok {
		return dao.ErrNotFound
	}
	if name != nil {
		conv.Name = *name
	}
	if avatar != nil {
		conv.Avatar = *avatar
	}
	f.conversations[id] = conv
	return nil
}

func (f *fakeConversationStore) UpdateConversationCreator(ctx context.Context, conversationID, creatorID int64) error {
	conv, ok := f.conversations[conversationID]
	if !ok {
		return dao.ErrNotFound
	}
	conv.CreatorID = creatorID
	f.conversations[conversationID] = conv
	return nil
}

func (f *fakeConversationStore) UpdateConversationMemberRole(ctx context.Context, conversationID, userID int64, role string) (int64, error) {
	members := f.members[conversationID]
	for i, m := range members {
		if m.UserID == userID {
			members[i].Role = role
			f.members[conversationID] = members
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeConversationStore) DeactivateConversation(ctx context.Context, id int64) error {
	conv, ok := f.conversations[id]
	if !ok {
		return dao.ErrNotFound
	}
	conv.IsActive = false
	f.conversations[id] = conv
	return nil
}

func (f *fakeConversationStore) GetConversationCreator(ctx context.Context, id int64) (int64, error) {
	conv, ok := f.conversations[id]
	if !ok {
		return 0, dao.ErrNotFound
	}
	return conv.CreatorID, nil
}

func (f *fakeConversationStore) IsConversationMember(ctx context.Context, conversationID, userID int64) (bool, error) {
	for _, m := range f.members[conversationID] {
		if m.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeConversationStore) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]dao.ConversationMemberDetail, error) {
	var result []dao.ConversationMemberDetail
	for _, m := range f.members[conversationID] {
		result = append(result, dao.ConversationMemberDetail{
			UserID:   m.UserID,
			Email:    "",
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		})
	}
	return result, nil
}

func (f *fakeConversationStore) InsertMessage(ctx context.Context, message orm.Message) error {
	message.CreatedAt = time.Now()
	f.messages[message.ConversationID] = append(f.messages[message.ConversationID], message)
	return nil
}

func (f *fakeConversationStore) InsertOutboxRecord(context.Context, string, string, []byte) (int64, error) {
	return 0, nil
}

func (f *fakeConversationStore) UpsertConversationReadState(_ context.Context, conversationID, userID, lastReadMessageID int64) (orm.ConversationReadState, error) {
	if f.upsertReadErr != nil {
		return orm.ConversationReadState{}, f.upsertReadErr
	}

	if f.readStates == nil {
		f.readStates = make(map[int64]map[int64]orm.ConversationReadState)
	}

	conv, ok := f.readStates[conversationID]
	if !ok {
		conv = make(map[int64]orm.ConversationReadState)
		f.readStates[conversationID] = conv
	}

	now := time.Now()
	state, exists := conv[userID]
	if !exists {
		state = orm.ConversationReadState{
			ConversationID:    conversationID,
			UserID:            userID,
			LastReadMessageID: lastReadMessageID,
			UpdatedAt:         now,
		}
	} else if lastReadMessageID > state.LastReadMessageID {
		state.LastReadMessageID = lastReadMessageID
		state.UpdatedAt = now
	}

	conv[userID] = state
	return state, nil
}

func (f *fakeConversationStore) ListConversationReadStates(_ context.Context, conversationID int64) ([]orm.ConversationReadState, error) {
	if f.listReadStatesErr != nil {
		return nil, f.listReadStatesErr
	}

	conv := f.readStates[conversationID]
	items := make([]orm.ConversationReadState, 0, len(conv))
	for _, state := range conv {
		items = append(items, state)
	}
	return items, nil
}

func (f *fakeConversationStore) GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	if f.getMembersErr != nil {
		return nil, f.getMembersErr
	}

	return f.members[conversationID], nil
}

func (f *fakeConversationStore) GetConversationsByUserID(ctx context.Context, userID int64) ([]orm.Conversation, error) {
	var result []orm.Conversation
	for convID, conv := range f.conversations {
		for _, m := range f.members[convID] {
			if m.UserID == userID {
				result = append(result, orm.Conversation(conv))
				break
			}
		}
	}
	return result, nil
}

func (f *fakeConversationStore) ListMessagesByConversation(ctx context.Context, conversationID int64, cursorCreatedAt time.Time, cursorID int64, limit int32) ([]orm.Message, error) {
	if f.listMessagesErr != nil {
		return nil, f.listMessagesErr
	}

	msgs := f.messages[conversationID]
	if msgs == nil {
		return []orm.Message{}, nil
	}

	// Filter messages based on cursor (CreatedAt and ID)
	var result []orm.Message
	for _, msg := range msgs {
		// Simple cursor filter: messages with ID < cursorID
		if cursorID > 0 && msg.ID >= cursorID {
			continue
		}
		result = append(result, msg)
		if int64(len(result)) >= int64(limit) {
			break
		}
	}
	return result, nil
}

func (f *fakeConversationStore) ListMessagesByConversationInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error) {
	if f.listMessagesInitErr != nil {
		return nil, f.listMessagesInitErr
	}

	msgs := f.messages[conversationID]
	if msgs == nil {
		return []orm.Message{}, nil
	}

	// Return first N messages
	if int64(len(msgs)) <= int64(limit) {
		return msgs, nil
	}
	return msgs[:limit], nil
}

func (f *fakeConversationStore) CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return int64(len(f.messages[conversationID])), nil
}

func (f *fakeConversationStore) GetDirectConversationByMembers(ctx context.Context, userID, peerID int64) (orm.Conversation, error) {
	if f.getErr != nil {
		return orm.Conversation{}, f.getErr
	}
	// Search for a conversation where both users are members
	for convID, conv := range f.conversations {
		if conv.ConversationType != "direct" || !conv.IsActive {
			continue
		}
		memberSet := make(map[int64]bool)
		for _, m := range f.members[convID] {
			memberSet[m.UserID] = true
		}
		if memberSet[userID] && memberSet[peerID] {
			return orm.Conversation(conv), nil
		}
	}

	return orm.Conversation{}, dao.ErrNotFound
}

// testSnowflake is a shared Snowflake instance for tests.
var testSnowflake = func() *tools.Snowflake {
	s, err := tools.NewSnowflake(1)
	if err != nil {
		panic(err)
	}
	return s
}()

func newFakeConversationStore() *fakeConversationStore {
	return &fakeConversationStore{
		conversations: make(map[int64]orm.Conversation),
		members:       make(map[int64][]orm.ConversationMember),
		messages:      make(map[int64][]orm.Message),
	}
}

// ---------------------------------------------------------------------------
// CreateConversation tests
// ---------------------------------------------------------------------------

func TestConversationService_CreateConversation_ValidDirect(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// direct conversations: member_ids must contain exactly the peer (creator is auto-added).
	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)
	assert.Equal(t, "direct", conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify members were added (creator auto-added on top of peer)
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_CreateConversation_ValidGroup(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3, 4}, "Group Chat", "")
	require.NoError(t, err)
	assert.Equal(t, "group", conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify all members were added
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 4)
	var foundOwner bool
	for _, m := range members {
		if m.UserID == 1 {
			assert.Equal(t, "owner", m.Role)
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
			store := newFakeConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), tt.conversationType, 1, []int64{1, 2}, "conv", "")
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
			store := newFakeConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2}, tt.convName, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestConversationService_CreateConversation_DirectAllowsEmptyName(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Direct conversation names are now computed by the logic RPC layer before calling the service.
	// The service layer accepts whatever name is passed, including empty.
	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "", "")
	require.NoError(t, err)
	assert.Empty(t, conv.Name)
}

func TestConversationService_CreateConversation_EmptyMembers(t *testing.T) {
	store := newFakeConversationStore()
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
			store := newFakeConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			_, err := svc.CreateConversation(context.Background(), "direct", 1, tt.memberIDs, "direct", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestConversationService_CreateConversation_DirectWithCreatorNotInMembers(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Creator (1) is not in the member list; the service should auto-add it.
	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
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
			assert.Equal(t, "owner", m.Role)
			foundOwner = true
		}
	}
	assert.True(t, foundOwner)
}

func TestConversationService_RoleGuards(t *testing.T) {
	store := newFakeConversationStore()
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

func testMemberRole(t *testing.T, store *fakeConversationStore, conversationID, userID int64) string {
	t.Helper()
	for _, m := range store.members[conversationID] {
		if m.UserID == userID {
			return m.Role
		}
	}
	t.Fatalf("member %d not found in conversation %d", userID, conversationID)
	return ""
}

func TestConversationService_GrantAndRevokeGroupAdmin(t *testing.T) {
	store := newFakeConversationStore()
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
	store := newFakeConversationStore()
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
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.NoError(t, err)

	_, err = svc.TransferGroupOwner(context.Background(), conv.ID, 2, 3)
	require.ErrorIs(t, err, ErrNotOwner)

	updated, err := svc.TransferGroupOwner(context.Background(), conv.ID, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.CreatorID)
	assert.Equal(t, int64(2), store.conversations[conv.ID].CreatorID)
	assert.Equal(t, "admin", testMemberRole(t, store, conv.ID, 1))
	assert.Equal(t, "owner", testMemberRole(t, store, conv.ID, 2))

	// Old owner is now admin, not owner, and cannot transfer again.
	_, err = svc.TransferGroupOwner(context.Background(), conv.ID, 1, 3)
	require.ErrorIs(t, err, ErrNotOwner)
}

func TestConversationService_OwnerOnlyRoleTargets(t *testing.T) {
	store := newFakeConversationStore()
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
	store := newFakeConversationStore()
	store.createErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2}, "Group", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

func TestConversationService_CreateConversation_AddMemberError(t *testing.T) {
	store := newFakeConversationStore()
	store.addMemberErr = errors.New("failed to add member")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3}, "Group", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add member")
}

func TestConversationService_CreateConversation_DirectDedup_Existing(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// First call: create a direct conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Second call: same two users should return the existing conversation
	conv2, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Should return the SAME conversation ID
	assert.Equal(t, conv1.ID, conv2.ID)
	assert.Equal(t, "direct", conv2.ConversationType)
	assert.True(t, conv2.IsActive)
}

func TestConversationService_CreateConversation_DirectDedup_NewMembers(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// Create conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
	require.NoError(t, err)

	// Create conversation between users 3 and 4 (different pair)
	conv2, err := svc.CreateConversation(context.Background(), "direct", 3, []int64{4}, "direct-3-4", "")
	require.NoError(t, err)

	// Should be DIFFERENT conversation IDs
	assert.NotEqual(t, conv1.ID, conv2.ID)
}

func TestConversationService_CreateConversation_DirectDedup_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.getErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2}, "direct-1-2", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

// ---------------------------------------------------------------------------
// GetConversationByID tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationByID_Found(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[100] = orm.Conversation{
		ID:               100,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	conv, err := svc.GetConversationByID(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(100), conv.ID)
	assert.Equal(t, "direct", conv.ConversationType)
}

func TestConversationService_GetConversationByID_NotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationByID(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationByID_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.getErr = errors.New("database connection error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationByID(context.Background(), 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

// ---------------------------------------------------------------------------
// GetConversationHistory tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationHistory_WithCursor(t *testing.T) {
	store := newFakeConversationStore()
	// Create conversation
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	// Add messages
	now := time.Now()
	store.messages[1] = []orm.Message{
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
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.messages[1] = []orm.Message{
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
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistory(context.Background(), 999, 0, 1, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistory_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.listMessagesErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistory(context.Background(), 1, 0, 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistory_LimitNormalization(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
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
	store.messages[1] = msgs
	svc := NewConversationService(store, testSnowflake, nil, nil)

	// When limit > 100, it should be normalized to 50
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 200)
	require.NoError(t, err)
	assert.Len(t, messages, 50)
}

func TestConversationService_GetConversationHistory_ZeroLimit(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.messages[1] = []orm.Message{
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
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.messages[1] = []orm.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("first"), CreatedAt: now},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("second"), CreatedAt: now.Add(time.Second)},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	messages, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestConversationService_GetConversationHistoryInitial_NotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 999, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistoryInitial_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.listMessagesInitErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistoryInitial_LimitNormalization(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
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
	store.messages[1] = msgs
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
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	members, err := svc.GetConversationMembers(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_GetConversationMembers_ConversationNotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationMembers(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationMembers_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.getMembersErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.GetConversationMembers(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationMembers_EmptyMembers(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
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
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	isMember, err := svc.IsMember(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.True(t, isMember)
}

func TestConversationService_IsMember_False(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.members[1] = []orm.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: time.Now()},
	}
	svc := NewConversationService(store, testSnowflake, nil, nil)

	isMember, err := svc.IsMember(context.Background(), 1, 999)
	require.NoError(t, err)
	assert.False(t, isMember)
}

func TestConversationService_IsMember_ConversationNotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.IsMember(context.Background(), 999, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_IsMember_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.getMembersErr = errors.New("database error")
	svc := NewConversationService(store, testSnowflake, nil, nil)

	_, err := svc.IsMember(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

// ---------------------------------------------------------------------------
// ConversationToGRPCError tests
// ---------------------------------------------------------------------------

func TestConversationToGRPCError_ConversationNotFound(t *testing.T) {
	err := ConversationToGRPCError(ErrConversationNotFound)
	assert.Contains(t, err.Error(), "conversation not found")
}

func TestConversationToGRPCError_NotMember(t *testing.T) {
	err := ConversationToGRPCError(ErrNotMember)
	assert.Contains(t, err.Error(), "not a member of the conversation")
}

func TestConversationToGRPCError_Unknown(t *testing.T) {
	err := ConversationToGRPCError(errors.New("some random error"))
	assert.Contains(t, err.Error(), "internal error")
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
			store := newFakeConversationStore()
			svc := NewConversationService(store, testSnowflake, nil, nil)

			conv, err := svc.CreateConversation(context.Background(), tt.convType, tt.creatorID, tt.memberIDs, tt.convName, "")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.convType, conv.ConversationType)

				members, err := store.GetConversationMembers(context.Background(), conv.ID)
				require.NoError(t, err)
				assert.Len(t, members, tt.expectedMembers)
			}
		})
	}
}

func TestConversationService_GetConversationHistory_TableDriven(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	now := time.Now()
	store.messages[1] = []orm.Message{
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
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.members[1] = []orm.ConversationMember{
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

func newReadReceiptFixture(t *testing.T) (*ConversationService, *fakeConversationStore) {
	t.Helper()
	store := newFakeConversationStore()
	store.conversations[1] = orm.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        time.Now(),
	}
	store.members[1] = []orm.ConversationMember{
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
