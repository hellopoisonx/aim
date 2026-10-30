package service

import (
	"context"
	"strings"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
)

type fakeTagStore struct {
	created      []orm.FriendTag
	replaced     []int64
	removed      int64
	friendships  []orm.Friendship
	renameErr    error
	notFoundTag  bool
	listedTags   []orm.FriendTag
	requestedIDs []int64
}

func (f *fakeTagStore) CreateFriendTag(_ context.Context, id, userID int64, name string) (orm.FriendTag, error) {
	tag := orm.FriendTag{ID: id, UserID: userID, Name: name}
	f.created = append(f.created, tag)

	return tag, nil
}

func (f *fakeTagStore) GetFriendTagByID(_ context.Context, _, _ int64) (orm.FriendTag, error) {
	if f.notFoundTag {
		return orm.FriendTag{}, dao.ErrNotFound
	}

	return orm.FriendTag{ID: 1}, nil
}

func (f *fakeTagStore) GetFriendTagsByIDs(_ context.Context, _ int64, ids []int64) ([]orm.FriendTag, error) {
	f.requestedIDs = ids
	return nil, nil
}

func (f *fakeTagStore) RenameFriendTag(_ context.Context, _, _ int64, name string) (orm.FriendTag, error) {
	if f.renameErr != nil {
		return orm.FriendTag{}, f.renameErr
	}

	return orm.FriendTag{Name: name}, nil
}

func (f *fakeTagStore) DeleteFriendTag(_ context.Context, _, _ int64) (int64, error) {
	return 1, nil
}

func (f *fakeTagStore) ReplaceFriendTags(_ context.Context, _, _ int64, tagIDs []int64) error {
	f.replaced = tagIDs
	return nil
}

func (f *fakeTagStore) RemoveFriendTagAssignment(_ context.Context, _, _, _ int64) (int64, error) {
	return f.removed, nil
}

func (f *fakeTagStore) ListFriendTagsForFriend(_ context.Context, _, _ int64) ([]orm.FriendTag, error) {
	return f.listedTags, nil
}

func (f *fakeTagStore) ListFriendsByTagID(_ context.Context, _, _ int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeTagStore) ListFriendsByTagName(_ context.Context, _ int64, _ string) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeTagStore) ListFriends(_ context.Context, _ int64) ([]orm.Friendship, error) {
	return f.friendships, nil
}

func (f *fakeTagStore) ListFriendTags(_ context.Context, _ int64) ([]orm.FriendTag, error) {
	return f.listedTags, nil
}

func (f *fakeTagStore) GetFriendshipByPair(_ context.Context, _, _ int64) (orm.Friendship, error) {
	if len(f.friendships) == 0 {
		return orm.Friendship{}, dao.ErrNotFound
	}

	return f.friendships[0], nil
}

func (f *fakeTagStore) GetFriendshipBidirectional(_ context.Context, _, _ int64) ([]orm.Friendship, error) {
	return f.friendships, nil
}

type fixedIDGen struct{ id int64 }

func (g fixedIDGen) NextID() (int64, error) { return g.id, nil }

func requireTagErr(t *testing.T, err error, wantCode int) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, wantCode, codeErr.Code)
}

func TestCreateTagValidatesName(t *testing.T) {
	svc := NewFriendshipTagService(&fakeTagStore{}, fixedIDGen{id: 1})
	ctx := context.Background()

	requireTagErr(t, mustErr(svc.CreateTag(ctx, 1, "   ")), CodeTagNameEmpty)
	requireTagErr(t, mustErr(svc.CreateTag(ctx, 1, strings.Repeat("x", friendTagNameMaxLen+1))), CodeTagNameTooLong)

	tag, err := svc.CreateTag(ctx, 1, "  work  ")
	require.NoError(t, err)
	require.Equal(t, "work", tag.Name, "tag names are trimmed")
	require.EqualValues(t, 1, tag.ID)
}

func mustErr(_ orm.FriendTag, err error) error { return err }

func TestRenameTagMapsMissingTag(t *testing.T) {
	svc := NewFriendshipTagService(&fakeTagStore{notFoundTag: true}, fixedIDGen{id: 1})

	_, err := svc.RenameTag(context.Background(), 1, 5, "new")
	requireTagErr(t, err, CodeTagNotFound)
}

func TestSetTagsRequiresAcceptedFriendship(t *testing.T) {
	svc := NewFriendshipTagService(&fakeTagStore{
		friendships: []orm.Friendship{{Status: FriendshipStatusPending}},
	}, fixedIDGen{id: 1})

	err := svc.SetTags(context.Background(), 1, 2, []int64{3})
	requireTagErr(t, err, CodeNotFriend)
}

func TestSetTagsReplacesAssignmentsForAcceptedFriend(t *testing.T) {
	store := &fakeTagStore{friendships: []orm.Friendship{{Status: FriendshipStatusAccepted}}}
	svc := NewFriendshipTagService(store, fixedIDGen{id: 1})

	require.NoError(t, svc.SetTags(context.Background(), 1, 2, []int64{4, 5}))
	require.Equal(t, []int64{4, 5}, store.requestedIDs, "tag ownership is verified before replacement")
	require.Equal(t, []int64{4, 5}, store.replaced)
}

func TestRemoveTagReportsMissingAssignment(t *testing.T) {
	store := &fakeTagStore{
		friendships: []orm.Friendship{{Status: FriendshipStatusAccepted}},
		removed:     0,
	}
	svc := NewFriendshipTagService(store, fixedIDGen{id: 1})

	removed, err := svc.RemoveTag(context.Background(), 1, 2, 9)
	require.NoError(t, err)
	require.False(t, removed)
}

func TestDeleteTagReportsWhetherRowExisted(t *testing.T) {
	svc := NewFriendshipTagService(&fakeTagStore{}, fixedIDGen{id: 1})

	deleted, err := svc.DeleteTag(context.Background(), 1, 3)
	require.NoError(t, err)
	require.True(t, deleted)
}
