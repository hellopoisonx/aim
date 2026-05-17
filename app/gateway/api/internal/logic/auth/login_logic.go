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

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginRequest) (resp *types.LoginResponse, err error) {
	rpcResp, err := l.svcCtx.AuthClient.Login(l.ctx, &authservice.LoginReq{
		Email:    req.Email,
		Password: req.Password,
		DeviceId: req.DeviceId,
	})
	if err != nil {
		return nil, l.sanitizeAuthRPCError("login", err)
	}

	return &types.LoginResponse{UserId: rpcResp.UserId, AccessToken: rpcResp.AccessToken, RefreshToken: rpcResp.RefreshToken, ExpiresAt: rpcResp.ExpiresAt}, nil
}
