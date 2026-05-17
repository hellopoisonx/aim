package logic

import (
	"context"

	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LoginLogic) Login(in *pb.LoginReq) (*pb.LoginResp, error) {
	email := authsvc.NormalizeEmail(in.GetEmail())
	if email == "" || in.GetPassword() == "" || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(authsvc.CodeInvalidArgument, "missing required auth fields")
	}

	user, err := l.svcCtx.Users.GetUserByEmail(l.ctx, email)
	if err != nil {
		if authsvc.IsNotFound(err) {
			return nil, errorx.NewCodeError(authsvc.CodeUnauthorized, "invalid credentials")
		}

		return nil, errorx.NewCodeError(authsvc.CodeInternal, "load user failed")
	}

	if user.Status != authsvc.StatusNormal || !authsvc.CheckPassword(user.PasswordHash, in.GetPassword()) {
		return nil, errorx.NewCodeError(authsvc.CodeUnauthorized, "invalid credentials")
	}

	accessToken, expiresAt, err := l.svcCtx.TokenIssuer.Issue(l.ctx, user.ID, in.GetDeviceId())
	if err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "issue access token failed")
	}

	refreshToken, err := l.svcCtx.Sessions.Create(l.ctx, user.ID, in.GetDeviceId())
	if err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "issue refresh token failed")
	}

	return &pb.LoginResp{UserId: user.ID, AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt}, nil
}
