package users

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/config"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetUserByNameLogic(t *testing.T) {
	client := &fakeLogicUserClient{}
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, client)

	got, err := NewGetUserByNameLogic(context.Background(), svcCtx).GetUserByName(&types.GetUserByNameRequest{Name: "Alice"})
	require.NoError(t, err)
	require.Equal(t, "Alice", client.nickname)
	require.Equal(t, int64(7), got.User.Id)
	require.Equal(t, "Alice", got.User.Nickname)
}

func TestGetUserByNameLogicRequiresName(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{})

	_, err := NewGetUserByNameLogic(context.Background(), svcCtx).GetUserByName(&types.GetUserByNameRequest{})
	requireCodeError(t, err, int(errorx.CodeBadInput), "name is required")
}

func TestGetUserByNameLogicPreservesLogicGrpcError(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{
		err: status.Error(codes.NotFound, "user not found"),
	})

	_, err := NewGetUserByNameLogic(context.Background(), svcCtx).GetUserByName(&types.GetUserByNameRequest{Name: "missing"})
	requireCodeError(t, err, int(errorx.CodeNotFound), "user not found")
}

func TestGetUserByNameLogicSanitizesPlainError(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{
		err: errors.New("postgres password leaked"),
	})

	_, err := NewGetUserByNameLogic(context.Background(), svcCtx).GetUserByName(&types.GetUserByNameRequest{Name: "Alice"})
	requireCodeError(t, err, int(errorx.CodeInternal), "internal error")
}

func requireCodeError(t *testing.T, err error, wantCode int, wantMsg string) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, wantCode, codeErr.Code)
	require.Equal(t, wantMsg, codeErr.Message)
}

type fakeLogicUserClient struct {
	nickname string
	err      error
}

func (c *fakeLogicUserClient) CreateUserInfo(context.Context, *userservice.CreateUserInfoReq, ...grpc.CallOption) (*userservice.CreateUserInfoResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) GetUserInfo(context.Context, *userservice.GetUserInfoReq, ...grpc.CallOption) (*userservice.GetUserInfoResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) GetUserInfoByEmail(context.Context, *userservice.GetUserInfoByEmailReq, ...grpc.CallOption) (*userservice.GetUserInfoResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) GetUserInfoByNickname(_ context.Context, req *userservice.GetUserInfoByNicknameReq, _ ...grpc.CallOption) (*userservice.GetUserInfoResp, error) {
	if c.err != nil {
		return nil, c.err
	}

	c.nickname = req.GetNickname()

	return &userservice.GetUserInfoResp{
		User: &pb.UserInfoResponse{
			Id:        7,
			Email:     "alice@example.com",
			Status:    1,
			Nickname:  "Alice",
			Avatar:    "https://implement.me",
			CreatedAt: 123,
			UpdatedAt: 456,
		},
	}, nil
}

func (c *fakeLogicUserClient) UpdateUserInfoProfile(context.Context, *userservice.UpdateUserInfoProfileReq, ...grpc.CallOption) (*userservice.UpdateUserInfoProfileResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) UpdateUserInfoStatus(context.Context, *userservice.UpdateUserInfoStatusReq, ...grpc.CallOption) (*userservice.UpdateUserInfoStatusResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) SearchUserInfoByNickname(context.Context, *userservice.SearchUserInfoByNicknameReq, ...grpc.CallOption) (*userservice.SearchUserInfoByNicknameResp, error) {
	return nil, nil
}
