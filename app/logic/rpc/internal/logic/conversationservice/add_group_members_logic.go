package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	convSvc, err := requireGroupMembersOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId(), in.GetMemberIds())
	if err != nil {
		return nil, err
	}

	conv, err := convSvc.AddGroupMembers(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetOperatorName(), in.GetMemberIds())
	if err != nil {
		return nil, err
	}

	memberIDs, err := conversationMemberIDs(l.ctx, convSvc, conv.ID)
	if err != nil {
		return nil, err
	}

	return &pb.AddGroupMembersResp{
		Conversation: pbconv.Conversation(conv, memberIDs, conv.Name),
	}, nil
}
