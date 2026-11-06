package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type DismissGroupLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDismissGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DismissGroupLogic {
	return &DismissGroupLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DismissGroupLogic) DismissGroup(in *pb.DismissGroupReq) (*pb.DismissGroupResp, error) {
	convSvc, err := requireGroupOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId())
	if err != nil {
		return nil, err
	}

	if err := convSvc.DismissGroup(l.ctx, in.GetConversationId(), in.GetOperatorId()); err != nil {
		return nil, err
	}

	return &pb.DismissGroupResp{}, nil
}
