package logic

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
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
	email := service.NormalizeEmail(in.GetEmail())
	name := strings.TrimSpace(in.GetUsername())

	if email == "" || in.GetPassword() == "" || name == "" || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(service.CodeInvalidArgument, "missing required auth fields")
	}

	passwordHash, err := service.HashPassword(in.GetPassword())
	if err != nil {
		return nil, errorx.NewCodeError(service.CodeInternal, "hash password failed")
	}

	var user service.UserCredential

	// With transactional publishing configured the registrar owns the transaction
	// and the user.created event; otherwise the plain user store is enough.
	if l.svcCtx.Registrar != nil {
		user, err = l.svcCtx.Registrar.CreateUser(l.ctx, email, passwordHash, name)
	} else {
		user, err = l.svcCtx.Users.CreateUser(l.ctx, email, passwordHash, name)
	}
	if err != nil {
		if service.IsDuplicateEmail(err) {
			return nil, errorx.NewCodeError(service.CodeConflict, "email already registered")
		}
		return nil, errorx.NewCodeError(service.CodeInternal, "create user failed")
	}

	return &pb.RegisterResp{UserId: user.ID}, nil
}
