package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RemoveGroupMembersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRemoveGroupMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveGroupMembersLogic {
	return &RemoveGroupMembersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RemoveGroupMembersLogic) RemoveGroupMembers(in *pb.RemoveGroupMembersReq) (*pb.RemoveGroupMembersResp, error) {
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

	err := convSvc.RemoveGroupMembers(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetOperatorName(), in.GetMemberIds())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	return &pb.RemoveGroupMembersResp{}, nil
}
