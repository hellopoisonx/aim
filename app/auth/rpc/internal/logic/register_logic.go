package logic

import (
	"context"

	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RegisterLogic) Register(in *pb.RegisterReq) (*pb.RegisterResp, error) {
	email := authsvc.NormalizeEmail(in.GetEmail())
	if email == "" || in.GetPassword() == "" || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(authsvc.CodeInvalidArgument, "missing required auth fields")
	}

	passwordHash, err := authsvc.HashPassword(in.GetPassword())
	if err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "hash password failed")
	}

	user, err := l.svcCtx.Users.CreateUser(l.ctx, email, passwordHash)
	if err != nil {
		if authsvc.IsDuplicateEmail(err) {
			return nil, errorx.NewCodeError(authsvc.CodeConflict, "email already registered")
		}

		return nil, errorx.NewCodeError(authsvc.CodeInternal, "create user failed")
	}

	return &pb.RegisterResp{UserId: user.ID}, nil
}
