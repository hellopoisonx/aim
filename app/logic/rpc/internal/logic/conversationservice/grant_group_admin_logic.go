package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
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
	if err := convSvc.GrantGroupAdmin(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetTargetUserId()); err != nil {
		return nil, service.ConversationToGRPCError(err)
	}
	return &pb.GrantGroupAdminResp{}, nil
}
