package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddUserBotToConversationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddUserBotToConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddUserBotToConversationLogic {
	return &AddUserBotToConversationLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *AddUserBotToConversationLogic) AddUserBotToConversation(req *types.AddUserBotToConversationRequest) (resp *types.AddUserBotToConversationResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}
	rpcResp, err := l.svcCtx.LogicBotClient.AddUserBotToConversation(l.ctx, &botservice.AddUserBotToConversationReq{
		OwnerUserId: userID, BotUserId: req.Id,
		ConversationId: req.ConversationId, OperatorId: userID, OperatorName: "",
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "add user bot to conversation", err)
	}
	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	item := types.AddUserBotToConversationResponse(pbconv.Conversation(conv))
	return &item, nil
}
