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

type RevokeGroupAdminLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRevokeGroupAdminLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeGroupAdminLogic {
	return &RevokeGroupAdminLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RevokeGroupAdminLogic) RevokeGroupAdmin(req *types.RevokeGroupAdminRequest) error {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicConversationClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	_, err := l.svcCtx.LogicConversationClient.RevokeGroupAdmin(l.ctx, &conversationservice.RevokeGroupAdminReq{
		ConversationId: req.Id,
		OperatorId:     identity.UserID,
		TargetUserId:   req.Uid,
	})
	if err != nil {
		return errorx.SanitizeGRPCError(l, "revoke group admin", err)
	}
	return nil
}
