package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserInfoProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserInfoProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserInfoProfileLogic {
	return &UpdateUserInfoProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UpdateUserInfoProfile updates nickname and avatar.
func (l *UpdateUserInfoProfileLogic) UpdateUserInfoProfile(in *pb.UpdateUserInfoProfileReq) (*pb.UpdateUserInfoProfileResp, error) {
	if in.GetId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.UpdateUserInfoProfile(l.ctx, in.GetId(), in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.UpdateUserInfoProfileResp{
		User: userInfoToResponse(info),
	}, nil
}
