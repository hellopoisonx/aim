package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteFriendTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteFriendTagLogic {
	return &DeleteFriendTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeleteFriendTagLogic) DeleteFriendTag(in *pb.DeleteFriendTagReq) (*pb.DeleteFriendTagResp, error) {
	tagSvc, err := requireTagOp(l.svcCtx, in.GetUserId(), in.GetTagId())
	if err != nil {
		return nil, err
	}

	deleted, err := tagSvc.DeleteTag(l.ctx, in.GetUserId(), in.GetTagId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.DeleteFriendTagResp{Deleted: deleted}, nil
}
