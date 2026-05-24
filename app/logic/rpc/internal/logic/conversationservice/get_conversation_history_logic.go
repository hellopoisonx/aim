package conversationservicelogic

import (
	"context"
	"encoding/json"

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
			Content:        messageContentFromJSONB(msg.Content),
			ClientMsgId:    clientMsgID,
			CreatedAt:      service.UnixFromPGTimestamptz(msg.CreatedAt),
			Mentions:       service.ParseMentionsJSON(msg.Mentions),
			IsSystem:       msg.SenderID == 0 && msg.MessageType == "system",
		})
	}

	hasMore := len(messages) == int(limit)
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
	readStateByUserID := make(map[int64]service.ReadState, len(readStates))
	for i, st := range readStates {
		pbReadStates[i] = &pb.ReadStateItem{
			UserId:            st.UserID,
			LastReadMessageId: st.LastReadMessageID,
			UpdatedAt:         st.UpdatedAt,
		}
		readStateByUserID[st.UserID] = st
	}

	members, err := convSvc.GetConversationMembersDetail(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	// Enrich read states and read details with member name snapshots.
	memberByUserID := make(map[int64]service.MemberDetail, len(members))
	for _, m := range members {
		memberByUserID[m.UserID] = m
	}
	for i, st := range pbReadStates {
		if m, ok := memberByUserID[st.UserId]; ok {
			pbReadStates[i].Email = m.Email
			pbReadStates[i].Avatar = m.Avatar
			pbReadStates[i].DisplayName = m.DisplayName
		}
	}
	for _, msg := range pbMessages {
		msg.ReadDetails = buildMessageReadDetails(msg.GetId(), msg.GetSenderId(), members, readStateByUserID)
	}

	return &pb.GetConversationHistoryResp{
		Messages:            pbMessages,
		NextCursorCreatedAt: nextCursorCreatedAt,
		NextCursorId:        nextCursorID,
		HasMore:             hasMore,
		ReadStates:          pbReadStates,
	}, nil
}

func messageContentFromJSONB(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		return content
	}

	// Non-string JSON payloads (for example system-message objects) are kept as
	// their JSON representation so clients can render or parse them explicitly.
	return string(raw)
}

func buildMessageReadDetails(
	messageID int64,
	senderID int64,
	members []service.MemberDetail,
	readStateByUserID map[int64]service.ReadState,
) []*pb.MessageReadDetailItem {
	if messageID <= 0 || len(members) == 0 {
		return nil
	}

	details := make([]*pb.MessageReadDetailItem, 0, len(members))
	for _, member := range members {
		// The sender naturally has the message locally; omit them so read details
		// represent how other members read this message. The comparison below
		// relies on current Snowflake message IDs being time-ordered within a
		// conversation. If message IDs become non-time-ordered, replace this with
		// a per-conversation sequence or an equivalent read cursor.
		if member.UserID == senderID {
			continue
		}
		state := readStateByUserID[member.UserID]
		details = append(details, &pb.MessageReadDetailItem{
			UserId:            member.UserID,
			IsRead:            state.LastReadMessageID >= messageID,
			LastReadMessageId: state.LastReadMessageID,
			UpdatedAt:         state.UpdatedAt,
			Email:             member.Email,
			Avatar:            member.Avatar,
			DisplayName:       member.DisplayName,
		})
	}

	return details
}
