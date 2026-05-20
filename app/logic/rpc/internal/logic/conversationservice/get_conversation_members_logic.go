package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetConversationMembersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetConversationMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationMembersLogic {
	return &GetConversationMembersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetConversationMembers retrieves the member IDs for a conversation.
func (l *GetConversationMembersLogic) GetConversationMembers(in *pb.GetConversationMembersReq) (*pb.GetConversationMembersResp, error) {
	if in.GetConversationId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	members, err := convSvc.GetConversationMembers(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	memberIDs := make([]int64, len(members))
	for i, member := range members {
		memberIDs[i] = member.UserID
	}

	return &pb.GetConversationMembersResp{
		ConversationId: in.GetConversationId(),
		MemberIds:      memberIDs,
	}, nil
}
