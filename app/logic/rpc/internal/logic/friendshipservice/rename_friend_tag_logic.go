package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RenameFriendTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRenameFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenameFriendTagLogic {
	return &RenameFriendTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RenameFriendTagLogic) RenameFriendTag(in *pb.RenameFriendTagReq) (*pb.RenameFriendTagResp, error) {
	tagSvc, err := requireTagOp(l.svcCtx, in.GetUserId(), in.GetTagId())
	if err != nil {
		return nil, err
	}

	tag, err := tagSvc.RenameTag(l.ctx, in.GetUserId(), in.GetTagId(), in.GetName())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.RenameFriendTagResp{Tag: friendTagToProto(tag)}, nil
}
