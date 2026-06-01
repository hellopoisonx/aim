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

type RemoveGroupMemberLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveGroupMemberLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveGroupMemberLogic {
	return &RemoveGroupMemberLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RemoveGroupMemberLogic) RemoveGroupMember(req *types.RemoveGroupMemberRequest) (err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	_, err = l.svcCtx.LogicConversationClient.RemoveGroupMembers(l.ctx, &conversationservice.RemoveGroupMembersReq{
		ConversationId: req.Id,
		OperatorId:     identity.UserID,
		OperatorName:   "",
		MemberIds:      []int64{req.Uid},
	})
	if err != nil {
		return errorx.SanitizeGRPCError(l, "remove group member", err)
	}

	return nil
}
