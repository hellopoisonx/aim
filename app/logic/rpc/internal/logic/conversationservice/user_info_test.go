package conversationservicelogic

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
)

type fakeUserInfoService struct {
	users map[int64]model.UserInfo
	err   error
}

func (f *fakeUserInfoService) GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}
	if info, ok := f.users[id]; ok {
		return info, nil
	}
	return model.UserInfo{}, errors.New("user not found")
}

func (f *fakeUserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	return model.UserInfo{}, errors.New("GetUserInfoByEmail not implemented")
}

func (f *fakeUserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	return model.UserInfo{}, errors.New("GetUserInfoByNickname not implemented")
}

func (f *fakeUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error) {
	return model.UserInfo{}, errors.New("CreateUserInfo not implemented")
}

func (f *fakeUserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (model.UserInfo, error) {
	return model.UserInfo{}, errors.New("UpdateUserInfoProfile not implemented")
}

func (f *fakeUserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (model.UserInfo, error) {
	return model.UserInfo{}, errors.New("UpdateUserInfoStatus not implemented")
}

func (f *fakeUserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]model.UserInfo, error) {
	return nil, errors.New("SearchUserInfoByNickname not implemented")
}
