package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateFriendTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateFriendTagLogic {
	return &CreateFriendTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateFriendTagLogic) CreateFriendTag(in *pb.CreateFriendTagReq) (*pb.CreateFriendTagResp, error) {
	tagSvc, err := requireTagService(l.svcCtx, in.GetUserId())
	if err != nil {
		return nil, err
	}

	tag, err := tagSvc.CreateTag(l.ctx, in.GetUserId(), in.GetName())
	if err != nil {
		return nil, err
	}

	return &pb.CreateFriendTagResp{Tag: friendTagToProto(tag)}, nil
}

func friendTagToProto(tag orm.FriendTag) *pb.FriendTagResponse {
	return &pb.FriendTagResponse{
		Id:        tag.ID,
		UserId:    tag.UserID,
		Name:      tag.Name,
		CreatedAt: tag.CreatedAt.UnixMilli(),
		UpdatedAt: tag.UpdatedAt.UnixMilli(),
	}
}
