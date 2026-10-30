// Package servicefakes provides in-memory implementations of the logic service
// contracts, shared by the service and RPC-logic test suites so behaviour stays
// identical wherever a store is faked.
package servicefakes

import (
	"context"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
)

// The fake is assigned to service.ConversationStore by the tests that build the
// service, so a contract drift fails to compile there.

// ConversationStore is an in-memory ConversationStore with error injection.
type ConversationStore struct {
	Conversations       map[int64]orm.Conversation
	Members             map[int64][]orm.ConversationMember // key: conversationID
	Messages            map[int64][]orm.Message            // key: conversationID
	ReadStates          map[int64]map[int64]orm.ConversationReadState
	CreateErr           error
	GetErr              error
	AddMemberErr        error
	GetMembersErr       error
	ListMessagesErr     error
	ListMessagesInitErr error
	CountErr            error
	UpsertReadErr       error
	ListReadStatesErr   error
}

func (f *ConversationStore) CreateConversation(ctx context.Context, id int64, conversationType sharedconversation.Type, name, avatar string, creatorID int64) (orm.Conversation, error) {
	if f.CreateErr != nil {
		return orm.Conversation{}, f.CreateErr
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
	f.Conversations[id] = conv
	return orm.Conversation(conv), nil
}

func (f *ConversationStore) GetConversation(ctx context.Context, id int64) (orm.Conversation, error) {
	if f.GetErr != nil {
		return orm.Conversation{}, f.GetErr
	}

	conv, ok := f.Conversations[id]
	if !ok {
		return orm.Conversation{}, dao.ErrNotFound
	}
	return conv, nil
}

func (f *ConversationStore) AddConversationMembers(ctx context.Context, conversationID, userID int64) (int64, error) {
	if f.AddMemberErr != nil {
		return 0, f.AddMemberErr
	}

	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		IsMuted:        false,
		JoinedAt:       time.Now(),
	}
	f.Members[conversationID] = append(f.Members[conversationID], member)
	return 1, nil
}

func (f *ConversationStore) AddConversationMemberWithRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error) {
	if f.AddMemberErr != nil {
		return 0, f.AddMemberErr
	}

	member := orm.ConversationMember{
		ConversationID: conversationID,
		UserID:         userID,
		Role:           role,
		JoinedAt:       time.Now(),
	}
	f.Members[conversationID] = append(f.Members[conversationID], member)
	return 1, nil
}

func (f *ConversationStore) RemoveConversationMembers(ctx context.Context, conversationID int64, userIDs []int64) (int64, error) {
	removeSet := make(map[int64]bool)
	for _, uid := range userIDs {
		removeSet[uid] = true
	}
	kept := make([]orm.ConversationMember, 0, len(f.Members[conversationID]))
	removed := int64(0)
	for _, m := range f.Members[conversationID] {
		if removeSet[m.UserID] {
			removed++
		} else {
			kept = append(kept, m)
		}
	}
	f.Members[conversationID] = kept
	return removed, nil
}

func (f *ConversationStore) UpdateConversation(ctx context.Context, id int64, name, avatar *string) error {
	conv, ok := f.Conversations[id]
	if !ok {
		return dao.ErrNotFound
	}
	if name != nil {
		conv.Name = *name
	}
	if avatar != nil {
		conv.Avatar = *avatar
	}
	f.Conversations[id] = conv
	return nil
}

func (f *ConversationStore) UpdateConversationCreator(ctx context.Context, conversationID, creatorID int64) error {
	conv, ok := f.Conversations[conversationID]
	if !ok {
		return dao.ErrNotFound
	}
	conv.CreatorID = creatorID
	f.Conversations[conversationID] = conv
	return nil
}

func (f *ConversationStore) UpdateConversationMemberRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error) {
	Members := f.Members[conversationID]
	for i, m := range Members {
		if m.UserID == userID {
			Members[i].Role = role
			f.Members[conversationID] = Members
			return 1, nil
		}
	}
	return 0, nil
}

func (f *ConversationStore) DeactivateConversation(ctx context.Context, id int64) error {
	conv, ok := f.Conversations[id]
	if !ok {
		return dao.ErrNotFound
	}
	conv.IsActive = false
	f.Conversations[id] = conv
	return nil
}

func (f *ConversationStore) GetConversationCreator(ctx context.Context, id int64) (int64, error) {
	conv, ok := f.Conversations[id]
	if !ok {
		return 0, dao.ErrNotFound
	}
	return conv.CreatorID, nil
}

