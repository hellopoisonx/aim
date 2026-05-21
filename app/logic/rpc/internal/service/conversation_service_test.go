package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConversationStore implements ConversationStore for testing.
type fakeConversationStore struct {
	conversations      map[int64]model.Conversation
	members            map[int64][]model.ConversationMember // key: conversationID
	messages           map[int64][]model.Message           // key: conversationID
	createErr          error
	getErr             error
	addMemberErr       error
	getMembersErr      error
	listMessagesErr    error
	listMessagesInitErr error
	countErr           error
}

func (f *fakeConversationStore) CreateConversation(ctx context.Context, arg model.CreateConversationParams) (model.Conversation, error) {
	if f.createErr != nil {
		return model.Conversation{}, f.createErr
	}

	conv := model.Conversation{
		ID:               arg.ID,
		ConversationType: arg.ConversationType,
		IsActive:         true,
		CreatedAt: pgtype.Timestamptz{
			Time:  time.Now(),
			Valid: true,
		},
	}
	f.conversations[conv.ID] = conv
	return conv, nil
}

func (f *fakeConversationStore) GetConversation(ctx context.Context, id int64) (model.Conversation, error) {
	if f.getErr != nil {
		return model.Conversation{}, f.getErr
	}

	conv, ok := f.conversations[id]
	if !ok {
		return model.Conversation{}, pgx.ErrNoRows
	}
	return conv, nil
}

func (f *fakeConversationStore) AddConversationMembers(ctx context.Context, arg model.AddConversationMembersParams) (int64, error) {
	if f.addMemberErr != nil {
		return 0, f.addMemberErr
	}

	member := model.ConversationMember{
		ConversationID: arg.ConversationID,
		UserID:        arg.UserID,
		IsMuted:       false,
		JoinedAt: pgtype.Timestamptz{
			Time:  time.Now(),
			Valid: true,
		},
	}
	f.members[arg.ConversationID] = append(f.members[arg.ConversationID], member)
	return 1, nil
}

func (f *fakeConversationStore) GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error) {
	if f.getMembersErr != nil {
		return nil, f.getMembersErr
	}

	return f.members[conversationID], nil
}

func (f *fakeConversationStore) ListMessagesByConversation(ctx context.Context, arg model.ListMessagesByConversationParams) ([]model.Message, error) {
	if f.listMessagesErr != nil {
		return nil, f.listMessagesErr
	}

	msgs := f.messages[arg.ConversationID]
	if msgs == nil {
		return []model.Message{}, nil
	}

	// Filter messages based on cursor (CreatedAt and ID)
	var result []model.Message
	for _, msg := range msgs {
		// Simple cursor filter: messages with ID < cursorID
		if arg.ID > 0 && msg.ID >= arg.ID {
			continue
		}
		result = append(result, msg)
		if int32(len(result)) >= arg.Limit {
			break
		}
	}
	return result, nil
}

func (f *fakeConversationStore) ListMessagesByConversationInitial(ctx context.Context, arg model.ListMessagesByConversationInitialParams) ([]model.Message, error) {
	if f.listMessagesInitErr != nil {
		return nil, f.listMessagesInitErr
	}

	msgs := f.messages[arg.ConversationID]
	if msgs == nil {
		return []model.Message{}, nil
	}

	// Return first N messages
	if int32(len(msgs)) <= arg.Limit {
		return msgs, nil
	}
	return msgs[:arg.Limit], nil
}

func (f *fakeConversationStore) CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return int64(len(f.messages[conversationID])), nil
}

func (f *fakeConversationStore) GetDirectConversationByMembers(ctx context.Context, arg model.GetDirectConversationByMembersParams) (model.Conversation, error) {
	if f.getErr != nil {
		return model.Conversation{}, f.getErr
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
		if memberSet[arg.UserID] && memberSet[arg.UserID_2] {
			return conv, nil
		}
	}
	return model.Conversation{}, pgx.ErrNoRows
}

func newFakeConversationStore() *fakeConversationStore {
	return &fakeConversationStore{
		conversations: make(map[int64]model.Conversation),
		members:       make(map[int64][]model.ConversationMember),
		messages:      make(map[int64][]model.Message),
	}
}

