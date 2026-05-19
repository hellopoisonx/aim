package userservicelogic

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeUserInfoService struct {
	info   model.UserInfo
	err    error
	search []model.UserInfo
}

func (f *fakeUserInfoService) GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return f.info, nil
}

func (f *fakeUserInfoService) GetUserInfoByEmail(ctx context.Context, email string) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return f.info, nil
}

func (f *fakeUserInfoService) GetUserInfoByNickname(ctx context.Context, nickname string) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return f.info, nil
}

func (f *fakeUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return model.UserInfo{ID: id, Email: email, Nickname: nickname, Avatar: avatar, Status: 1}, nil
}

func (f *fakeUserInfoService) UpdateUserInfoProfile(ctx context.Context, id int64, nickname, avatar string) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return f.info, nil
}

func (f *fakeUserInfoService) UpdateUserInfoStatus(ctx context.Context, id int64, status int16) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	return f.info, nil
}

func (f *fakeUserInfoService) SearchUserInfoByNickname(ctx context.Context, nickname string, limit int32) ([]model.UserInfo, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.search, nil
}

func newTestServiceContext(userSvc service.UserInfoQuerier) *svc.ServiceContext {
	return svc.NewServiceContextWithChecker(config.Config{}, service.DenyAllPermissionChecker{}, userSvc)
}

func TestCreateUserInfo(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.CreateUserInfoReq
		wantResp *pb.CreateUserInfoResp
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Email: "test@example.com", Nickname: "Test User", Status: 1},
			},
			req: &pb.CreateUserInfoReq{
				Id:       1,
				Email:    "test@example.com",
				Nickname: "Test User",
				Avatar:   "https://avatar.example.com/1.png",
			},
			wantResp: &pb.CreateUserInfoResp{
				User: &pb.UserInfoResponse{Id: 1, Email: "test@example.com", Status: 1},
			},
		},
		{
			name:     "missing id",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.CreateUserInfoReq{Id: 0, Email: "test@example.com", Nickname: "Test"},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "missing email",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.CreateUserInfoReq{Id: 1, Email: "", Nickname: "Test"},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "missing nickname",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.CreateUserInfoReq{Id: 1, Email: "test@example.com", Nickname: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "duplicate email",
			userSvc:  &fakeUserInfoService{err: service.ErrUserExists},
			req:      &pb.CreateUserInfoReq{Id: 1, Email: "test@example.com", Nickname: "Test"},
			wantCode: codes.AlreadyExists,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.CreateUserInfoReq{Id: 1, Email: "test@example.com", Nickname: "Test"},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewCreateUserInfoLogic(context.Background(), svcCtx).CreateUserInfo(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantResp.User.Id, got.User.Id)
		})
	}
}

func TestGetUserInfo(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.GetUserInfoReq
		wantResp *pb.UserInfoResponse
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Email: "test@example.com", Nickname: "Test", Status: 1},
			},
			req:      &pb.GetUserInfoReq{Id: 1},
			wantResp: &pb.UserInfoResponse{Id: 1, Email: "test@example.com"},
		},
		{
			name:     "missing id",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.GetUserInfoReq{Id: 0},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "not found",
			userSvc:  &fakeUserInfoService{err: service.ErrUserNotFound},
			req:      &pb.GetUserInfoReq{Id: 999},
			wantCode: codes.NotFound,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.GetUserInfoReq{Id: 1},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewGetUserInfoLogic(context.Background(), svcCtx).GetUserInfo(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantResp.Id, got.User.Id)
		})
	}
}

func TestGetUserInfoByEmail(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.GetUserInfoByEmailReq
		wantResp *pb.UserInfoResponse
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Email: "test@example.com", Nickname: "Test", Status: 1},
			},
			req:      &pb.GetUserInfoByEmailReq{Email: "test@example.com"},
			wantResp: &pb.UserInfoResponse{Id: 1, Email: "test@example.com"},
		},
		{
			name:     "missing email",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.GetUserInfoByEmailReq{Email: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "not found",
			userSvc:  &fakeUserInfoService{err: service.ErrUserNotFound},
			req:      &pb.GetUserInfoByEmailReq{Email: "notfound@example.com"},
			wantCode: codes.NotFound,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.GetUserInfoByEmailReq{Email: "test@example.com"},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewGetUserInfoByEmailLogic(context.Background(), svcCtx).GetUserInfoByEmail(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantResp.Email, got.User.Email)
		})
	}
}

func TestGetUserInfoByNickname(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.GetUserInfoByNicknameReq
		wantResp *pb.UserInfoResponse
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Email: "alice@example.com", Nickname: "Alice", Status: 1},
			},
			req:      &pb.GetUserInfoByNicknameReq{Nickname: "Alice"},
			wantResp: &pb.UserInfoResponse{Id: 1, Nickname: "Alice"},
		},
		{
			name:     "missing nickname",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.GetUserInfoByNicknameReq{Nickname: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "not found",
			userSvc:  &fakeUserInfoService{err: service.ErrUserNotFound},
			req:      &pb.GetUserInfoByNicknameReq{Nickname: "missing"},
			wantCode: codes.NotFound,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.GetUserInfoByNicknameReq{Nickname: "Alice"},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewGetUserInfoByNicknameLogic(context.Background(), svcCtx).GetUserInfoByNickname(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantResp.Nickname, got.User.Nickname)
		})
	}
}

