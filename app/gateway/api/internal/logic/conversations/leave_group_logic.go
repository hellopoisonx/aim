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

type LeaveGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLeaveGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LeaveGroupLogic {
	return &LeaveGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LeaveGroupLogic) LeaveGroup(req *types.LeaveGroupRequest) (err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	_, err = l.svcCtx.LogicConversationClient.LeaveGroup(l.ctx, &conversationservice.LeaveGroupReq{
		ConversationId: req.Id,
		UserId:         identity.UserID,
		UserName:       "",
	})
	if err != nil {
		return errorx.SanitizeGRPCError(l, "leave group", err)
	}

	return nil
}
