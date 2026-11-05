package searchservicelogic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/pgtypes"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeSearchStore overrides the queries the tests exercise; anything else
// panics on the nil embedded interface, which is what an unexpected call
// should do.
type fakeSearchStore struct {
	service.SearchStore

	userRows    []dao.UserSearchRow
	friendRows  []dao.FriendSearchRow
	convRows    []dao.ConversationSearchRow
	messageRows []dao.MessageSearchRow
	err         error

	queries        []string
	lastQuery      string
	lastLimit      int32
	lastCursorTime *time.Time
	lastCursorID   int64
	lastConvID     int64
}

func (f *fakeSearchStore) SearchUserInfoByQuery(_ context.Context, query string, limit int32) ([]dao.UserSearchRow, error) {
	f.record("users", query, limit)
	return f.userRows, f.err
}

func (f *fakeSearchStore) SearchFriendsByQuery(_ context.Context, _ int64, query string, limit int32) ([]dao.FriendSearchRow, error) {
	f.record("friends", query, limit)
	return f.friendRows, f.err
}

func (f *fakeSearchStore) SearchConversationsByName(_ context.Context, _ int64, query string, limit int32) ([]dao.ConversationSearchRow, error) {
	f.record("conversations", query, limit)
	return f.convRows, f.err
}

func (f *fakeSearchStore) SearchMessagesGlobal(_ context.Context, _ int64, query string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]dao.MessageSearchRow, error) {
	f.record("messages", query, limit)
	f.lastCursorTime, f.lastCursorID = cursorCreatedAt, cursorID
	return f.messageRows, f.err
}

func (f *fakeSearchStore) SearchMessagesInConversation(_ context.Context, _, conversationID int64, query string, cursorCreatedAt *time.Time, cursorID int64, limit int32) ([]dao.MessageSearchRow, error) {
	f.record("messages", query, limit)
	f.lastConvID = conversationID
	f.lastCursorTime, f.lastCursorID = cursorCreatedAt, cursorID
	return f.messageRows, f.err
}

func (f *fakeSearchStore) GetUserInfoByID(_ context.Context, id int64) (orm.UserInfo, error) {
	return orm.UserInfo{
		ID:        id,
		Nickname:  "user-" + itoa(id),
		CreatedAt: fakeUserCreated,
		UpdatedAt: fakeUserCreated,
	}, nil
}

func (f *fakeSearchStore) record(kind, query string, limit int32) {
	f.queries = append(f.queries, kind)
	f.lastQuery, f.lastLimit = query, limit
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

var fakeUserCreated = time.UnixMilli(1_600_000_000_000)

func searchSvcCtx(store service.SearchStore) *svc.ServiceContext {
	return &svc.ServiceContext{SearchService: service.NewSearchService(store)}
}

func requireBizCode(t *testing.T, err error, code int, message string) {
	t.Helper()
	require.Error(t, err)

	codeErr := errorx.FromGRPCError(err)
	require.NotNil(t, codeErr, "error must carry an AIM biz code: %v", err)
	require.Equal(t, code, codeErr.Code)
	if message != "" {
		// Error() keeps the caller-facing message; FromGRPCError rewrites it
		// to "internal error" for infrastructure gRPC codes.
		require.Contains(t, err.Error(), message)
	}
}

func TestUnifiedSearchGuards(t *testing.T) {
	store := &fakeSearchStore{}

	tests := []struct {
		name     string
		svcCtx   *svc.ServiceContext
		req      *pb.UnifiedSearchReq
		code     int
		grpcCode codes.Code
		message  string
	}{
		{
			name:     "zero user id",
			svcCtx:   searchSvcCtx(store),
			req:      &pb.UnifiedSearchReq{Query: "alice"},
			code:     errorx.CodeBadInput,
			grpcCode: codes.InvalidArgument,
			message:  "user_id is required and must be positive",
		},
		{
			name:     "negative user id",
			svcCtx:   searchSvcCtx(store),
			req:      &pb.UnifiedSearchReq{UserId: -1, Query: "alice"},
			code:     errorx.CodeBadInput,
			grpcCode: codes.InvalidArgument,
			message:  "user_id is required and must be positive",
		},
		{
			name:     "unconfigured search service",
			svcCtx:   &svc.ServiceContext{},
			req:      &pb.UnifiedSearchReq{UserId: 3, Query: "alice"},
			code:     errorx.CodeInternal,
			grpcCode: codes.Internal,
			message:  "search service not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUnifiedSearchLogic(context.Background(), tt.svcCtx).UnifiedSearch(tt.req)
			requireBizCode(t, err, tt.code, tt.message)
			require.Equal(t, tt.grpcCode, status.Code(err))
		})
	}

	require.Empty(t, store.queries, "guards must short-circuit before the store")
}

