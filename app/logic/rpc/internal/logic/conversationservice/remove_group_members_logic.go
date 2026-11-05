package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	convSvc, err := requireGroupMembersOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId(), in.GetMemberIds())
	if err != nil {
		return nil, err
	}

	err = convSvc.RemoveGroupMembers(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetMemberIds())
	if err != nil {
		return nil, err
	}

	return &pb.RemoveGroupMembersResp{}, nil
}
