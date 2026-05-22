package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddGroupMembersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAddGroupMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddGroupMembersLogic {
	return &AddGroupMembersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AddGroupMembersLogic) AddGroupMembers(in *pb.AddGroupMembersReq) (*pb.AddGroupMembersResp, error) {
	if in.GetConversationId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}
	if in.GetOperatorId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "operator_id is required")
	}
	if len(in.GetMemberIds()) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	conv, err := convSvc.AddGroupMembers(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetOperatorName(), in.GetMemberIds())
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

	return &pb.AddGroupMembersResp{
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
