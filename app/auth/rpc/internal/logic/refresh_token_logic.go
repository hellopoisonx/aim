package logic

import (
	"context"

	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
	return &RefreshTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RefreshTokenLogic) RefreshToken(in *pb.RefreshTokenReq) (*pb.RefreshTokenResp, error) {
	if in.GetRefreshToken() == "" {
		return nil, errorx.NewCodeError(authsvc.CodeInvalidArgument, "missing refresh token")
	}

	userID, deviceID, refreshToken, err := l.svcCtx.Sessions.Rotate(l.ctx, in.GetRefreshToken())
	if err != nil {
		return nil, err
	}

	accessToken, expiresAt, err := l.svcCtx.TokenIssuer.Issue(l.ctx, userID, deviceID)
	if err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "issue access token failed")
	}

	return &pb.RefreshTokenResp{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt}, nil
}
