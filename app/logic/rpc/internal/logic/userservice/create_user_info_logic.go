package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserInfoLogic {
	return &CreateUserInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateUserInfo creates a new user profile.
func (l *CreateUserInfoLogic) CreateUserInfo(in *pb.CreateUserInfoReq) (*pb.CreateUserInfoResp, error) {
	userSvc, err := requireUserOp(l.svcCtx, in.GetId())
	if err != nil {
		return nil, err
	}

	if in.GetEmail() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "email is required")
	}

	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	info, err := userSvc.CreateUserInfo(l.ctx, in.GetId(), in.GetEmail(), in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.CreateUserInfoResp{
		User: pbconv.User(info),
	}, nil
}
