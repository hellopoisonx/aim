package service

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

// fakeQuerier implements PermissionQuerier for tests.
type fakeQuerier struct {
	conversations    map[int64]orm.Conversation
	members          map[string]orm.ConversationMember
	friendships      map[string][]orm.Friendship
	messageCounts    map[int64]int64
	userTypes        map[int64]string
	getConvErr       error
	getMemberErr     error
	getFriendshipErr error
}

func (f *fakeQuerier) GetConversation(_ context.Context, id int64) (orm.Conversation, error) {
	if f.getConvErr != nil {
		return orm.Conversation{}, f.getConvErr
	}

	conv, ok := f.conversations[id]
	if !ok {
		return orm.Conversation{}, dao.ErrNotFound
	}

	return conv, nil
}

func (f *fakeQuerier) GetConversationMembers(_ context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	if f.getMemberErr != nil {
		return nil, f.getMemberErr
	}

	rows := make([]orm.ConversationMember, 0)
	for _, member := range f.members {
		if member.ConversationID == conversationID {
			rows = append(rows, member)
		}
	}
	return rows, nil
}

func (f *fakeQuerier) GetFriendshipBidirectional(_ context.Context, userID, friendID int64) ([]orm.Friendship, error) {
	if f.getFriendshipErr != nil {
		return nil, f.getFriendshipErr
	}

	return f.friendships[friendshipKey(userID, friendID)], nil
}

func (f *fakeQuerier) CountMessagesByConversation(_ context.Context, conversationID int64) (int64, error) {
	return f.messageCounts[conversationID], nil
}

func (f *fakeQuerier) GetUserType(_ context.Context, id int64) (string, error) {
	if userType, ok := f.userTypes[id]; ok {
		return userType, nil
	}

	return "human", nil
}
