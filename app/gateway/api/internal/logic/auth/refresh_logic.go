// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package auth

import (
	"context"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRefreshLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshLogic {
	return &RefreshLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RefreshLogic) Refresh(req *types.RefreshRequest) (resp *types.RefreshResponse, err error) {
	rpcResp, err := l.svcCtx.AuthClient.RefreshToken(l.ctx, &authservice.RefreshTokenReq{RefreshToken: req.RefreshToken})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "refresh", err)
	}

	return &types.RefreshResponse{AccessToken: rpcResp.AccessToken, RefreshToken: rpcResp.RefreshToken, ExpiresAt: rpcResp.ExpiresAt}, nil
}
