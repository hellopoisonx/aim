package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateGroupInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateGroupInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateGroupInfoLogic {
	return &UpdateGroupInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateGroupInfoLogic) UpdateGroupInfo(in *pb.UpdateGroupInfoReq) (*pb.UpdateGroupInfoResp, error) {
	if in.GetConversationId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}
	if in.GetOperatorId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "operator_id is required")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	var name, avatar *string
	if in.Name != nil {
		name = in.Name
	}
	if in.Avatar != nil {
		avatar = in.Avatar
	}

	conv, err := convSvc.UpdateGroupInfo(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetOperatorName(), name, avatar)
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	members, err := convSvc.GetConversationMembers(l.ctx, conv.ID)
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	memberIDs := make([]int64, len(members))
	for i, m := range members {
		memberIDs[i] = m.UserID
	}

	return &pb.UpdateGroupInfoResp{
		Conversation: &pb.ConversationResponse{
			Id:               conv.ID,
			ConversationType: conv.ConversationType,
			IsActive:         conv.IsActive,
			CreatedAt:        service.UnixFromPGTimestamptz(conv.CreatedAt),
			MemberIds:        memberIDs,
			Name:             conv.Name,
			Avatar:           conv.Avatar,
			CreatorId:        conv.CreatorID,
		},
	}, nil
}
