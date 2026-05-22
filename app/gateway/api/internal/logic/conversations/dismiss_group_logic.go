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

type DismissGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDismissGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DismissGroupLogic {
	return &DismissGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DismissGroupLogic) DismissGroup(req *types.DismissGroupRequest) (err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	_, err = l.svcCtx.LogicConversationClient.DismissGroup(l.ctx, &conversationservice.DismissGroupReq{
		ConversationId: req.Id,
		OperatorId:     identity.UserID,
	})
	if err != nil {
		return sanitizeLogicRPCError(l, "dismiss group", err)
	}

	return nil
}

func (l *DismissGroupLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
