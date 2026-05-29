package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
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
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if in.GetTagId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "tag_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	tag, err := l.svcCtx.FriendTagService.RenameTag(l.ctx, in.GetUserId(), in.GetTagId(), in.GetName())
	if err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	return &pb.RenameFriendTagResp{Tag: friendTagToProto(tag)}, nil
}
