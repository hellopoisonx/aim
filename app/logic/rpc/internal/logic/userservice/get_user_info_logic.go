package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoLogic {
	return &GetUserInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserInfo retrieves a user profile by ID.
func (l *GetUserInfoLogic) GetUserInfo(in *pb.GetUserInfoReq) (*pb.GetUserInfoResp, error) {
	if in.GetId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.GetUserInfo(l.ctx, in.GetId())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.GetUserInfoResp{
		User: userInfoToResponse(info),
	}, nil
}