// ---------------------------------------------------------------------------
// CreateConversation tests
// ---------------------------------------------------------------------------

func TestConversationService_CreateConversation_ValidDirect(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{1, 2})
	require.NoError(t, err)
	assert.Equal(t, "direct", conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify members were added
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_CreateConversation_ValidGroup(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	conv, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3, 4})
	require.NoError(t, err)
	assert.Equal(t, "group", conv.ConversationType)
	assert.True(t, conv.IsActive)

	// Verify all members were added
	members, err := store.GetConversationMembers(context.Background(), conv.ID)
	require.NoError(t, err)
	assert.Len(t, members, 4)
}

func TestConversationService_CreateConversation_InvalidType(t *testing.T) {
	tests := []struct {
		name            string
		conversationType string
	}{
		{name: "empty_string", conversationType: ""},
		{name: "random_type", conversationType: "channel"},
		{name: "uppercase_direct", conversationType: "Direct"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeConversationStore()
			svc := NewConversationService(store)

			_, err := svc.CreateConversation(context.Background(), tt.conversationType, 1, []int64{1, 2})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "conversation_type must be 'direct' or 'group'")
		})
	}
}

func TestConversationService_CreateConversation_EmptyMembers(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "member_ids must not be empty")
}

func TestConversationService_CreateConversation_DirectWithNotTwoMembers(t *testing.T) {
	tests := []struct {
		name      string
		memberIDs []int64
	}{
		{name: "single_member", memberIDs: []int64{1}},
		{name: "three_members", memberIDs: []int64{1, 2, 3}},
		{name: "no_creator", memberIDs: []int64{2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeConversationStore()
			svc := NewConversationService(store)

			_, err := svc.CreateConversation(context.Background(), "direct", 1, tt.memberIDs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "direct conversation must have exactly 2 members")
		})
	}
}

func TestConversationService_CreateConversation_DirectWithCreatorNotInMembers(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	// Creator (1) is not in the member list
	conv, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{2})
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
}

func TestConversationService_CreateConversation_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.createErr = errors.New("database connection error")
	svc := NewConversationService(store)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

func TestConversationService_CreateConversation_AddMemberError(t *testing.T) {
	store := newFakeConversationStore()
	store.addMemberErr = errors.New("failed to add member")
	svc := NewConversationService(store)

	_, err := svc.CreateConversation(context.Background(), "group", 1, []int64{1, 2, 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add member")
}

func TestConversationService_CreateConversation_DirectDedup_Existing(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	// First call: create a direct conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{1, 2})
	require.NoError(t, err)

	// Second call: same two users should return the existing conversation
	conv2, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{1, 2})
	require.NoError(t, err)

	// Should return the SAME conversation ID
	assert.Equal(t, conv1.ID, conv2.ID)
	assert.Equal(t, "direct", conv2.ConversationType)
	assert.True(t, conv2.IsActive)
}

func TestConversationService_CreateConversation_DirectDedup_NewMembers(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	// Create conversation between users 1 and 2
	conv1, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{1, 2})
	require.NoError(t, err)

	// Create conversation between users 3 and 4 (different pair)
	conv2, err := svc.CreateConversation(context.Background(), "direct", 3, []int64{3, 4})
	require.NoError(t, err)

	// Should be DIFFERENT conversation IDs
	assert.NotEqual(t, conv1.ID, conv2.ID)
}