func TestUnifiedSearchForwardsRequestToService(t *testing.T) {
	cursorAt := time.UnixMilli(1_700_000_000_000).UTC()

	tests := []struct {
		name           string
		req            *pb.UnifiedSearchReq
		wantQueries    []string
		wantQuery      string
		wantLimit      int32
		wantConvID     int64
		wantCursorTime *time.Time
	}{
		{
			name:        "query is trimmed and limit defaults",
			req:         &pb.UnifiedSearchReq{UserId: 3, Query: "  alice  ", Scopes: []string{"messages"}},
			wantQueries: []string{"messages"},
			wantQuery:   "alice",
			wantLimit:   21,
		},
		{
			name: "empty scopes search everything and limit is capped",
			req:  &pb.UnifiedSearchReq{UserId: 3, Query: "alice", Limit: 500},
			wantQueries: []string{
				"users", "friends", "conversations", "messages",
			},
			wantQuery: "alice",
			wantLimit: 101,
		},
		{
			name: "scope and conversation id restrict message search",
			req: &pb.UnifiedSearchReq{
				UserId: 3, Query: "alice", Scopes: []string{"messages"},
				ConversationId: 11, CursorCreatedAt: cursorAt.UnixMilli(), CursorId: 900, Limit: 5,
			},
			wantQueries:    []string{"messages"},
			wantQuery:      "alice",
			wantLimit:      6,
			wantConvID:     11,
			wantCursorTime: &cursorAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeSearchStore{}
			_, err := NewUnifiedSearchLogic(context.Background(), searchSvcCtx(store)).UnifiedSearch(tt.req)
			require.NoError(t, err)

			require.Equal(t, tt.wantQueries, store.queries)
			require.Equal(t, tt.wantQuery, store.lastQuery)
			require.Equal(t, tt.wantLimit, store.lastLimit)
			require.Equal(t, tt.wantConvID, store.lastConvID)
			if tt.wantCursorTime == nil {
				require.Nil(t, store.lastCursorTime)
			} else {
				require.NotNil(t, store.lastCursorTime)
				require.True(t, tt.wantCursorTime.Equal(*store.lastCursorTime))
			}
		})
	}
}

