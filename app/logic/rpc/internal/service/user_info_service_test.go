package service

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUserInfoQuerier struct {
	users           map[int64]model.UserInfo
	usersByEmail    map[string]model.UserInfo
	usersByNickname map[string]model.UserInfo
	createErr       error
	getByIDErr      error
	getByEmailErr   error
	getByNickErr    error
	updateProfErr   error
	updateStatusErr error
	searchResults   []model.UserInfo
	searchErr       error
}

func (f *fakeUserInfoQuerier) CreateUserInfo(ctx context.Context, arg model.CreateUserInfoParams) (model.UserInfo, error) {
	if f.createErr != nil {
		return model.UserInfo{}, f.createErr
	}

	return model.UserInfo{
		ID:       arg.ID,
		Email:    arg.Email,
		Nickname: arg.Nickname,
		Avatar:   arg.Avatar,
		Status:   1,
	}, nil
}

func (f *fakeUserInfoQuerier) GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error) {
	if f.getByIDErr != nil {
		return model.UserInfo{}, f.getByIDErr
	}

	user, ok := f.users[id]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}

	return user, nil
}

func (f *fakeUserInfoQuerier) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	if f.getByEmailErr != nil {
		return model.UserInfo{}, f.getByEmailErr
	}

	user, ok := f.usersByEmail[email]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}

	return user, nil
}

func (f *fakeUserInfoQuerier) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	if f.getByNickErr != nil {
		return model.UserInfo{}, f.getByNickErr
	}

	user, ok := f.usersByNickname[nickname]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}

	return user, nil
}

func (f *fakeUserInfoQuerier) UpdateUserInfoProfile(ctx context.Context, arg model.UpdateUserInfoProfileParams) (model.UserInfo, error) {
	if f.updateProfErr != nil {
		return model.UserInfo{}, f.updateProfErr
	}

	user, ok := f.users[arg.ID]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}

	user.Nickname = arg.Nickname
	user.Avatar = arg.Avatar

	return user, nil
}

func (f *fakeUserInfoQuerier) UpdateUserInfoStatus(ctx context.Context, arg model.UpdateUserInfoStatusParams) (model.UserInfo, error) {
	if f.updateStatusErr != nil {
		return model.UserInfo{}, f.updateStatusErr
	}

	user, ok := f.users[arg.ID]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}

	user.Status = arg.Status

	return user, nil
}

func (f *fakeUserInfoQuerier) SearchUserInfoByNickname(ctx context.Context, arg model.SearchUserInfoByNicknameParams) ([]model.UserInfo, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}

	return f.searchResults, nil
}

