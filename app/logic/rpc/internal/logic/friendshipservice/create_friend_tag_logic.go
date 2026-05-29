package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

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
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if l.svcCtx.FriendTagService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "friend tag service not configured")
	}

	tag, err := l.svcCtx.FriendTagService.CreateTag(l.ctx, in.GetUserId(), in.GetName())
	if err != nil {
		return nil, service.TagErrToGRPCError(err)
	}

	return &pb.CreateFriendTagResp{Tag: friendTagToProto(tag)}, nil
}

func friendTagToProto(tag model.FriendTag) *pb.FriendTagResponse {
	return &pb.FriendTagResponse{
		Id:        tag.ID,
		UserId:    tag.UserID,
		Name:      tag.Name,
		CreatedAt: service.UnixFromPGTimestamptz(tag.CreatedAt),
		UpdatedAt: service.UnixFromPGTimestamptz(tag.UpdatedAt),
	}
}
