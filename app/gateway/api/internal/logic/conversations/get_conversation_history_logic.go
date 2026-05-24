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

type GetConversationHistoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetConversationHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationHistoryLogic {
	return &GetConversationHistoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetConversationHistoryLogic) GetConversationHistory(req *types.GetConversationHistoryRequest) (resp *types.GetConversationHistoryResponse, err error) {
	// Verify the caller is authenticated; the middleware guarantees this,
	// but a defensive check prevents misuse if the handler is wired without Auth.
	_, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if req.Id <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.GetConversationHistory(l.ctx, &conversationservice.GetConversationHistoryReq{
		ConversationId:  req.Id,
		CursorCreatedAt: req.CursorCreatedAt,
		CursorId:        req.CursorId,
		Limit:           limit,
	})
	if err != nil {
		return nil, l.sanitizeLogicRPCError("get conversation history", err)
	}

	messages := make([]types.MessageItem, 0, len(rpcResp.GetMessages()))
	for _, msg := range rpcResp.GetMessages() {
		senderInfo := msg.GetSenderInfo()
		readDetails := make([]types.MessageReadDetailItem, 0, len(msg.GetReadDetails()))
		for _, rd := range msg.GetReadDetails() {
			readDetails = append(readDetails, types.MessageReadDetailItem{
				UserId:            rd.GetUserId(),
				IsRead:            rd.GetIsRead(),
				LastReadMessageId: rd.GetLastReadMessageId(),
				UpdatedAt:         rd.GetUpdatedAt(),
				Email:             rd.GetEmail(),
				Avatar:            rd.GetAvatar(),
				DisplayName:       rd.GetDisplayName(),
			})
		}
		messages = append(messages, types.MessageItem{
			Id:             msg.GetId(),
			ConversationId: msg.GetConversationId(),
			SenderId:       msg.GetSenderId(),
			SenderInfo: types.SenderInfo{
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

	var readStates []types.ReadStateItem
	if rpcStates := rpcResp.GetReadStates(); len(rpcStates) > 0 {
		readStates = make([]types.ReadStateItem, 0, len(rpcStates))
		for _, st := range rpcStates {
			readStates = append(readStates, types.ReadStateItem{
				UserId:            st.GetUserId(),
				LastReadMessageId: st.GetLastReadMessageId(),
				UpdatedAt:         st.GetUpdatedAt(),
				Email:             st.GetEmail(),
				Avatar:            st.GetAvatar(),
				DisplayName:       st.GetDisplayName(),
			})
		}
	}

	return &types.GetConversationHistoryResponse{
		Messages:            messages,
		NextCursorCreatedAt: rpcResp.GetNextCursorCreatedAt(),
		NextCursorId:        rpcResp.GetNextCursorId(),
		HasMore:             rpcResp.GetHasMore(),
		ReadStates:          readStates,
	}, nil
}

func (l *GetConversationHistoryLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
