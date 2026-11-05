// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package users

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
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

	rpcResp, err := l.svcCtx.LogicUserClient.SearchUserInfoByNickname(l.ctx, &userservice.SearchUserInfoByNicknameReq{
		Nickname: req.Name,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "get user by name", err)
	}

	users := make([]types.UserListItem, 0, len(rpcResp.GetUsers()))
	for _, user := range rpcResp.GetUsers() {
		users = append(users, pbconv.UserListItem(user))
	}

	return &types.GetUserByNameResponse{
		Users: users,
	}, nil
}
