package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	CodeConversationNotFound = 40410
	CodeNotMember            = 40310
	CodeConversationExists   = 40910
	CodeNotGroupConversation = 40011
	CodeNotOwner             = 40311
	CodeNotAdmin             = 40312
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrNotMember            = errors.New("not a member of the conversation")
	ErrConversationExists   = errors.New("conversation already exists")
	ErrNotGroupConversation = errors.New("not a group conversation")
	ErrNotOwner             = errors.New("not the group owner")
	ErrNotAdmin             = errors.New("not a group admin")
)

type ConversationQuerier interface {
	CreateConversation(ctx context.Context, conversationType string, creatorID int64, memberIDs []int64, name string, avatar string) (model.Conversation, error)
	GetConversationByID(ctx context.Context, id int64) (model.Conversation, error)
	GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]model.Message, error)
	GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]model.Message, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error)
	GetUserConversations(ctx context.Context, userID int64) ([]model.Conversation, error)
	AddGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, newMemberIDs []int64) (model.Conversation, error)
	RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error
	LeaveGroup(ctx context.Context, conversationID, userID int64, userName string) error
	DismissGroup(ctx context.Context, conversationID, operatorID int64) error
	UpdateGroupInfo(ctx context.Context, conversationID, operatorID int64, operatorName string, name, avatar *string) (model.Conversation, error)
	GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]MemberDetail, error)
	UpdateReadReceipt(ctx context.Context, conversationID, userID, lastReadMessageID int64) (ReadState, error)
	ListConversationReadStates(ctx context.Context, conversationID int64) ([]ReadState, error)
}

// ReadState represents a member's last-read cursor in a conversation.
type ReadState struct {
	ConversationID    int64
	UserID            int64
	LastReadMessageID int64
	UpdatedAt         int64 // unix milliseconds
}

type MemberDetail struct {
	UserID   int64
	Email    string
	Avatar   string
	Role     string
	JoinedAt int64
}

type ConversationStore interface {
	CreateConversation(ctx context.Context, arg model.CreateConversationParams) (model.CreateConversationRow, error)
	GetConversation(ctx context.Context, id int64) (model.GetConversationRow, error)
	AddConversationMembers(ctx context.Context, arg model.AddConversationMembersParams) (int64, error)
	AddConversationMemberWithRole(ctx context.Context, arg model.AddConversationMemberWithRoleParams) (int64, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]model.GetConversationMembersRow, error)
	GetConversationsByUserID(ctx context.Context, userID int64) ([]model.GetConversationsByUserIDRow, error)
	ListMessagesByConversation(ctx context.Context, arg model.ListMessagesByConversationParams) ([]model.Message, error)
	ListMessagesByConversationInitial(ctx context.Context, arg model.ListMessagesByConversationInitialParams) ([]model.Message, error)
	CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error)
	GetDirectConversationByMembers(ctx context.Context, arg model.GetDirectConversationByMembersParams) (model.GetDirectConversationByMembersRow, error)
	RemoveConversationMembers(ctx context.Context, arg model.RemoveConversationMembersParams) (int64, error)
	UpdateConversation(ctx context.Context, arg model.UpdateConversationParams) error
	DeactivateConversation(ctx context.Context, id int64) error
	GetConversationCreator(ctx context.Context, id int64) (int64, error)
	IsConversationMember(ctx context.Context, arg model.IsConversationMemberParams) (bool, error)
	GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]model.GetConversationMembersDetailRow, error)
	InsertMessage(ctx context.Context, arg model.InsertMessageParams) error
	UpsertConversationReadState(ctx context.Context, arg model.UpsertConversationReadStateParams) (model.ConversationReadState, error)
	ListConversationReadStates(ctx context.Context, conversationID int64) ([]model.ConversationReadState, error)
}

type systemMessageContent struct {
	Event        string  `json:"event"`
	OperatorID   int64   `json:"operator_id"`
	OperatorName string  `json:"operator_name"`
	TargetIDs    []int64 `json:"target_ids"`
	TargetNames  []int64 `json:"target_names"`
}

