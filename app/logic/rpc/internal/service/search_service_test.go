package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/pgtypes"
	"github.com/stretchr/testify/require"
)

// fakeSearchStore records the queries the service issues and returns canned rows.
type fakeSearchStore struct {
	userRows    []dao.UserSearchRow
	friendRows  []dao.FriendSearchRow
	convRows    []dao.ConversationSearchRow
	messageRows []dao.MessageSearchRow
	err         error

	lastLimit           int32
	lastMessageQuery    string
	lastConversationID  int64
	lastCursorCreatedAt *time.Time
	usersQueried        bool
	friendsQueried      bool
	convsQueried        bool
	messagesQueried     bool
}

func (f *fakeSearchStore) SearchUserInfoByQuery(_ context.Context, _ string, limit int32) ([]dao.UserSearchRow, error) {
	f.usersQueried, f.lastLimit = true, limit
	return f.userRows, f.err
}

func (f *fakeSearchStore) SearchFriendsByQuery(_ context.Context, _ int64, _ string, limit int32) ([]dao.FriendSearchRow, error) {
	f.friendsQueried, f.lastLimit = true, limit
	return f.friendRows, f.err
}

func (f *fakeSearchStore) SearchConversationsByName(_ context.Context, _ int64, _ string, limit int32) ([]dao.ConversationSearchRow, error) {
	f.convsQueried, f.lastLimit = true, limit
	return f.convRows, f.err
}

func (f *fakeSearchStore) SearchMessagesGlobal(_ context.Context, _ int64, query string, cursorCreatedAt *time.Time, _ int64, limit int32) ([]dao.MessageSearchRow, error) {
	f.messagesQueried, f.lastLimit, f.lastMessageQuery, f.lastCursorCreatedAt = true, limit, query, cursorCreatedAt
	return f.messageRows, f.err
}

func (f *fakeSearchStore) SearchMessagesInConversation(_ context.Context, _, conversationID int64, query string, cursorCreatedAt *time.Time, _ int64, limit int32) ([]dao.MessageSearchRow, error) {
	f.messagesQueried, f.lastLimit, f.lastMessageQuery = true, limit, query
	f.lastConversationID, f.lastCursorCreatedAt = conversationID, cursorCreatedAt
	return f.messageRows, f.err
}

func (f *fakeSearchStore) GetConversation(context.Context, int64) (orm.Conversation, error) {
	return orm.Conversation{}, f.err
}

func (f *fakeSearchStore) GetConversationMembers(context.Context, int64) ([]orm.ConversationMember, error) {
	return nil, f.err
}

func (f *fakeSearchStore) GetUserInfoByID(_ context.Context, id int64) (orm.UserInfo, error) {
	return orm.UserInfo{ID: id, Nickname: "friend-" + itoa(id)}, f.err
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}

	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}

	return string(digits)
}

func TestSearchRejectsEmptyQuery(t *testing.T) {
	store := &fakeSearchStore{}
	_, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "   ", Scopes: nil, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 20})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, CodeSearchQueryEmpty, codeErr.Code)
	require.False(t, store.usersQueried, "an empty query must not hit the store")
}

func TestSearchAllScopesWhenNoneRequested(t *testing.T) {
	store := &fakeSearchStore{userRows: []dao.UserSearchRow{{UserInfo: orm.UserInfo{ID: 7}}}}
	results, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "alice", Scopes: nil, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 0})
	require.NoError(t, err)

	require.True(t, store.usersQueried)
	require.True(t, store.friendsQueried)
	require.True(t, store.convsQueried)
	require.True(t, store.messagesQueried)
	require.Len(t, results.Users, 1)
	// The message search fetches limit+1 rows to detect has_more.
	require.EqualValues(t, defaultSearchLimit+1, store.lastLimit)
}

func TestSearchHonoursRequestedScope(t *testing.T) {
	store := &fakeSearchStore{}
	_, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "alice", Scopes: []string{"messages"}, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 5})
	require.NoError(t, err)

	require.False(t, store.usersQueried)
	require.True(t, store.messagesQueried)
}

func TestSearchClampsLimit(t *testing.T) {
	store := &fakeSearchStore{}
	_, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "alice", Scopes: []string{"users"}, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 500})
	require.NoError(t, err)
	require.Equal(t, int32(maxSearchLimit), store.lastLimit)
}

func TestSearchRoutesMessagesToConversationScope(t *testing.T) {
	store := &fakeSearchStore{}
	_, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "alice", Scopes: []string{"messages"}, ConversationID: 99, CursorCreatedAt: 1700000000, CursorID: 5, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, int64(99), store.lastConversationID)
	require.NotNil(t, store.lastCursorCreatedAt)
	require.Equal(t, int64(1700000000), store.lastCursorCreatedAt.UnixMilli(), "cursor is passed as unix milliseconds")
}

func TestSearchFriendsEnrichesUserInfo(t *testing.T) {
	store := &fakeSearchStore{friendRows: []dao.FriendSearchRow{{UserID: 1, FriendID: 2, Snippet: "fr<mark>iend</mark>"}}}
	results, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "frie", Scopes: []string{"friends"}, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 20})
	require.NoError(t, err)

	require.Len(t, results.Friends, 1)
	require.Equal(t, "friend-2", results.Friends[0].UserInfo.Nickname)
	require.Equal(t, FriendshipStatusAccepted, results.Friends[0].Friendship.Status)
}

func TestSearchMessagesMapsContentAndPagination(t *testing.T) {
	createdAt := time.UnixMilli(1700000000123)
	store := &fakeSearchStore{messageRows: []dao.MessageSearchRow{
		{Message: orm.Message{ID: 11, ConversationID: 4, SenderID: 2, MessageType: "text",
			Content: pgtypes.JSON(`{"text":"hi"}`), CreatedAt: createdAt}, Snippet: "h<mark>i</mark>"},
	}}

	results, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "hi", Scopes: []string{"messages"}, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 20})
	require.NoError(t, err)
	require.Len(t, results.Messages, 1)
	require.Equal(t, int64(11), results.Messages[0].Message.ID)
	require.Equal(t, "h<mark>i</mark>", results.Messages[0].Snippet)
}

func TestSearchPropagatesStoreError(t *testing.T) {
	store := &fakeSearchStore{err: errors.New("postgres down")}
	_, err := NewSearchService(store).Search(context.Background(), SearchParams{UserID: 1, Query: "alice", Scopes: []string{"users"}, ConversationID: 0, CursorCreatedAt: 0, CursorID: 0, Limit: 20})
	require.Error(t, err)
}