func (f *ConversationStore) IsConversationMember(ctx context.Context, conversationID, userID int64) (bool, error) {
	for _, m := range f.Members[conversationID] {
		if m.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (f *ConversationStore) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]dao.ConversationMemberDetail, error) {
	var result []dao.ConversationMemberDetail
	for _, m := range f.Members[conversationID] {
		result = append(result, dao.ConversationMemberDetail{
			UserID:   m.UserID,
			Email:    "",
			Role:     string(m.Role),
			JoinedAt: m.JoinedAt,
		})
	}
	return result, nil
}

func (f *ConversationStore) InsertMessage(ctx context.Context, message orm.Message) error {
	message.CreatedAt = time.Now()
	f.Messages[message.ConversationID] = append(f.Messages[message.ConversationID], message)
	return nil
}

func (f *ConversationStore) InsertOutboxRecord(context.Context, string, string, []byte) (int64, error) {
	return 0, nil
}

func (f *ConversationStore) UpsertConversationReadState(_ context.Context, conversationID, userID, lastReadMessageID int64) (orm.ConversationReadState, error) {
	if f.UpsertReadErr != nil {
		return orm.ConversationReadState{}, f.UpsertReadErr
	}

	if f.ReadStates == nil {
		f.ReadStates = make(map[int64]map[int64]orm.ConversationReadState)
	}

	conv, ok := f.ReadStates[conversationID]
	if !ok {
		conv = make(map[int64]orm.ConversationReadState)
		f.ReadStates[conversationID] = conv
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

func (f *ConversationStore) ListConversationReadStates(_ context.Context, conversationID int64) ([]orm.ConversationReadState, error) {
	if f.ListReadStatesErr != nil {
		return nil, f.ListReadStatesErr
	}

	conv := f.ReadStates[conversationID]
	items := make([]orm.ConversationReadState, 0, len(conv))
	for _, state := range conv {
		items = append(items, state)
	}
	return items, nil
}

func (f *ConversationStore) GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	if f.GetMembersErr != nil {
		return nil, f.GetMembersErr
	}

	return f.Members[conversationID], nil
}

func (f *ConversationStore) GetConversationsByUserID(ctx context.Context, userID int64) ([]orm.Conversation, error) {
	var result []orm.Conversation
	for convID, conv := range f.Conversations {
		for _, m := range f.Members[convID] {
			if m.UserID == userID {
				result = append(result, orm.Conversation(conv))
				break
			}
		}
	}
	return result, nil
}

func (f *ConversationStore) ListMessagesByConversation(ctx context.Context, conversationID int64, cursorCreatedAt time.Time, cursorID int64, limit int32) ([]orm.Message, error) {
	if f.ListMessagesErr != nil {
		return nil, f.ListMessagesErr
	}

	msgs := f.Messages[conversationID]
	if msgs == nil {
		return []orm.Message{}, nil
	}

	// Filter Messages based on cursor (CreatedAt and ID)
	var result []orm.Message
	for _, msg := range msgs {
		// Simple cursor filter: Messages with ID < cursorID
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

func (f *ConversationStore) ListMessagesByConversationInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error) {
	if f.ListMessagesInitErr != nil {
		return nil, f.ListMessagesInitErr
	}

	msgs := f.Messages[conversationID]
	if msgs == nil {
		return []orm.Message{}, nil
	}

	// Return first N Messages
	if int64(len(msgs)) <= int64(limit) {
		return msgs, nil
	}
	return msgs[:limit], nil
}

func (f *ConversationStore) CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error) {
	if f.CountErr != nil {
		return 0, f.CountErr
	}
	return int64(len(f.Messages[conversationID])), nil
}

func (f *ConversationStore) GetDirectConversationByMembers(ctx context.Context, userID, peerID int64) (orm.Conversation, error) {
	if f.GetErr != nil {
		return orm.Conversation{}, f.GetErr
	}
	// Search for a conversation where both users are Members
	for convID, conv := range f.Conversations {
		if conv.ConversationType != "direct" || !conv.IsActive {
			continue
		}
		memberSet := make(map[int64]bool)
		for _, m := range f.Members[convID] {
			memberSet[m.UserID] = true
		}
		if memberSet[userID] && memberSet[peerID] {
			return orm.Conversation(conv), nil
		}
	}

	return orm.Conversation{}, dao.ErrNotFound
}

func NewConversationStore() *ConversationStore {
	return &ConversationStore{
		Conversations: make(map[int64]orm.Conversation),
		Members:       make(map[int64][]orm.ConversationMember),
		Messages:      make(map[int64][]orm.Message),
		ReadStates:    make(map[int64]map[int64]orm.ConversationReadState),
	}
}