type conversationEventPayload struct {
	tracing.TraceContextFields
	MessageID      int64   `json:"message_id"`
	ConversationID int64   `json:"conversation_id"`
	SenderID       int64   `json:"sender_id"`
	MessageType    string  `json:"message_type"`
	Content        string  `json:"content"`
	TargetUserIDs  []int64 `json:"target_user_ids"`
	Timestamp      int64   `json:"timestamp"`
}

type ConversationService struct {
	store  ConversationStore
	idGen  *tools.Snowflake
	pool   *pgxpool.Pool
	pusher *kq.Pusher
}

func NewConversationService(store ConversationStore, idGen *tools.Snowflake, pool *pgxpool.Pool, pusher *kq.Pusher) *ConversationService {
	return &ConversationService{store: store, idGen: idGen, pool: pool, pusher: pusher}
}

func (s *ConversationService) InTx(ctx context.Context, fn func(txQueries *model.Queries) error) error {
	if s.pool == nil {
		return fmt.Errorf("database pool not available for transactions")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(model.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *ConversationService) publishConversationEvent(ctx context.Context, event conversationEventPayload) {
	if s.pusher == nil {
		logx.WithContext(ctx).Debugf("conversation event pusher not configured, skipping event: conv=%d", event.ConversationID)
		return
	}

	event.TraceContextFields = tracing.InjectTraceContext(ctx)
	data, err := json.Marshal(event)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to marshal conversation event: %v", err)
		return
	}

	key := strconv.FormatInt(event.ConversationID, 10)
	if err := s.pusher.PushWithKey(ctx, key, string(data)); err != nil {
		logx.WithContext(ctx).Errorf("failed to push conversation event to Kafka: %v", err)
	}
}

func (s *ConversationService) buildSystemMessage(event string, operatorID int64, operatorName string, targetIDs []int64) []byte {
	content := systemMessageContent{
		Event:        event,
		OperatorID:   operatorID,
		OperatorName: operatorName,
		TargetIDs:    targetIDs,
	}
	data, _ := json.Marshal(content)
	return data
}

func (s ConversationService) CreateConversation(ctx context.Context, conversationType string, creatorID int64, memberIDs []int64, name string, avatar string) (model.Conversation, error) {
	if creatorID <= 0 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "creator_id is required and must be positive")
	}

	if conversationType != "direct" && conversationType != "group" {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
	}

	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		avatar = sharedconversation.DefaultAvatar
	}

	if len(memberIDs) == 0 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	if conversationType == "direct" && len(memberIDs) != 1 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id")
	}

	deduped := make([]int64, 0, len(memberIDs)+1)
	seen := make(map[int64]struct{}, len(memberIDs)+1)
	found := false
	for _, id := range memberIDs {
		if id <= 0 {
			return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
		if id == creatorID {
			found = true
		}
	}
	if !found {
		deduped = append(deduped, creatorID)
		seen[creatorID] = struct{}{}
	}
	memberIDs = deduped

	if conversationType == "direct" && len(memberIDs) != 2 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation must have exactly 2 members")
	}

	if conversationType == "direct" {
		existing, err := s.store.GetDirectConversationByMembers(ctx, model.GetDirectConversationByMembersParams{
			UserID:   memberIDs[0],
			UserID_2: memberIDs[1],
		})
		if err == nil {
			conv := model.Conversation{
				ID:               existing.ID,
				ConversationType: existing.ConversationType,
				IsActive:         existing.IsActive,
				Name:             existing.Name,
				Avatar:           existing.Avatar,
				CreatorID:        existing.CreatorID,
			}
			return conv, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return model.Conversation{}, err
		}
	}

	conversationID, err := s.idGen.NextID()
	if err != nil {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeInternal, "failed to generate conversation id")
	}

	convRow, err := s.store.CreateConversation(ctx, model.CreateConversationParams{
		ID:               conversationID,
		ConversationType: conversationType,
		Name:             name,
		Avatar:           avatar,
		CreatorID:        creatorID,
	})
	if err != nil {
		return model.Conversation{}, err
	}

	conv := model.Conversation{
		ID:               convRow.ID,
		ConversationType: convRow.ConversationType,
		IsActive:         convRow.IsActive,
		Name:             convRow.Name,
		Avatar:           convRow.Avatar,
		CreatorID:        convRow.CreatorID,
	}

	for _, memberID := range memberIDs {
		role := "member"
		if memberID == creatorID {
			role = "owner"
		}
		_, err := s.store.AddConversationMemberWithRole(ctx, model.AddConversationMemberWithRoleParams{
			ConversationID: conv.ID,
			UserID:         memberID,
			Role:           role,
		})
		if err != nil {
			return model.Conversation{}, err
		}
	}

	return conv, nil
}

