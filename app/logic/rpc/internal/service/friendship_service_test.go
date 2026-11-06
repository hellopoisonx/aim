package service

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFriendshipStore struct {
	direct  orm.Friendship
	err     error
	upserts []orm.Friendship
}

func (f *fakeFriendshipStore) GetFriendshipBidirectional(context.Context, int64, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendshipStore) GetFriendshipByPair(context.Context, int64, int64) (orm.Friendship, error) {
	return f.direct, f.err
}

func (f *fakeFriendshipStore) UpsertFriendship(_ context.Context, userID, friendID int64, status string) (orm.Friendship, error) {
	record := orm.Friendship{UserID: userID, FriendID: friendID, Status: status}
	f.upserts = append(f.upserts, record)
	return record, nil
}

func (f *fakeFriendshipStore) ListPendingFriendApplications(context.Context, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendshipStore) ListFriends(context.Context, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendshipStore) ListFriendsByTagID(context.Context, int64, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendshipStore) ListFriendsByTagName(context.Context, int64, string) ([]orm.Friendship, error) {
	return nil, nil
}

func TestFriendshipServiceAcceptWritesBothDirections(t *testing.T) {
	store := &fakeFriendshipStore{direct: orm.Friendship{UserID: 2, FriendID: 1, Status: FriendshipStatusPending}}

	accepted, err := NewFriendshipService(store).AcceptFriend(context.Background(), 2, 1)

	require.NoError(t, err)
	assert.Equal(t, orm.Friendship{UserID: 2, FriendID: 1, Status: FriendshipStatusAccepted}, accepted)
	assert.Equal(t, []orm.Friendship{
		{UserID: 2, FriendID: 1, Status: FriendshipStatusAccepted},
		{UserID: 1, FriendID: 2, Status: FriendshipStatusAccepted},
	}, store.upserts)
}

func TestFriendshipServiceAcceptRequiresPending(t *testing.T) {
	store := &fakeFriendshipStore{direct: orm.Friendship{UserID: 2, FriendID: 1, Status: FriendshipStatusAccepted}}

	_, err := NewFriendshipService(store).AcceptFriend(context.Background(), 2, 1)

	require.ErrorIs(t, err, ErrNotPending)
	assert.Empty(t, store.upserts)
}

func TestFriendshipServiceRejectBlocksRequester(t *testing.T) {
	store := &fakeFriendshipStore{direct: orm.Friendship{UserID: 2, FriendID: 1, Status: FriendshipStatusPending}}

	blocked, err := NewFriendshipService(store).RejectFriend(context.Background(), 2, 1)

	require.NoError(t, err)
	assert.Equal(t, FriendshipStatusBlocked, blocked.Status)
}

func TestFriendshipServiceMissingRequest(t *testing.T) {
	store := &fakeFriendshipStore{err: dao.ErrNotFound}

	_, err := NewFriendshipService(store).RejectFriend(context.Background(), 2, 1)

	require.ErrorIs(t, err, ErrFriendNotFound)
}