func TestUpdateUserInfoProfile(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.UpdateUserInfoProfileReq
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Nickname: "New Name", Status: 1},
			},
			req:      &pb.UpdateUserInfoProfileReq{Id: 1, Nickname: "New Name", Avatar: "https://newavatar.com/1.png"},
			wantCode: codes.OK,
		},
		{
			name:     "missing id",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.UpdateUserInfoProfileReq{Id: 0, Nickname: "New"},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "missing nickname",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.UpdateUserInfoProfileReq{Id: 1, Nickname: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "not found",
			userSvc:  &fakeUserInfoService{err: service.ErrUserNotFound},
			req:      &pb.UpdateUserInfoProfileReq{Id: 999, Nickname: "New"},
			wantCode: codes.NotFound,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.UpdateUserInfoProfileReq{Id: 1, Nickname: "New"},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewUpdateUserInfoProfileLogic(context.Background(), svcCtx).UpdateUserInfoProfile(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got.User)
		})
	}
}

func TestUpdateUserInfoStatus(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.UpdateUserInfoStatusReq
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				info: model.UserInfo{ID: 1, Status: 2},
			},
			req:      &pb.UpdateUserInfoStatusReq{Id: 1, Status: 2},
			wantCode: codes.OK,
		},
		{
			name:     "missing id",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.UpdateUserInfoStatusReq{Id: 0, Status: 2},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "not found",
			userSvc:  &fakeUserInfoService{err: service.ErrUserNotFound},
			req:      &pb.UpdateUserInfoStatusReq{Id: 999, Status: 2},
			wantCode: codes.NotFound,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.UpdateUserInfoStatusReq{Id: 1, Status: 2},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewUpdateUserInfoStatusLogic(context.Background(), svcCtx).UpdateUserInfoStatus(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got.User)
		})
	}
}

func TestSearchUserInfoByNickname(t *testing.T) {
	tests := []struct {
		name     string
		userSvc  service.UserInfoQuerier
		req      *pb.SearchUserInfoByNicknameReq
		wantLen  int
		wantCode codes.Code
	}{
		{
			name: "success",
			userSvc: &fakeUserInfoService{
				search: []model.UserInfo{
					{ID: 1, Nickname: "Alice"},
					{ID: 2, Nickname: "Bob"},
				},
			},
			req:     &pb.SearchUserInfoByNicknameReq{Nickname: "Ali", Limit: 20},
			wantLen: 2,
		},
		{
			name:     "missing nickname",
			userSvc:  &fakeUserInfoService{},
			req:      &pb.SearchUserInfoByNicknameReq{Nickname: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "empty results",
			userSvc:  &fakeUserInfoService{search: []model.UserInfo{}},
			req:      &pb.SearchUserInfoByNicknameReq{Nickname: "xyz"},
			wantCode: codes.OK,
			wantLen:  0,
		},
		{
			name:     "service not configured",
			userSvc:  nil,
			req:      &pb.SearchUserInfoByNicknameReq{Nickname: "Ali"},
			wantCode: codes.Internal,
		},
		{
			name:    "limit applied",
			userSvc: &fakeUserInfoService{},
			req:     &pb.SearchUserInfoByNicknameReq{Nickname: "Ali", Limit: 200},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx := newTestServiceContext(tt.userSvc)

			got, err := NewSearchUserInfoByNicknameLogic(context.Background(), svcCtx).SearchUserInfoByNickname(tt.req)
			if tt.wantCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, status.Code(err))

				return
			}

			require.NoError(t, err)

			if tt.wantLen > 0 {
				require.Len(t, got.Users, tt.wantLen)
			}
		})
	}
}

func TestUserInfoToResponse(t *testing.T) {
	info := model.UserInfo{
		ID:       1,
		Email:    "test@example.com",
		Status:   1,
		Nickname: "Test User",
		Avatar:   "https://avatar.example.com/1.png",
	}

	resp := userInfoToResponse(info)
	require.Equal(t, int64(1), resp.Id)
	require.Equal(t, "test@example.com", resp.Email)
	require.Equal(t, "Test User", resp.Nickname)
}

func TestCreateUserInfo_InternalError(t *testing.T) {
	svcCtx := newTestServiceContext(&fakeUserInfoService{err: errors.New("database error")})
	_, err := NewCreateUserInfoLogic(context.Background(), svcCtx).CreateUserInfo(&pb.CreateUserInfoReq{
		Id:       1,
		Email:    "test@example.com",
		Nickname: "Test",
	})
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
}
