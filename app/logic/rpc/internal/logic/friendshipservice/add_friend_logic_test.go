package friendshipservicelogic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockUserInfoService mocks the user info service for testing.
type MockUserInfoService struct {
	mock.Mock
}

func (m *MockUserInfoService) GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	args := m.Called(ctx, nickname)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error) {
	args := m.Called(ctx, id, email, nickname, avatar)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (model.UserInfo, error) {
	args := m.Called(ctx, id, nickname, avatar)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (model.UserInfo, error) {
	args := m.Called(ctx, id, status)
	return args.Get(0).(model.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]model.UserInfo, error) {
	args := m.Called(ctx, nickname, limit)
	return args.Get(0).([]model.UserInfo), args.Error(1)
}

func newTestCtx() context.Context {
	return context.Background()
}

type fakeFriendDB struct {
	bidirectional []model.GetFriendshipBidirectionalRow
	direct        model.Friendship
	directErr     error
	upserted      model.Friendship
}

func (f *fakeFriendDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeFriendDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &fakeFriendRows{rows: f.bidirectional}, nil
}

func (f *fakeFriendDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	switch {
	case strings.Contains(query, "GetFriendshipByPair"):
		return fakeFriendRow{friendship: f.direct, err: f.directErr}
	case strings.Contains(query, "UpsertFriendship"):
		friendship := f.upserted
		if friendship.UserID == 0 && len(args) >= 3 {
			friendship = newFriendship(args[0].(int64), args[1].(int64), args[2].(string))
		}

		return fakeFriendRow{friendship: friendship}
	default:
		return fakeFriendRow{err: pgx.ErrNoRows}
	}
}

type fakeFriendRow struct {
	friendship model.Friendship
	err        error
}

func (r fakeFriendRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	*(dest[0].(*int64)) = r.friendship.UserID
	*(dest[1].(*int64)) = r.friendship.FriendID
	*(dest[2].(*string)) = r.friendship.Status
	*(dest[3].(*pgtype.Timestamptz)) = r.friendship.CreatedAt
	*(dest[4].(*pgtype.Timestamptz)) = r.friendship.UpdatedAt

	return nil
}

type fakeFriendRows struct {
	rows []model.GetFriendshipBidirectionalRow
	idx  int
}

func (r *fakeFriendRows) Close() {}

func (r *fakeFriendRows) Err() error { return nil }

func (r *fakeFriendRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeFriendRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeFriendRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}

	r.idx++

	return true
}

func (r *fakeFriendRows) Scan(dest ...any) error {
	row := r.rows[r.idx-1]
	*(dest[0].(*int64)) = row.UserID
	*(dest[1].(*int64)) = row.FriendID
	*(dest[2].(*string)) = row.Status

	return nil
}

func (r *fakeFriendRows) Values() ([]any, error) { return nil, nil }

func (r *fakeFriendRows) RawValues() [][]byte { return nil }

func (r *fakeFriendRows) Conn() *pgx.Conn { return nil }

func newFriendship(userID int64, friendID int64, status string) model.Friendship {
	now := pgtype.Timestamptz{Time: time.UnixMilli(123), Valid: true}

	return model.Friendship{
		UserID:    userID,
		FriendID:  friendID,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestAddFriend_SelfAdd(t *testing.T) {
	ctx := newTestCtx()
	svcCtx := &svc.ServiceContext{}

	l := NewAddFriendLogic(ctx, svcCtx)

	// User trying to add themselves
	_, err := l.AddFriend(&pb.AddFriendReq{
		UserId:   1,
		FriendId: 1,
	})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeBadInput, codeErr.Code)
}

func TestAddFriend_InvalidIDs(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		friendID int64
	}{
		{
			name:     "user_id zero",
			userID:   0,
			friendID: 2,
		},
		{
			name:     "friend_id zero",
			userID:   1,
			friendID: 0,
		},
		{
			name:     "user_id negative",
			userID:   -1,
			friendID: 2,
		},
		{
			name:     "friend_id negative",
			userID:   1,
			friendID: -2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestCtx()
			svcCtx := &svc.ServiceContext{}

			l := NewAddFriendLogic(ctx, svcCtx)

			_, err := l.AddFriend(&pb.AddFriendReq{
				UserId:   tt.userID,
				FriendId: tt.friendID,
			})

			var codeErr *errorx.CodeError
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, errorx.CodeBadInput, codeErr.Code)
		})
	}
}

