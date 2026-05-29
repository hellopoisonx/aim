package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
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
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if in.GetTagId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "tag_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	deleted, err := l.svcCtx.FriendTagService.DeleteTag(l.ctx, in.GetUserId(), in.GetTagId())
	if err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	return &pb.DeleteFriendTagResp{Deleted: deleted}, nil
}
