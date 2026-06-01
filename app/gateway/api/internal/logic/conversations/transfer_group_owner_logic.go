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

type TransferGroupOwnerLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewTransferGroupOwnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TransferGroupOwnerLogic {
	return &TransferGroupOwnerLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *TransferGroupOwnerLogic) TransferGroupOwner(req *types.TransferGroupOwnerRequest) (resp *types.TransferGroupOwnerResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	rpcResp, err := l.svcCtx.LogicConversationClient.TransferGroupOwner(l.ctx, &conversationservice.TransferGroupOwnerReq{
		ConversationId: req.Id,
		OperatorId:     identity.UserID,
		TargetUserId:   req.UserId,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "transfer group owner", err)
	}
	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	memberIDs := conv.GetMemberIds()
	if memberIDs == nil {
		memberIDs = []int64{}
	}
	return &types.TransferGroupOwnerResponse{
		ConversationId:   conv.GetId(),
		ConversationType: conv.GetConversationType(),
		IsActive:         conv.GetIsActive(),
		CreatedAt:        conv.GetCreatedAt(),
		MemberIds:        memberIDs,
		Name:             conv.GetName(),
		Avatar:           conv.GetAvatar(),
		CreatorId:        conv.GetCreatorId(),
	}, nil
}
