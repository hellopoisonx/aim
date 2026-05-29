package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListFriendTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendTagsLogic {
	return &ListFriendTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListFriendTagsLogic) ListFriendTags(in *pb.ListFriendTagsReq) (*pb.ListFriendTagsResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	tags, err := l.svcCtx.FriendTagService.ListTags(l.ctx, in.GetUserId())
	if err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	pbTags := make([]*pb.FriendTagResponse, 0, len(tags))
	for _, tag := range tags {
		pbTags = append(pbTags, friendTagToProto(tag))
	}

	return &pb.ListFriendTagsResp{Tags: pbTags}, nil
}
