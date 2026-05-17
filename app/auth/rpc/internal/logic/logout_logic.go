package logic

import (
	"context"

	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type LogoutLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLogoutLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LogoutLogic {
	return &LogoutLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LogoutLogic) Logout(in *pb.LogoutReq) (*pb.LogoutResp, error) {
	if in.GetUserId() == 0 || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(authsvc.CodeInvalidArgument, "missing logout identity")
	}

	if err := l.svcCtx.Sessions.RevokeDevice(l.ctx, in.GetUserId(), in.GetDeviceId()); err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "revoke refresh token failed")
	}

	return &pb.LogoutResp{Success: true}, nil
}
