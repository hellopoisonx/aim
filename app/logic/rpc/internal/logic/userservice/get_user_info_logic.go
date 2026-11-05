package userservicelogic

import (
	"context"

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
	userSvc, err := requireUserOp(l.svcCtx, in.GetId())
	if err != nil {
		return nil, err
	}

	info, err := userSvc.GetUserInfo(l.ctx, in.GetId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.GetUserInfoResp{
		User: userInfoToResponse(info),
	}, nil
}
