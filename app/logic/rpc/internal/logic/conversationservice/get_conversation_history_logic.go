package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetConversationHistoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetConversationHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationHistoryLogic {
	return &GetConversationHistoryLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetConversationHistory retrieves message history for a conversation with cursor-based pagination.
func (l *GetConversationHistoryLogic) GetConversationHistory(in *pb.GetConversationHistoryReq) (*pb.GetConversationHistoryResp, error) {
	if in.GetConversationId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required and must be positive")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	limit := in.GetLimit()
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	cursorCreatedAt := in.GetCursorCreatedAt()
	cursorID := in.GetCursorId()

	messages, err := convSvc.GetConversationHistory(l.ctx, in.GetConversationId(), cursorCreatedAt, cursorID, limit)
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	senderInfoCache := make(map[int64]*pb.SenderInfo)
	pbMessages := make([]*pb.MessageItem, 0, len(messages))
	for _, msg := range messages {
		senderInfo, ok := senderInfoCache[msg.SenderID]
		if !ok {
			var err error
			senderInfo, err = senderInfoForUser(l.ctx, l.svcCtx, msg.SenderID)
			if err != nil {
				return nil, err
			}
			senderInfoCache[msg.SenderID] = senderInfo
		}

		clientMsgID := ""
		if msg.ClientMsgID != nil {
			clientMsgID = *msg.ClientMsgID
		}

		pbMessages = append(pbMessages, &pb.MessageItem{
			Id:             msg.ID,
			ConversationId: msg.ConversationID,
			SenderId:       msg.SenderID,
			SenderInfo:     senderInfo,
			MessageType:    msg.MessageType,
			Content:        string(msg.Content),
			ClientMsgId:    clientMsgID,
			CreatedAt:      service.UnixFromPGTimestamptz(msg.CreatedAt),
			Mentions:       service.ParseMentionsJSON(msg.Mentions),
		})
	}

	hasMore := int32(len(messages)) == limit
	var nextCursorCreatedAt int64
	var nextCursorID int64

	if hasMore && len(messages) > 0 {
		lastMsg := messages[len(messages)-1]
		nextCursorCreatedAt = service.UnixFromPGTimestamptz(lastMsg.CreatedAt)
		nextCursorID = lastMsg.ID
	}

	readStates, err := convSvc.ListConversationReadStates(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	pbReadStates := make([]*pb.ReadStateItem, len(readStates))
	for i, st := range readStates {
		pbReadStates[i] = &pb.ReadStateItem{
			UserId:            st.UserID,
			LastReadMessageId: st.LastReadMessageID,
			UpdatedAt:         st.UpdatedAt,
		}
	}

	return &pb.GetConversationHistoryResp{
		Messages:            pbMessages,
		NextCursorCreatedAt: nextCursorCreatedAt,
		NextCursorId:        nextCursorID,
		HasMore:             hasMore,
		ReadStates:          pbReadStates,
	}, nil
}
