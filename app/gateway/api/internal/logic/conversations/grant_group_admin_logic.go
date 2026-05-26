// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package conversations

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GrantGroupAdminLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGrantGroupAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GrantGroupAdminLogic {
	return &GrantGroupAdminLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GrantGroupAdminLogic) GrantGroupAdmin(req *types.GrantGroupAdminRequest) error {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicConversationClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	_, err := l.svcCtx.LogicConversationClient.GrantGroupAdmin(l.ctx, &conversationservice.GrantGroupAdminReq{
		ConversationId: req.Id,
		OperatorId:     identity.UserID,
		TargetUserId:   req.Uid,
	})
	if err != nil {
		return sanitizeLogicRPCError(l, "grant group admin", err)
	}
	return nil
}
