package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotListConversationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotListConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotListConversationsLogic {
	return &BotListConversationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotListConversationsLogic) BotListConversations() (*types.BotListConversationsResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionConversationList)
	if err != nil {
		return nil, err
	}

	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicBotClient.ListBotConversations(l.ctx, &botservice.ListBotConversationsReq{
		BotUserId: identity.BotUserID,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	convs := rpcResp.GetConversations()
	out := make([]types.BotConversationItem, 0, len(convs))
	for _, c := range convs {
		out = append(out, types.BotConversationItem{
			ConversationId:   formatID(c.GetConversationId()),
			ConversationType: c.GetConversationType(),
			Name:             c.GetName(),
			Avatar:           c.GetAvatar(),
			CreatedAt:        c.GetCreatedAt(),
		})
	}

	return &types.BotListConversationsResponse{Conversations: out}, nil
}
