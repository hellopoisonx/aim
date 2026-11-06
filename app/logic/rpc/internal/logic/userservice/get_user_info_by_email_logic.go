package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoByEmailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserInfoByEmailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoByEmailLogic {
	return &GetUserInfoByEmailLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserInfoByEmail retrieves a user profile by email.
func (l *GetUserInfoByEmailLogic) GetUserInfoByEmail(in *pb.GetUserInfoByEmailReq) (*pb.GetUserInfoResp, error) {
	if in.GetEmail() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "email is required")
	}

	userSvc, err := requireUserService(l.svcCtx)
	if err != nil {
		return nil, err
	}

	info, err := userSvc.GetUserInfoByEmail(l.ctx, in.GetEmail())
	if err != nil {
		return nil, err
	}

	return &pb.GetUserInfoResp{
		User: pbconv.User(info),
	}, nil
}
