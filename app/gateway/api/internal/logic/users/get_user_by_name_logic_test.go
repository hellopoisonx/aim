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
	require.Len(t, got.Users, 2)
	require.Equal(t, "7", got.Users[0].Id)
	require.Equal(t, "alice@example.com", got.Users[0].Email)
	require.Equal(t, "https://example.com/alice.png", got.Users[0].Avatar)
	require.Equal(t, "8", got.Users[1].Id)
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

func TestGetUserByIdLogic(t *testing.T) {
	client := &fakeLogicUserClient{}
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, client)

	got, err := NewGetUserByIdLogic(context.Background(), svcCtx).GetUserById(&types.GetUserByIdRequest{Id: 7})
	require.NoError(t, err)
	require.Equal(t, int64(7), client.id)
	require.Equal(t, int64(7), got.User.Id)
	require.Equal(t, "Alice", got.User.Nickname)
	require.Equal(t, int64(123), got.User.CreatedAt)
}

func TestGetUserByIdLogicRequiresId(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{})

	_, err := NewGetUserByIdLogic(context.Background(), svcCtx).GetUserById(&types.GetUserByIdRequest{})
	requireCodeError(t, err, int(errorx.CodeBadInput), "id is required")
}

func TestGetUserByIdLogicPreservesLogicGrpcError(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{
		err: status.Error(codes.NotFound, "user not found"),
	})

	_, err := NewGetUserByIdLogic(context.Background(), svcCtx).GetUserById(&types.GetUserByIdRequest{Id: 404})
	requireCodeError(t, err, int(errorx.CodeNotFound), "user not found")
}

func TestGetUserByIdLogicSanitizesPlainError(t *testing.T) {
	svcCtx := svc.NewServiceContextWithLogic(config.Config{}, &fakeLogicUserClient{
		err: errors.New("postgres password leaked"),
	})

	_, err := NewGetUserByIdLogic(context.Background(), svcCtx).GetUserById(&types.GetUserByIdRequest{Id: 7})
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
	id       int64
	err      error
}

func (c *fakeLogicUserClient) CreateUserInfo(context.Context, *userservice.CreateUserInfoReq, ...grpc.CallOption) (*userservice.CreateUserInfoResp, error) {
	return nil, nil
}

func (c *fakeLogicUserClient) GetUserInfo(_ context.Context, req *userservice.GetUserInfoReq, _ ...grpc.CallOption) (*userservice.GetUserInfoResp, error) {
	if c.err != nil {
		return nil, c.err
	}

	c.id = req.GetId()

	return &userservice.GetUserInfoResp{
		User: &pb.UserInfoResponse{
			Id:        7,
			Email:     "alice@example.com",
			Status:    1,
			Nickname:  "Alice",
			Avatar:    "https://example.com/alice.png",
			CreatedAt: 123,
			UpdatedAt: 456,
		},
	}, nil
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
			Avatar:    "https://example.com/alice.png",
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

func (c *fakeLogicUserClient) SearchUserInfoByNickname(_ context.Context, req *userservice.SearchUserInfoByNicknameReq, _ ...grpc.CallOption) (*userservice.SearchUserInfoByNicknameResp, error) {
	if c.err != nil {
		return nil, c.err
	}

	c.nickname = req.GetNickname()

	return &userservice.SearchUserInfoByNicknameResp{
		Users: []*pb.UserInfoResponse{
			{
				Id:        7,
				Email:     "alice@example.com",
				Status:    1,
				Nickname:  "Alice",
				Avatar:    "https://example.com/alice.png",
				CreatedAt: 123,
				UpdatedAt: 456,
			},
			{
				Id:        8,
				Email:     "alice2@example.com",
				Status:    1,
				Nickname:  "Alice",
				Avatar:    "https://example.com/alice2.png",
				CreatedAt: 124,
				UpdatedAt: 457,
			},
		},
	}, nil
}
