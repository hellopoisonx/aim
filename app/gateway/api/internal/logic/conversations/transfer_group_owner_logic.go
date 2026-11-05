// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package conversations

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
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
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	rpcResp, err := l.svcCtx.LogicConversationClient.TransferGroupOwner(l.ctx, &conversationservice.TransferGroupOwnerReq{
		ConversationId: req.Id,
		OperatorId:     userID,
		TargetUserId:   req.UserId,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "transfer group owner", err)
	}
	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	item := types.TransferGroupOwnerResponse(pbconv.Conversation(conv))

	return &item, nil
}