func TestAddFriend_UserServiceNotConfigured(t *testing.T) {
	ctx := newTestCtx()
	svcCtx := &svc.ServiceContext{
		UserInfoService: nil,
	}

	l := NewAddFriendLogic(ctx, svcCtx)

	_, err := l.AddFriend(&pb.AddFriendReq{
		UserId:   1,
		FriendId: 2,
	})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeInternal, codeErr.Code)
}

func TestAddFriend_TargetUserNotFound(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(999)).Return(model.UserInfo{}, service.ErrUserNotFound)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		DB:              nil,
	}

	l := NewAddFriendLogic(ctx, svcCtx)

	_, err := l.AddFriend(&pb.AddFriendReq{
		UserId:   1,
		FriendId: 999,
	})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeNotFound, codeErr.Code)
}

func TestAddFriend_DatabaseNotConfigured(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(model.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		DB:              nil,
	}

	l := NewAddFriendLogic(ctx, svcCtx)

	_, err := l.AddFriend(&pb.AddFriendReq{
		UserId:   1,
		FriendId: 2,
	})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeInternal, codeErr.Code)
}

func TestAddFriend_ReturnsExistingAcceptedFriendship(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(model.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		DB: &fakeFriendDB{
			direct: newFriendship(1, 2, "accepted"),
		},
	}

	resp, err := NewAddFriendLogic(ctx, svcCtx).AddFriend(&pb.AddFriendReq{UserId: 1, FriendId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp.GetFriendship())
	assert.Equal(t, "accepted", resp.GetFriendship().GetStatus())
	assert.Equal(t, int64(123), resp.GetFriendship().GetCreatedAt())
}

func TestAddFriend_CreatesPendingFriendship(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(model.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		DB: &fakeFriendDB{
			directErr: pgx.ErrNoRows,
			upserted:  newFriendship(1, 2, FriendshipStatusPending),
		},
	}

	resp, err := NewAddFriendLogic(ctx, svcCtx).AddFriend(&pb.AddFriendReq{UserId: 1, FriendId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp.GetFriendship())
	assert.Equal(t, FriendshipStatusPending, resp.GetFriendship().GetStatus())
}

func TestAddFriend_BlockedRelationship(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(model.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		DB: &fakeFriendDB{
			bidirectional: []model.GetFriendshipBidirectionalRow{{UserID: 2, FriendID: 1, Status: "blocked"}},
		},
	}

	_, err := NewAddFriendLogic(ctx, svcCtx).AddFriend(&pb.AddFriendReq{UserId: 1, FriendId: 2})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeForbidden, codeErr.Code)
}

func TestFriendshipToGRPCError(t *testing.T) {
	tests := []struct {
		name     string
		inputErr error
		expected int
	}{
		{
			name:     "user not found",
			inputErr: service.ErrUserNotFound,
			expected: errorx.CodeNotFound,
		},
		{
			name:     "self add",
			inputErr: service.ErrSelfAdd,
			expected: errorx.CodeBadInput,
		},
		{
			name:     "blocked",
			inputErr: service.ErrBlocked,
			expected: errorx.CodeForbidden,
		},
		{
			name:     "unknown error",
			inputErr: errors.New("some unknown error"),
			expected: errorx.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FriendshipToGRPCError(tt.inputErr)

			var codeErr *errorx.CodeError
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, tt.expected, codeErr.Code)
		})
	}
}
