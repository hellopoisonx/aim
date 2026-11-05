package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
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
	userSvc, err := requireUserOp(l.svcCtx, in.GetId())
	if err != nil {
		return nil, err
	}

	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	info, err := userSvc.UpdateUserInfoProfile(l.ctx, in.GetId(), in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.UpdateUserInfoProfileResp{
		User: pbconv.User(info),
	}, nil
}
