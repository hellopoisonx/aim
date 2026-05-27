package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotListReadStatesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotListReadStatesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotListReadStatesLogic {
	return &BotListReadStatesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotListReadStatesLogic) BotListReadStates(req *types.BotListReadStatesRequest) (*types.BotListReadStatesResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionReadReceiptRead)
	if err != nil {
		return nil, err
	}

	conversationID, err := parseRequiredID(req.Id, "id")
	if err != nil {
		return nil, err
	}

	client, err := requireLogicConversationClient(l.svcCtx)
	if err != nil {
		return nil, err
	}

	if err := ensureBotConversationMember(l.ctx, client, identity, conversationID); err != nil {
		return nil, err
	}

	rpcResp, err := client.ListConversationReadStates(l.ctx, &conversationservice.ListConversationReadStatesReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return nil, sanitizeBotLogicRPCError(err)
	}

	states := make([]types.BotReadStateItem, 0, len(rpcResp.GetReadStates()))
	for _, st := range rpcResp.GetReadStates() {
		states = append(states, convertBotReadState(st))
	}

	return &types.BotListReadStatesResponse{ReadStates: states}, nil
}
