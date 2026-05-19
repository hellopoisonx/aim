package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserInfoByNicknameLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserInfoByNicknameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserInfoByNicknameLogic {
	return &GetUserInfoByNicknameLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserInfoByNickname retrieves a user profile by exact nickname.
func (l *GetUserInfoByNicknameLogic) GetUserInfoByNickname(in *pb.GetUserInfoByNicknameReq) (*pb.GetUserInfoResp, error) {
	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.GetUserInfoByNickname(l.ctx, in.GetNickname())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.GetUserInfoResp{
		User: userInfoToResponse(info),
	}, nil
}