func TestConversationService_CreateConversation_DirectDedup_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.getErr = errors.New("database connection error")
	svc := NewConversationService(store)

	_, err := svc.CreateConversation(context.Background(), "direct", 1, []int64{1, 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

// ---------------------------------------------------------------------------
// GetConversationByID tests
// ---------------------------------------------------------------------------

func TestConversationService_GetConversationByID_Found(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[100] = model.Conversation{
		ID:               100,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	svc := NewConversationService(store)

	conv, err := svc.GetConversationByID(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(100), conv.ID)
	assert.Equal(t, "direct", conv.ConversationType)
}

func TestConversationService_GetConversationByID_NotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.GetConversationByID(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationByID_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.getErr = errors.New("database connection error")
	svc := NewConversationService(store)

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
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	// Add messages
	now := time.Now()
	store.messages[1] = []model.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg1"), CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("msg2"), CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true}},
		{ID: 3, ConversationID: 1, SenderID: 1, Content: []byte("msg3"), CreatedAt: pgtype.Timestamptz{Time: now.Add(2 * time.Second), Valid: true}},
	}
	svc := NewConversationService(store)

	// Get messages with cursor at message ID 2
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 2, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, int64(1), messages[0].ID)
}

func TestConversationService_GetConversationHistory_InitialPage(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	now := time.Now()
	store.messages[1] = []model.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("first"), CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("second"), CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true}},
	}
	svc := NewConversationService(store)

	// When cursor is 0,0 it should call GetConversationHistoryInitial
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestConversationService_GetConversationHistory_ConversationNotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.GetConversationHistory(context.Background(), 999, 0, 1, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistory_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.listMessagesErr = errors.New("database error")
	svc := NewConversationService(store)

	_, err := svc.GetConversationHistory(context.Background(), 1, 0, 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistory_LimitNormalization(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	// Create more than 100 messages
	now := time.Now()
	var msgs []model.Message
	for i := int64(1); i <= 150; i++ {
		msgs = append(msgs, model.Message{
			ID:             i,
			ConversationID: 1,
			SenderID:       1,
			Content:        []byte("msg"),
			CreatedAt:      pgtype.Timestamptz{Time: now.Add(time.Duration(i) * time.Second), Valid: true},
		})
	}
	store.messages[1] = msgs
	svc := NewConversationService(store)

	// When limit > 100, it should be normalized to 50
	messages, err := svc.GetConversationHistory(context.Background(), 1, 0, 0, 200)
	require.NoError(t, err)
	assert.Len(t, messages, 50)
}

func TestConversationService_GetConversationHistory_ZeroLimit(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	now := time.Now()
	store.messages[1] = []model.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg"), CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
	}
	svc := NewConversationService(store)

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
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	now := time.Now()
	store.messages[1] = []model.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("first"), CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("second"), CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true}},
	}
	svc := NewConversationService(store)

	messages, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