func (s ConversationService) GetConversationByID(ctx context.Context, id int64) (model.Conversation, error) {
	row, err := s.store.GetConversation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Conversation{}, ErrConversationNotFound
		}
		return model.Conversation{}, err
	}

	return model.Conversation{
		ID:               row.ID,
		ConversationType: row.ConversationType,
		IsActive:         row.IsActive,
		Name:             row.Name,
		Avatar:           row.Avatar,
		CreatorID:        row.CreatorID,
	}, nil
}

func (s ConversationService) GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]model.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if cursorCreatedAt == 0 && cursorID == 0 {
		return s.GetConversationHistoryInitial(ctx, conversationID, limit)
	}

	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversation(ctx, model.ListMessagesByConversationParams{
		ConversationID: conversationID,
		CreatedAt:      pgTimestamptzFromUnix(cursorCreatedAt),
		ID:             cursorID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (s ConversationService) GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]model.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversationInitial(ctx, model.ListMessagesByConversationInitialParams{
		ConversationID: conversationID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (s ConversationService) GetConversationMembers(ctx context.Context, conversationID int64) ([]model.ConversationMember, error) {
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	rows, err := s.store.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	members := make([]model.ConversationMember, len(rows))
	for i, r := range rows {
		members[i] = model.ConversationMember{
			ConversationID: r.ConversationID,
			UserID:         r.UserID,
			IsMuted:        r.IsMuted,
			Role:           r.Role,
		}
	}

	return members, nil
}

func (s ConversationService) IsMember(ctx context.Context, conversationID int64, userID int64) (bool, error) {
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return false, err
	}

	for _, m := range members {
		if m.UserID == userID {
			return true, nil
		}
	}

	return false, nil
}

func (s ConversationService) GetUserConversations(ctx context.Context, userID int64) ([]model.Conversation, error) {
	rows, err := s.store.GetConversationsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if rows == nil {
		return []model.Conversation{}, nil
	}

	conversations := make([]model.Conversation, len(rows))
	for i, r := range rows {
		conversations[i] = model.Conversation{
			ID:               r.ID,
			ConversationType: r.ConversationType,
			IsActive:         r.IsActive,
			Name:             r.Name,
			Avatar:           r.Avatar,
			CreatorID:        r.CreatorID,
		}
	}

	return conversations, nil
}

func (s ConversationService) AddGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, newMemberIDs []int64) (model.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return model.Conversation{}, err
	}

	if conv.ConversationType != "group" {
		return model.Conversation{}, ErrNotGroupConversation
	}

	var targetUserIDs []int64
	var messageID int64

	err = s.InTx(ctx, func(tx *model.Queries) error {
		existingMembers, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}

		existingSet := make(map[int64]struct{}, len(existingMembers))
		for _, m := range existingMembers {
			existingSet[m.UserID] = struct{}{}
		}

		for _, mid := range newMemberIDs {
			if _, ok := existingSet[mid]; ok {
				continue
			}
			_, err := tx.AddConversationMemberWithRole(ctx, model.AddConversationMemberWithRoleParams{
				ConversationID: conversationID,
				UserID:         mid,
				Role:           "member",
			})
			if err != nil {
				return err
			}
		}

		allMembers, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}
		targetUserIDs = make([]int64, 0, len(allMembers))
		for _, m := range allMembers {
			targetUserIDs = append(targetUserIDs, m.UserID)
		}

		id, err := s.idGen.NextID()
		if err != nil {
			return err
		}
		messageID = id

		content := s.buildSystemMessage("member_joined", operatorID, operatorName, newMemberIDs)
		return tx.InsertMessage(ctx, model.InsertMessageParams{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    "system",
			Content:        content,
		})
	})
	if err != nil {
		return model.Conversation{}, err
	}

	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    "system",
		Content:        string(s.buildSystemMessage("member_joined", operatorID, operatorName, newMemberIDs)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s ConversationService) RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}

	if conv.ConversationType != "group" {
		return ErrNotGroupConversation
	}

	var targetUserIDs []int64
	var messageID int64

	err = s.InTx(ctx, func(tx *model.Queries) error {
		members, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}

		targetUserIDs = make([]int64, 0, len(members))
		for _, m := range members {
			targetUserIDs = append(targetUserIDs, m.UserID)
		}

		_, err = tx.RemoveConversationMembers(ctx, model.RemoveConversationMembersParams{
			ConversationID: conversationID,
			Column2:        removeMemberIDs,
		})
		if err != nil {
			return err
		}

		id, err := s.idGen.NextID()
		if err != nil {
			return err
		}
		messageID = id

		content := s.buildSystemMessage("member_removed", operatorID, operatorName, removeMemberIDs)
		return tx.InsertMessage(ctx, model.InsertMessageParams{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    "system",
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    "system",
		Content:        string(s.buildSystemMessage("member_removed", operatorID, operatorName, removeMemberIDs)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	return nil
}

func (s ConversationService) LeaveGroup(ctx context.Context, conversationID, userID int64, userName string) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}

	if conv.ConversationType != "group" {
		return ErrNotGroupConversation
	}

	var targetUserIDs []int64
	var messageID int64

	err = s.InTx(ctx, func(tx *model.Queries) error {
		members, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}

		targetUserIDs = make([]int64, 0, len(members))
		for _, m := range members {
			targetUserIDs = append(targetUserIDs, m.UserID)
		}

		_, err = tx.RemoveConversationMembers(ctx, model.RemoveConversationMembersParams{
			ConversationID: conversationID,
			Column2:        []int64{userID},
		})
		if err != nil {
			return err
		}

		id, err := s.idGen.NextID()
		if err != nil {
			return err
		}
		messageID = id

		content := s.buildSystemMessage("member_left", userID, userName, []int64{userID})
		return tx.InsertMessage(ctx, model.InsertMessageParams{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    "system",
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    "system",
		Content:        string(s.buildSystemMessage("member_left", userID, userName, []int64{userID})),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	return nil
}

func (s ConversationService) DismissGroup(ctx context.Context, conversationID, operatorID int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}

	if conv.ConversationType != "group" {
		return ErrNotGroupConversation
	}

	var targetUserIDs []int64
	var messageID int64

	err = s.InTx(ctx, func(tx *model.Queries) error {
		members, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}

		targetUserIDs = make([]int64, 0, len(members))
		for _, m := range members {
			targetUserIDs = append(targetUserIDs, m.UserID)
		}

		if err := tx.DeactivateConversation(ctx, conversationID); err != nil {
			return err
		}

		id, err := s.idGen.NextID()
		if err != nil {
			return err
		}
		messageID = id

		content := s.buildSystemMessage("group_dismissed", operatorID, "", nil)
		return tx.InsertMessage(ctx, model.InsertMessageParams{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    "system",
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    "system",
		Content:        string(s.buildSystemMessage("group_dismissed", operatorID, "", nil)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	return nil
}

func (s ConversationService) UpdateGroupInfo(ctx context.Context, conversationID, operatorID int64, operatorName string, name, avatar *string) (model.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return model.Conversation{}, err
	}

	if conv.ConversationType != "group" {
		return model.Conversation{}, ErrNotGroupConversation
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			name = nil
		} else {
			name = &trimmed
		}
	}
	if avatar != nil {
		trimmed := strings.TrimSpace(*avatar)
		if trimmed == "" {
			avatar = nil
		} else {
			avatar = &trimmed
		}
	}
	if name == nil && avatar == nil {
		return conv, nil
	}

	var targetUserIDs []int64
	var messageID int64
	eventType := "group_renamed"

	err = s.InTx(ctx, func(tx *model.Queries) error {
		if err := tx.UpdateConversation(ctx, model.UpdateConversationParams{
			ID:     conversationID,
			Name:   name,
			Avatar: avatar,
		}); err != nil {
			return err
		}

		if avatar != nil {
			eventType = "group_avatar_changed"
		}

		members, err := tx.GetConversationMembers(ctx, conversationID)
		if err != nil {
			return err
		}
		targetUserIDs = make([]int64, 0, len(members))
		for _, m := range members {
			targetUserIDs = append(targetUserIDs, m.UserID)
		}

		id, err := s.idGen.NextID()
		if err != nil {
			return err
		}
		messageID = id

		content := s.buildSystemMessage(eventType, operatorID, operatorName, nil)
		return tx.InsertMessage(ctx, model.InsertMessageParams{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    "system",
			Content:        content,
		})
	})
	if err != nil {
		return model.Conversation{}, err
	}

	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    "system",
		Content:        string(s.buildSystemMessage(eventType, operatorID, operatorName, nil)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s ConversationService) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]MemberDetail, error) {
	_, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.GetConversationMembersDetail(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	details := make([]MemberDetail, len(rows))
	for i, r := range rows {
		details[i] = MemberDetail{
			UserID:   r.UserID,
			Email:    r.Email,
			Avatar:   r.Avatar,
			Role:     r.Role,
			JoinedAt: unixFromPGTimestamptz(r.JoinedAt),
		}
	}

	return details, nil
}

// UpdateReadReceipt upserts the caller's last-read cursor for a conversation.
//
// Validates that:
//   - conversation_id > 0, user_id > 0, last_read_message_id > 0
//   - the conversation exists
//   - the user is a member of the conversation
//
// The underlying SQL uses GREATEST(...) so the cursor only advances
// monotonically; a stale or out-of-order receipt is treated as a no-op
// while still returning the latest stored state.
func (s ConversationService) UpdateReadReceipt(ctx context.Context, conversationID, userID, lastReadMessageID int64) (ReadState, error) {
	if conversationID <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required and must be positive")
	}
	if userID <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if lastReadMessageID <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "last_read_message_id is required and must be positive")
	}

	if _, err := s.GetConversationByID(ctx, conversationID); err != nil {
		return ReadState{}, err
	}

	isMember, err := s.IsMember(ctx, conversationID, userID)
	if err != nil {
		return ReadState{}, err
	}
	if !isMember {
		return ReadState{}, ErrNotMember
	}

	row, err := s.store.UpsertConversationReadState(ctx, model.UpsertConversationReadStateParams{
		ConversationID:    conversationID,
		UserID:            userID,
		LastReadMessageID: lastReadMessageID,
	})
	if err != nil {
		return ReadState{}, err
	}

	return ReadState{
		ConversationID:    row.ConversationID,
		UserID:            row.UserID,
		LastReadMessageID: row.LastReadMessageID,
		UpdatedAt:         unixFromPGTimestamptz(row.UpdatedAt),
	}, nil
}

// ListConversationReadStates returns the per-member last-read cursors for a conversation.
func (s ConversationService) ListConversationReadStates(ctx context.Context, conversationID int64) ([]ReadState, error) {
	if conversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required and must be positive")
	}

	if _, err := s.GetConversationByID(ctx, conversationID); err != nil {
		return nil, err
	}

	rows, err := s.store.ListConversationReadStates(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	states := make([]ReadState, len(rows))
	for i, r := range rows {
		states[i] = ReadState{
			ConversationID:    r.ConversationID,
			UserID:            r.UserID,
			LastReadMessageID: r.LastReadMessageID,
			UpdatedAt:         unixFromPGTimestamptz(r.UpdatedAt),
		}
	}

	return states, nil
}

func ConversationToGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrConversationNotFound):
		return errorx.NewCodeError(errorx.CodeNotFound, "conversation not found")
	case errors.Is(err, ErrNotMember):
		return errorx.NewCodeError(errorx.CodeForbidden, "not a member of the conversation")
	case errors.Is(err, ErrNotGroupConversation):
		return errorx.NewCodeError(errorx.CodeBadInput, "not a group conversation")
	case errors.Is(err, ErrNotOwner):
		return errorx.NewCodeError(errorx.CodeForbidden, "not the group owner")
	case errors.Is(err, ErrNotAdmin):
		return errorx.NewCodeError(errorx.CodeForbidden, "not a group admin")
	default:
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
}
