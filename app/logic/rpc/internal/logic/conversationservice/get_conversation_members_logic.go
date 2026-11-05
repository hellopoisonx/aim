package conversationservicelogic

import (
	"context"

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

// GetConversationMembers retrieves the member IDs and conversation type for a conversation.
func (l *GetConversationMembersLogic) GetConversationMembers(in *pb.GetConversationMembersReq) (*pb.GetConversationMembersResp, error) {
	convSvc, err := requireConversationOp(l.svcCtx.ConversationService, in.GetConversationId())
	if err != nil {
		return nil, err
	}

	conv, err := convSvc.GetConversationByID(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	memberIDs, err := conversationMemberIDs(l.ctx, convSvc, in.GetConversationId())
	if err != nil {
		return nil, err
	}

	return &pb.GetConversationMembersResp{
		ConversationId:   in.GetConversationId(),
		MemberIds:        memberIDs,
		ConversationType: string(conv.ConversationType),
	}, nil
}
