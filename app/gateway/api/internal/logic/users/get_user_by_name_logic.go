// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package users

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserByNameLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserByNameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserByNameLogic {
	return &GetUserByNameLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserByNameLogic) GetUserByName(req *types.GetUserByNameRequest) (resp *types.GetUserByNameResponse, err error) {
	if req.Name == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
	}

	if l.svcCtx.LogicUserClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicUserClient.GetUserInfoByNickname(l.ctx, &userservice.GetUserInfoByNicknameReq{
		Nickname: req.Name,
	})
	if err != nil {
		return nil, l.sanitizeLogicRPCError("get user by name", err)
	}

	return &types.GetUserByNameResponse{
		User: userInfoFromRPC(rpcResp.GetUser()),
	}, nil
}

func userInfoFromRPC(user *pb.UserInfoResponse) types.UserInfo {
	if user == nil {
		return types.UserInfo{}
	}

	return types.UserInfo{
		Id:        user.GetId(),
		Email:     user.GetEmail(),
		Status:    user.GetStatus(),
		Nickname:  user.GetNickname(),
		Avatar:    user.GetAvatar(),
		CreatedAt: user.GetCreatedAt(),
		UpdatedAt: user.GetUpdatedAt(),
	}
}

func sanitizeLogicRPCError(logger logicRPCErrorLogger, operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	logger.Errorf("logic rpc %s failed: %v", operation, err)

	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}

type logicRPCErrorLogger interface {
	Errorf(format string, v ...any)
}

func (l *GetUserByNameLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
