package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
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
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	rpcResp, err := l.svcCtx.LogicBotClient.AddUserBotToConversation(l.ctx, &botservice.AddUserBotToConversationReq{
		OwnerUserId: identity.UserID, BotUserId: req.Id,
		ConversationId: req.ConversationId, OperatorId: identity.UserID, OperatorName: "",
	})
	if err != nil {
		return nil, sanitizeError(l, "add user bot to conversation", err)
	}
	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	return &types.AddUserBotToConversationResponse{
		ConversationId: conv.GetId(), ConversationType: conv.GetConversationType(),
		IsActive: conv.GetIsActive(), CreatedAt: conv.GetCreatedAt(),
		MemberIds: conv.GetMemberIds(), Name: conv.GetName(), Avatar: conv.GetAvatar(),
		CreatorId: conv.GetCreatorId(),
	}, nil
}
