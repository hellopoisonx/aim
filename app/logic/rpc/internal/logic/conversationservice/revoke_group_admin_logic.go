package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RevokeGroupAdminLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRevokeGroupAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeGroupAdminLogic {
	return &RevokeGroupAdminLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RevokeGroupAdminLogic) RevokeGroupAdmin(in *pb.RevokeGroupAdminReq) (*pb.RevokeGroupAdminResp, error) {
	convSvc, err := requireGroupOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId())
	if err != nil {
		return nil, err
	}

	if in.GetTargetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "target_user_id is required")
	}

	if err := convSvc.RevokeGroupAdmin(l.ctx, in.GetConversationId(), service.Actor(in.GetOperatorId()), service.Target(in.GetTargetUserId())); err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	return &pb.RevokeGroupAdminResp{}, nil
}
