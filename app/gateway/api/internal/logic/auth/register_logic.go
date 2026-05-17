// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package auth

import (
	"context"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterRequest) (resp *types.RegisterResponse, err error) {
	rpcResp, err := l.svcCtx.AuthClient.Register(l.ctx, &authservice.RegisterReq{
		Email:    req.Email,
		Password: req.Password,
		Username: req.Username,
		Avatar:   req.Avatar,
		DeviceId: req.DeviceId,
	})
	if err != nil {
		return nil, l.sanitizeAuthRPCError("register", err)
	}

	return &types.RegisterResponse{UserId: rpcResp.UserId}, nil
}
