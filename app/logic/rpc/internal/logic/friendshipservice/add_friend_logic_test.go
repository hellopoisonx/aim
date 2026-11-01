package friendshipservicelogic

import (
	"context"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockUserInfoService mocks the user info service for testing.
type MockUserInfoService struct {
	mock.Mock
}

func (m *MockUserInfoService) GetUserInfo(ctx context.Context, id int64) (orm.UserInfo, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error) {
	args := m.Called(ctx, email)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error) {
	args := m.Called(ctx, nickname)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error) {
	args := m.Called(ctx, id, email, nickname, avatar)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error) {
	args := m.Called(ctx, id, nickname, avatar)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error) {
	args := m.Called(ctx, id, status)
	return args.Get(0).(orm.UserInfo), args.Error(1)
}

func (m *MockUserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]orm.UserInfo, error) {
	args := m.Called(ctx, nickname, limit)
	return args.Get(0).([]orm.UserInfo), args.Error(1)
}

func newTestCtx() context.Context {
	return context.Background()
}

type fakeFriendDAO struct {
	bidirectional []orm.Friendship
	direct        orm.Friendship
	directErr     error
	upserted      orm.Friendship
}

func (f *fakeFriendDAO) GetFriendshipBidirectional(context.Context, int64, int64) ([]orm.Friendship, error) {
	return f.bidirectional, nil
}

func (f *fakeFriendDAO) GetFriendshipByPair(context.Context, int64, int64) (orm.Friendship, error) {
	return f.direct, f.directErr
}

func (f *fakeFriendDAO) UpsertFriendship(_ context.Context, userID, friendID int64, status string) (orm.Friendship, error) {
	if f.upserted.UserID != 0 {
		return f.upserted, nil
	}
	return newFriendship(userID, friendID, status), nil
}

func (f *fakeFriendDAO) ListPendingFriendApplications(context.Context, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendDAO) ListFriends(context.Context, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendDAO) ListFriendsByTagID(context.Context, int64, int64) ([]orm.Friendship, error) {
	return nil, nil
}

func (f *fakeFriendDAO) ListFriendsByTagName(context.Context, int64, string) ([]orm.Friendship, error) {
	return nil, nil
}

func newFriendship(userID int64, friendID int64, status string) orm.Friendship {
	now := time.UnixMilli(123)
	return orm.Friendship{
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
	assert.Equal(t, service.CodeSelfAdd, codeErr.Code)
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
	mockUserSvc.On("GetUserInfo", ctx, int64(999)).Return(orm.UserInfo{}, service.ErrUserNotFound)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		FriendshipDAO:   nil,
	}

	l := NewAddFriendLogic(ctx, svcCtx)

	_, err := l.AddFriend(&pb.AddFriendReq{
		UserId:   1,
		FriendId: 999,
	})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	// The shared auth sentinel for "user not found" keeps its own code.
	assert.Equal(t, errorx.CodeUserNotFound, codeErr.Code)
}

func TestAddFriend_DatabaseNotConfigured(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(orm.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		FriendshipDAO:   nil,
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
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(orm.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		FriendshipDAO: &fakeFriendDAO{
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
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(orm.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		FriendshipDAO: &fakeFriendDAO{
			directErr: dao.ErrNotFound,
			upserted:  newFriendship(1, 2, service.FriendshipStatusPending),
		},
	}

	resp, err := NewAddFriendLogic(ctx, svcCtx).AddFriend(&pb.AddFriendReq{UserId: 1, FriendId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp.GetFriendship())
	assert.Equal(t, service.FriendshipStatusPending, resp.GetFriendship().GetStatus())
}

func TestAddFriend_BlockedRelationship(t *testing.T) {
	ctx := newTestCtx()
	mockUserSvc := new(MockUserInfoService)
	mockUserSvc.On("GetUserInfo", ctx, int64(2)).Return(orm.UserInfo{ID: 2, Nickname: "user2"}, nil)

	svcCtx := &svc.ServiceContext{
		UserInfoService: mockUserSvc,
		FriendshipDAO: &fakeFriendDAO{
			bidirectional: []orm.Friendship{{UserID: 2, FriendID: 1, Status: "blocked"}},
		},
	}

	_, err := NewAddFriendLogic(ctx, svcCtx).AddFriend(&pb.AddFriendReq{UserId: 1, FriendId: 2})

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	// The blocked relationship keeps its precise domain code.
	assert.Equal(t, service.CodeBlocked, codeErr.Code)
}