func TestUnifiedSearchMapsResults(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)
	clientMsgID := "c-1"

	store := &fakeSearchStore{
		userRows: []dao.UserSearchRow{{
			UserInfo: orm.UserInfo{
				ID: 9, Email: "u@example.com", Status: 1, Nickname: "alice", Avatar: "a.png",
				CreatedAt: created, UpdatedAt: created,
			},
			Snippet: "alice snippet",
		}},
		friendRows: []dao.FriendSearchRow{
			{UserID: 3, FriendID: 4, Snippet: "friend snippet"},
			// A row whose stored owner is not the caller exercises the swap branch.
			{UserID: 8, FriendID: 3, Snippet: "reverse snippet"},
		},
		convRows: []dao.ConversationSearchRow{{
			Conversation: orm.Conversation{
				ID: 11, ConversationType: "group", IsActive: true,
				Name: "team", Avatar: "t.png", CreatorID: 5, CreatedAt: created,
			},
			Snippet: "conv snippet",
		}},
		messageRows: []dao.MessageSearchRow{
			{
				Message: orm.Message{
					ID: 101, ConversationID: 11, SenderID: 3, MessageType: "text",
					Content: pgtypes.JSON(`{"b": 2, "a": 1}`), ClientMsgID: &clientMsgID, CreatedAt: created,
				},
				Snippet: "msg snippet",
			},
			{
				Message: orm.Message{
					ID: 100, ConversationID: 11, SenderID: 4, MessageType: "text",
					Content: pgtypes.JSON("not json"), CreatedAt: created,
				},
			},
		},
	}

	resp, err := NewUnifiedSearchLogic(context.Background(), searchSvcCtx(store)).UnifiedSearch(
		&pb.UnifiedSearchReq{UserId: 3, Query: "alice", Limit: 5})
	require.NoError(t, err)

	require.Equal(t, []*pb.SearchUserResult{{
		User: &pb.UserInfoResponse{
			Id: 9, Email: "u@example.com", Status: 1, Nickname: "alice", Avatar: "a.png",
			CreatedAt: created.UnixMilli(), UpdatedAt: created.UnixMilli(),
		},
		Snippet: "alice snippet",
	}}, resp.GetUsers())

	require.Equal(t, []*pb.SearchFriendResult{
		{
			Friendship: &pb.FriendshipResponse{UserId: 3, FriendId: 4, Status: service.FriendshipStatusAccepted},
			User: &pb.UserInfoResponse{
				Id: 4, Nickname: "user-4",
				CreatedAt: fakeUserCreated.UnixMilli(), UpdatedAt: fakeUserCreated.UnixMilli(),
			},
			Snippet: "friend snippet",
		},
		{
			Friendship: &pb.FriendshipResponse{UserId: 3, FriendId: 8, Status: service.FriendshipStatusAccepted},
			User: &pb.UserInfoResponse{
				Id: 3, Nickname: "user-3",
				CreatedAt: fakeUserCreated.UnixMilli(), UpdatedAt: fakeUserCreated.UnixMilli(),
			},
			Snippet: "reverse snippet",
		},
	}, resp.GetFriends())

	require.Equal(t, []*pb.SearchConversationResult{{
		Conversation: &pb.ConversationResponse{
			Id: 11, ConversationType: "group", IsActive: true,
			Name: "team", Avatar: "t.png", CreatorId: 5, CreatedAt: created.UnixMilli(),
		},
		Snippet: "conv snippet",
	}}, resp.GetConversations())

	require.Equal(t, []*pb.SearchMessageResult{
		{
			Message: &pb.MessageItem{
				Id: 101, ConversationId: 11, SenderId: 3, MessageType: "text",
				Content: `{"a":1,"b":2}`, ClientMsgId: "c-1", CreatedAt: created.UnixMilli(),
			},
			Snippet: "msg snippet",
		},
		{
			Message: &pb.MessageItem{
				Id: 100, ConversationId: 11, SenderId: 4, MessageType: "text",
				Content: "not json", CreatedAt: created.UnixMilli(),
			},
		},
	}, resp.GetMessages())

	require.False(t, resp.GetHasMore())
	require.Zero(t, resp.GetNextCursorCreatedAt())
	require.Zero(t, resp.GetNextCursorId())
}

func TestUnifiedSearchReportsHasMore(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)

	// limit 2 -> the service fetches 3 rows; three rows means a further page.
	store := &fakeSearchStore{messageRows: []dao.MessageSearchRow{
		{Message: orm.Message{ID: 103, CreatedAt: created}},
		{Message: orm.Message{ID: 102, CreatedAt: created}},
		{Message: orm.Message{ID: 101, CreatedAt: created}},
	}}

	resp, err := NewUnifiedSearchLogic(context.Background(), searchSvcCtx(store)).UnifiedSearch(
		&pb.UnifiedSearchReq{UserId: 3, Query: "alice", Scopes: []string{"messages"}, Limit: 2})
	require.NoError(t, err)

	require.Len(t, resp.GetMessages(), 2)
	require.True(t, resp.GetHasMore())
	require.Equal(t, created.UnixMilli(), resp.GetNextCursorCreatedAt())
	require.Equal(t, int64(102), resp.GetNextCursorId())
}

func TestUnifiedSearchErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		store    *fakeSearchStore
		query    string
		code     int
		grpcCode codes.Code
		message  string
	}{
		{
			name:     "deliberate biz errors keep their code",
			store:    &fakeSearchStore{},
			query:    "   ",
			code:     service.CodeSearchQueryEmpty,
			grpcCode: codes.InvalidArgument,
			message:  "search query must not be empty",
		},
		{
			name:     "infrastructure errors are sanitised",
			store:    &fakeSearchStore{err: errors.New("dial tcp: connection refused")},
			query:    "alice",
			code:     errorx.CodeInternal,
			grpcCode: codes.Internal,
			message:  "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUnifiedSearchLogic(context.Background(), searchSvcCtx(tt.store)).UnifiedSearch(
				&pb.UnifiedSearchReq{UserId: 3, Query: tt.query})
			require.Equal(t, tt.grpcCode, status.Code(err))
			requireBizCode(t, err, tt.code, tt.message)
		})
	}
}