func TestUserInfoService_GetUserInfo(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{
			1: {ID: 1, Email: "test@example.com", Nickname: "Test User", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.GetUserInfo(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), info.ID)
	assert.Equal(t, "test@example.com", info.Email)
}

func TestUserInfoService_GetUserInfo_NotFound(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.GetUserInfo(context.Background(), 999)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserInfoService_GetUserInfo_DBError(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		getByIDErr: errors.New("database connection error"),
	}
	svc := NewUserInfoService(fq)

	_, err := svc.GetUserInfo(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

func TestUserInfoService_GetUserInfoByEmail(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		usersByEmail: map[string]model.UserInfo{
			"test@example.com": {ID: 1, Email: "test@example.com", Nickname: "Test User", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.GetUserInfoByEmail(context.Background(), "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", info.Email)
}

func TestUserInfoService_GetUserInfoByEmail_NotFound(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		usersByEmail: map[string]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.GetUserInfoByEmail(context.Background(), "notfound@example.com")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserInfoService_CreateUserInfo(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.CreateUserInfo(context.Background(), 1, "test@example.com", "Test User", "https://avatar.example.com/1.png")
	require.NoError(t, err)
	assert.Equal(t, int64(1), info.ID)
	assert.Equal(t, "test@example.com", info.Email)
	assert.Equal(t, "Test User", info.Nickname)
}

func TestUserInfoService_CreateUserInfo_DefaultAvatar(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.CreateUserInfo(context.Background(), 1, "test@example.com", "Test User", "")
	require.NoError(t, err)
	require.Equal(t, DefaultUserAvatar, info.Avatar)
}

func TestUserInfoService_CreateUserInfo_DuplicateEmail(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		createErr: &pgconn.PgError{Code: "23505"},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.CreateUserInfo(context.Background(), 1, "test@example.com", "Test User", "https://avatar.example.com/1.png")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUserExists)
}

func TestUserInfoService_CreateUserInfo_DBError(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		createErr: errors.New("database connection error"),
	}
	svc := NewUserInfoService(fq)

	_, err := svc.CreateUserInfo(context.Background(), 1, "test@example.com", "Test User", "https://avatar.example.com/1.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection error")
}

func TestUserInfoService_UpdateUserInfoProfile(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{
			1: {ID: 1, Email: "test@example.com", Nickname: "Old Nickname", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.UpdateUserInfoProfile(context.Background(), 1, "New Nickname", "https://avatar.example.com/new.png")
	require.NoError(t, err)
	assert.Equal(t, "New Nickname", info.Nickname)
}

func TestUserInfoService_UpdateUserInfoProfile_DefaultAvatar(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{
			1: {ID: 1, Email: "test@example.com", Nickname: "Old Nickname", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.UpdateUserInfoProfile(context.Background(), 1, "New Nickname", "")
	require.NoError(t, err)
	require.Equal(t, DefaultUserAvatar, info.Avatar)
}

func TestUserInfoService_GetUserInfoByNickname(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		usersByNickname: map[string]model.UserInfo{
			"Alice": {ID: 1, Email: "alice@example.com", Nickname: "Alice", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.GetUserInfoByNickname(context.Background(), "Alice")
	require.NoError(t, err)
	require.Equal(t, "Alice", info.Nickname)
}

func TestUserInfoService_GetUserInfoByNickname_NotFound(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		usersByNickname: map[string]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.GetUserInfoByNickname(context.Background(), "missing")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserInfoService_UpdateUserInfoProfile_NotFound(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.UpdateUserInfoProfile(context.Background(), 999, "New Nickname", "https://avatar.example.com/new.png")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserInfoService_UpdateUserInfoStatus(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{
			1: {ID: 1, Email: "test@example.com", Nickname: "Test User", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	info, err := svc.UpdateUserInfoStatus(context.Background(), 1, 2)
	require.NoError(t, err)
	assert.Equal(t, int16(2), info.Status)
}

func TestUserInfoService_UpdateUserInfoStatus_NotFound(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		users: map[int64]model.UserInfo{},
	}
	svc := NewUserInfoService(fq)

	_, err := svc.UpdateUserInfoStatus(context.Background(), 999, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserInfoService_SearchUserInfoByNickname(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		searchResults: []model.UserInfo{
			{ID: 1, Nickname: "Alice", Status: 1},
			{ID: 2, Nickname: "Bob", Status: 1},
		},
	}
	svc := NewUserInfoService(fq)

	results, err := svc.SearchUserInfoByNickname(context.Background(), "Alice", 20)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestUserInfoService_SearchUserInfoByNickname_DBError(t *testing.T) {
	fq := &fakeUserInfoQuerier{
		searchErr: errors.New("database error"),
	}
	svc := NewUserInfoService(fq)

	_, err := svc.SearchUserInfoByNickname(context.Background(), "Alice", 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestToGRPCError_UserNotFound(t *testing.T) {
	err := ToGRPCError(ErrUserNotFound)
	assert.Contains(t, err.Error(), "user not found")
}

func TestToGRPCError_UserExists(t *testing.T) {
	err := ToGRPCError(ErrUserExists)
	assert.Contains(t, err.Error(), "user already exists")
}

func TestToGRPCError_Unknown(t *testing.T) {
	err := ToGRPCError(errors.New("some error"))
	assert.Contains(t, err.Error(), "internal error")
}