func TestConversationService_GetConversationHistoryInitial_NotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 999, 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationHistoryInitial_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.listMessagesInitErr = errors.New("database error")
	svc := NewConversationService(store)

	_, err := svc.GetConversationHistoryInitial(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationHistoryInitial_LimitNormalization(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	now := time.Now()
	// Create more than 100 messages
	var msgs []model.Message
	for i := int64(1); i <= 150; i++ {
		msgs = append(msgs, model.Message{
			ID:             i,
			ConversationID: 1,
			SenderID:       1,
			Content:        []byte("msg"),
			CreatedAt:      pgtype.Timestamptz{Time: now.Add(time.Duration(i) * time.Second), Valid: true},
		})
	}
	store.messages[1] = msgs
	svc := NewConversationService(store)

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
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.members[1] = []model.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
	}
	svc := NewConversationService(store)

	members, err := svc.GetConversationMembers(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestConversationService_GetConversationMembers_ConversationNotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.GetConversationMembers(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_GetConversationMembers_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.getMembersErr = errors.New("database error")
	svc := NewConversationService(store)

	_, err := svc.GetConversationMembers(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestConversationService_GetConversationMembers_EmptyMembers(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	// No members added
	svc := NewConversationService(store)

	members, err := svc.GetConversationMembers(context.Background(), 1)
	require.NoError(t, err)
	assert.Len(t, members, 0)
}

// ---------------------------------------------------------------------------
// IsMember tests
// ---------------------------------------------------------------------------

func TestConversationService_IsMember_True(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.members[1] = []model.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
	}
	svc := NewConversationService(store)

	isMember, err := svc.IsMember(context.Background(), 1, 1)
	require.NoError(t, err)
	assert.True(t, isMember)
}

func TestConversationService_IsMember_False(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.members[1] = []model.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
	}
	svc := NewConversationService(store)

	isMember, err := svc.IsMember(context.Background(), 1, 999)
	require.NoError(t, err)
	assert.False(t, isMember)
}

func TestConversationService_IsMember_ConversationNotFound(t *testing.T) {
	store := newFakeConversationStore()
	svc := NewConversationService(store)

	_, err := svc.IsMember(context.Background(), 999, 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConversationNotFound)
}

func TestConversationService_IsMember_StoreError(t *testing.T) {
	store := newFakeConversationStore()
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "direct",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.getMembersErr = errors.New("database error")
	svc := NewConversationService(store)

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
		wantErr         bool
		errContains     string
		expectedMembers int
	}{
		{
			name:            "valid_direct_two_members",
			convType:        "direct",
			creatorID:       1,
			memberIDs:       []int64{1, 2},
			wantErr:         false,
			expectedMembers: 2,
		},
		{
			name:            "valid_group_multiple_members",
			convType:        "group",
			creatorID:       1,
			memberIDs:       []int64{1, 2, 3},
			wantErr:         false,
			expectedMembers: 3,
		},
		{
			name:        "invalid_type_empty",
			convType:    "",
			creatorID:   1,
			memberIDs:   []int64{1, 2},
			wantErr:     true,
			errContains: "conversation_type must be 'direct' or 'group'",
		},
		{
			name:        "invalid_type_random",
			convType:    "channel",
			creatorID:   1,
			memberIDs:   []int64{1, 2},
			wantErr:     true,
			errContains: "conversation_type must be 'direct' or 'group'",
		},
		{
			name:        "empty_members",
			convType:    "group",
			creatorID:   1,
			memberIDs:   []int64{},
			wantErr:     true,
			errContains: "member_ids must not be empty",
		},
		{
			name:        "direct_single_member",
			convType:    "direct",
			creatorID:   1,
			memberIDs:    []int64{1},
			wantErr:     true,
			errContains: "direct conversation must have exactly 2 members",
		},
		{
			name:        "direct_three_members",
			convType:    "direct",
			creatorID:   1,
			memberIDs:    []int64{1, 2, 3},
			wantErr:     true,
			errContains: "direct conversation must have exactly 2 members",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeConversationStore()
			svc := NewConversationService(store)

			conv, err := svc.CreateConversation(context.Background(), tt.convType, tt.creatorID, tt.memberIDs)

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
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	now := time.Now()
	store.messages[1] = []model.Message{
		{ID: 1, ConversationID: 1, SenderID: 1, Content: []byte("msg1"), CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}},
		{ID: 2, ConversationID: 1, SenderID: 2, Content: []byte("msg2"), CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Second), Valid: true}},
		{ID: 3, ConversationID: 1, SenderID: 1, Content: []byte("msg3"), CreatedAt: pgtype.Timestamptz{Time: now.Add(2 * time.Second), Valid: true}},
	}
	svc := NewConversationService(store)

	tests := []struct {
		name           string
		conversationID int64
		cursorCreatedAt int64
		cursorID       int64
		limit          int32
		wantErr        bool
		errContains    string
		expectedCount  int
	}{
		{
			name:           "initial_page",
			conversationID: 1,
			cursorCreatedAt: 0,
			cursorID:       0,
			limit:          10,
			wantErr:        false,
			expectedCount:  3,
		},
		{
			name:           "with_cursor",
			conversationID: 1,
			cursorCreatedAt: 0,
			cursorID:       2,
			limit:          10,
			wantErr:        false,
			expectedCount:  1,
		},
		{
			name:           "conversation_not_found",
			conversationID: 999,
			cursorCreatedAt: 0,
			cursorID:       0,
			limit:          10,
			wantErr:        true,
			errContains:    "conversation not found",
		},
		{
			name:           "limit_exceeds_max",
			conversationID: 1,
			cursorCreatedAt: 0,
			cursorID:       0,
			limit:          200,
			wantErr:        false,
			expectedCount:  3, // normalized to 50, but store only has 3 messages
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
	store.conversations[1] = model.Conversation{
		ID:               1,
		ConversationType: "group",
		IsActive:         true,
		CreatedAt:        pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	store.members[1] = []model.ConversationMember{
		{ConversationID: 1, UserID: 1, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
		{ConversationID: 1, UserID: 2, IsMuted: false, JoinedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
	}
	svc := NewConversationService(store)

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
