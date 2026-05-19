// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package users

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserByIdLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserByIdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserByIdLogic {
	return &GetUserByIdLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserByIdLogic) GetUserById(req *types.GetUserByIdRequest) (resp *types.GetUserByIdResponse, err error) {
	if req.Id <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required")
	}

	if l.svcCtx.LogicUserClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicUserClient.GetUserInfo(l.ctx, &userservice.GetUserInfoReq{
		Id: req.Id,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "get user by id", err)
	}

	return &types.GetUserByIdResponse{
		User: userInfoFromRPC(rpcResp.GetUser()),
	}, nil
}
