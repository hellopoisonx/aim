package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
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

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.GetUserInfoByEmail(l.ctx, in.GetEmail())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.GetUserInfoResp{
		User: userInfoToResponse(info),
	}, nil
}
