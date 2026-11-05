package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotGetConversationHistoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotGetConversationHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotGetConversationHistoryLogic {
	return &BotGetConversationHistoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotGetConversationHistoryLogic) BotGetConversationHistory(req *types.BotGetConversationHistoryRequest) (*types.BotGetConversationHistoryResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionConversationHistory)
	if err != nil {
		return nil, err
	}

	conversationID, err := parseRequiredID(req.Id, "id")
	if err != nil {
		return nil, err
	}

	cursorID, err := parseOptionalID(req.CursorId, "cursor_id")
	if err != nil {
		return nil, err
	}

	client := logicConversationClient(l.svcCtx)

	if err := ensureBotConversationMember(l.ctx, client, identity, conversationID); err != nil {
		return nil, err
	}

	rpcResp, err := client.GetConversationHistory(l.ctx, &conversationservice.GetConversationHistoryReq{
		ConversationId:  conversationID,
		CursorCreatedAt: req.CursorCreatedAt,
		CursorId:        cursorID,
		Limit:           req.Limit,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCErrorNoLog(err)
	}

	messages := make([]types.BotMessageItem, 0, len(rpcResp.GetMessages()))
	for _, msg := range rpcResp.GetMessages() {
		messages = append(messages, pbconv.BotMessage(msg))
	}

	readStates := make([]types.BotReadStateItem, 0, len(rpcResp.GetReadStates()))
	for _, st := range rpcResp.GetReadStates() {
		readStates = append(readStates, pbconv.BotReadState(st))
	}

	return &types.BotGetConversationHistoryResponse{
		Messages:            messages,
		NextCursorCreatedAt: rpcResp.GetNextCursorCreatedAt(),
		NextCursorId:        formatID(rpcResp.GetNextCursorId()),
		HasMore:             rpcResp.GetHasMore(),
		ReadStates:          readStates,
	}, nil
}
