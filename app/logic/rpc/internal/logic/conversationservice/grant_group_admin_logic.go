package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GrantGroupAdminLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGrantGroupAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GrantGroupAdminLogic {
	return &GrantGroupAdminLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GrantGroupAdminLogic) GrantGroupAdmin(in *pb.GrantGroupAdminReq) (*pb.GrantGroupAdminResp, error) {
	convSvc, err := requireGroupOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId())
	if err != nil {
		return nil, err
	}

	if in.GetTargetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "target_user_id is required")
	}

	if err := convSvc.GrantGroupAdmin(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetTargetUserId()); err != nil {
		return nil, errorx.ToGRPCError(err)
	}
	return &pb.GrantGroupAdminResp{}, nil
}
