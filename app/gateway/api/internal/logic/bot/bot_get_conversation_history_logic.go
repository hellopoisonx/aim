package bot

import (
	"context"

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

	client, err := requireLogicConversationClient(l.svcCtx)
	if err != nil {
		return nil, err
	}

	if err := ensureBotConversationMember(l.ctx, client, identity, conversationID); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rpcResp, err := client.GetConversationHistory(l.ctx, &conversationservice.GetConversationHistoryReq{
		ConversationId:  conversationID,
		CursorCreatedAt: req.CursorCreatedAt,
		CursorId:        cursorID,
		Limit:           limit,
	})
	if err != nil {
		return nil, sanitizeBotLogicRPCError(err)
	}

	messages := make([]types.BotMessageItem, 0, len(rpcResp.GetMessages()))
	for _, msg := range rpcResp.GetMessages() {
		senderInfo := msg.GetSenderInfo()
		readDetails := make([]types.BotMessageReadDetailItem, 0, len(msg.GetReadDetails()))
		for _, rd := range msg.GetReadDetails() {
			readDetails = append(readDetails, types.BotMessageReadDetailItem{
				UserId:            formatID(rd.GetUserId()),
				IsRead:            rd.GetIsRead(),
				LastReadMessageId: formatID(rd.GetLastReadMessageId()),
				UpdatedAt:         rd.GetUpdatedAt(),
				Email:             rd.GetEmail(),
				Avatar:            rd.GetAvatar(),
				DisplayName:       rd.GetDisplayName(),
			})
		}

		messages = append(messages, types.BotMessageItem{
			Id:             formatID(msg.GetId()),
			ConversationId: formatID(msg.GetConversationId()),
			SenderId:       formatID(msg.GetSenderId()),
			SenderInfo: types.BotSenderInfo{
				Name:        senderInfo.GetName(),
				Email:       senderInfo.GetEmail(),
				DisplayName: senderInfo.GetDisplayName(),
			},
			MessageType: msg.GetMessageType(),
			Content:     msg.GetContent(),
			ClientMsgId: msg.GetClientMsgId(),
			CreatedAt:   msg.GetCreatedAt(),
			IsSystem:    msg.GetIsSystem(),
			Mentions:    msg.GetMentions(),
			ReadDetails: readDetails,
		})
	}

	readStates := make([]types.BotReadStateItem, 0, len(rpcResp.GetReadStates()))
	for _, st := range rpcResp.GetReadStates() {
		readStates = append(readStates, convertBotReadState(st))
	}

	return &types.BotGetConversationHistoryResponse{
		Messages:            messages,
		NextCursorCreatedAt: rpcResp.GetNextCursorCreatedAt(),
		NextCursorId:        formatID(rpcResp.GetNextCursorId()),
		HasMore:             rpcResp.GetHasMore(),
		ReadStates:          readStates,
	}, nil
}
