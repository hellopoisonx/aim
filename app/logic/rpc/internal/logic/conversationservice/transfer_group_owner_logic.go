package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type TransferGroupOwnerLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewTransferGroupOwnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TransferGroupOwnerLogic {
	return &TransferGroupOwnerLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *TransferGroupOwnerLogic) TransferGroupOwner(in *pb.TransferGroupOwnerReq) (*pb.TransferGroupOwnerResp, error) {
	if in.GetConversationId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}
	if in.GetOperatorId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "operator_id is required")
	}
	if in.GetTargetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "target_user_id is required")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}
	conv, err := convSvc.TransferGroupOwner(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetTargetUserId())
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

	return &pb.TransferGroupOwnerResp{Conversation: &pb.ConversationResponse{
		Id:               conv.ID,
		ConversationType: conv.ConversationType,
		IsActive:         conv.IsActive,
		CreatedAt:        service.UnixFromPGTimestamptz(conv.CreatedAt),
		MemberIds:        memberIDs,
		Name:             conv.Name,
		Avatar:           conv.Avatar,
		CreatorId:        conv.CreatorID,
	}}, nil
}
