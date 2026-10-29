package conversationservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
)

type fakeUserInfoService struct {
	users map[int64]orm.UserInfo
	err   error
}

func (f *fakeUserInfoService) GetUserInfo(ctx context.Context, id int64) (orm.UserInfo, error) {
	if f.err != nil {
		return orm.UserInfo{}, f.err
	}
	if info, ok := f.users[id]; ok {
		return info, nil
	}
	return orm.UserInfo{}, errors.New("user not found")
}

func (f *fakeUserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (orm.UserInfo, error) {
	return orm.UserInfo{}, errors.New("GetUserInfoByEmail not implemented")
}

func (f *fakeUserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (orm.UserInfo, error) {
	return orm.UserInfo{}, errors.New("GetUserInfoByNickname not implemented")
}

func (f *fakeUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (orm.UserInfo, error) {
	return orm.UserInfo{}, errors.New("CreateUserInfo not implemented")
}

func (f *fakeUserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (orm.UserInfo, error) {
	return orm.UserInfo{}, errors.New("UpdateUserInfoProfile not implemented")
}

func (f *fakeUserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (orm.UserInfo, error) {
	return orm.UserInfo{}, errors.New("UpdateUserInfoStatus not implemented")
}

func (f *fakeUserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]orm.UserInfo, error) {
	return nil, errors.New("SearchUserInfoByNickname not implemented")
}
