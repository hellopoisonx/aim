package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	tagSvc, err := requireTagService(l.svcCtx, in.GetUserId())
	if err != nil {
		return nil, err
	}

	tags, err := tagSvc.ListTags(l.ctx, in.GetUserId())
	if err != nil {
		return nil, err
	}

	pbTags := make([]*pb.FriendTagResponse, 0, len(tags))
	for _, tag := range tags {
		pbTags = append(pbTags, friendTagToProto(tag))
	}

	return &pb.ListFriendTagsResp{Tags: pbTags}, nil
}
