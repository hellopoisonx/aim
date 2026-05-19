package userservicelogic

import (
	"context"
	"math"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserInfoStatusLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserInfoStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserInfoStatusLogic {
	return &UpdateUserInfoStatusLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UpdateUserInfoStatus updates user status.
func (l *UpdateUserInfoStatusLogic) UpdateUserInfoStatus(in *pb.UpdateUserInfoStatusReq) (*pb.UpdateUserInfoStatusResp, error) {
	if in.GetId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	if in.GetStatus() < math.MinInt16 || in.GetStatus() > math.MaxInt16 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "status overflows int16")
	}

	info, err := userSvc.UpdateUserInfoStatus(l.ctx, in.GetId(), int16(in.GetStatus())) //nolint:gosec // bounded above before conversion.
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.UpdateUserInfoStatusResp{
		User: userInfoToResponse(info),
	}, nil
}
