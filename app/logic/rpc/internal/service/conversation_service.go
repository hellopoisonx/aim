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
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
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

const (
	ConversationTypeDirect = "direct"
	ConversationTypeGroup  = "group"

	RoleMember = "member"
	RoleAdmin  = "admin"
	RoleOwner  = "owner"

	MessageTypeSystem = "system"
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
	GrantGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error
	RevokeGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error
	TransferGroupOwner(ctx context.Context, conversationID, operatorID, targetUserID int64) (model.Conversation, error)
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
	Name     string
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
	UpdateConversationCreator(ctx context.Context, arg model.UpdateConversationCreatorParams) error
	UpdateConversationMemberRole(ctx context.Context, arg model.UpdateConversationMemberRoleParams) (int64, error)
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
	store        ConversationStore
	idGen        *tools.Snowflake
	pool         *pgxpool.Pool
	pusher       *kq.Pusher
	convCache    *sharedcache.TypedCache[model.GetConversationRow]
	membersCache *sharedcache.TypedCache[[]model.GetConversationMembersRow]
}

type ConversationServiceOption func(*ConversationService)

func WithConversationCaches(convCache *sharedcache.TypedCache[model.GetConversationRow], membersCache *sharedcache.TypedCache[[]model.GetConversationMembersRow]) ConversationServiceOption {
	return func(s *ConversationService) {
		s.convCache = convCache
		s.membersCache = membersCache
	}
}

func NewConversationService(store ConversationStore, idGen *tools.Snowflake, pool *pgxpool.Pool, pusher *kq.Pusher, opts ...ConversationServiceOption) *ConversationService {
	svc := &ConversationService{store: store, idGen: idGen, pool: pool, pusher: pusher}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}

	return svc
}

func (s *ConversationService) InTx(ctx context.Context, fn func(txQueries *model.Queries) error) error {
	if s.pool == nil {
		return fmt.Errorf("database pool not available for transactions")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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

func (s *ConversationService) getConversationRow(ctx context.Context, id int64) (model.GetConversationRow, error) {
	if s.convCache == nil {
		return s.store.GetConversation(ctx, id)
	}

	return s.convCache.Take(ctx, sharedcache.ConvKey(id), func() (model.GetConversationRow, error) {
		return s.store.GetConversation(ctx, id)
	})
}

func (s *ConversationService) getConversationMemberRows(ctx context.Context, conversationID int64) ([]model.GetConversationMembersRow, error) {
	if s.membersCache == nil {
		return s.store.GetConversationMembers(ctx, conversationID)
	}

	return s.membersCache.Take(ctx, sharedcache.ConvMembersKey(conversationID), func() ([]model.GetConversationMembersRow, error) {
		return s.store.GetConversationMembers(ctx, conversationID)
	})
}

func (s *ConversationService) invalidateConversation(ctx context.Context, conversationID int64) {
	if s.convCache != nil {
		_ = s.convCache.Del(ctx, sharedcache.ConvKey(conversationID))
	}
}

func (s *ConversationService) invalidateConversationMembers(ctx context.Context, conversationID int64) {
	if s.membersCache != nil {
		_ = s.membersCache.Del(ctx, sharedcache.ConvMembersKey(conversationID))
	}
}

func (s *ConversationService) invalidateConversationAll(ctx context.Context, conversationID int64) {
	s.invalidateConversation(ctx, conversationID)
	s.invalidateConversationMembers(ctx, conversationID)
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

func (s ConversationService) memberRole(ctx context.Context, conversationID, userID int64) (string, error) {
	if conversationID <= 0 {
		return "", errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required and must be positive")
	}
	if userID <= 0 {
		return "", errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	members, err := s.getConversationMemberRows(ctx, conversationID)
	if err != nil {
		return "", err
	}
	for _, m := range members {
		if m.UserID == userID {
			if m.Role == "" {
				return RoleMember, nil
			}
			return m.Role, nil
		}
	}
	return "", ErrNotMember
}

func (s ConversationService) isOwner(ctx context.Context, conversationID, userID int64) (bool, error) {
	role, err := s.memberRole(ctx, conversationID, userID)
	if err != nil {
		return false, err
	}
	return role == RoleOwner, nil
}

func (s ConversationService) isAdminOrOwner(ctx context.Context, conversationID, userID int64) (bool, error) {
	role, err := s.memberRole(ctx, conversationID, userID)
	if err != nil {
		return false, err
	}
	return role == RoleOwner || role == RoleAdmin, nil
}

func (s ConversationService) CreateConversation(ctx context.Context, conversationType string, creatorID int64, memberIDs []int64, name string, avatar string) (model.Conversation, error) {
	if creatorID <= 0 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "creator_id is required and must be positive")
	}

	if conversationType != ConversationTypeDirect && conversationType != ConversationTypeGroup {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name = strings.TrimSpace(name)

	if name == "" && conversationType == ConversationTypeGroup {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
	}

	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		avatar = sharedconversation.DefaultAvatar
	}

	if len(memberIDs) == 0 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	if conversationType == ConversationTypeDirect && len(memberIDs) != 1 {
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

	if conversationType == ConversationTypeDirect && len(memberIDs) != 2 {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation must have exactly 2 members")
	}

	if conversationType == ConversationTypeDirect {
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
		role := RoleMember
		if memberID == creatorID {
			role = RoleOwner
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

	s.invalidateConversationAll(ctx, conv.ID)

	return conv, nil
}

func (s ConversationService) GetConversationByID(ctx context.Context, id int64) (model.Conversation, error) {
	row, err := s.getConversationRow(ctx, id)
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

	_, err := s.getConversationRow(ctx, conversationID)
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

	_, err := s.getConversationRow(ctx, conversationID)
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
	_, err := s.getConversationRow(ctx, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	rows, err := s.getConversationMemberRows(ctx, conversationID)
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

	if conv.ConversationType != ConversationTypeGroup {
		return model.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isAdminOrOwner(ctx, conversationID, operatorID)
	if err != nil {
		return model.Conversation{}, err
	}
	if !ok {
		return model.Conversation{}, ErrNotAdmin
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
			if mid <= 0 {
				return errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
			}
			if _, ok := existingSet[mid]; ok {
				continue
			}
			_, err := tx.AddConversationMemberWithRole(ctx, model.AddConversationMemberWithRoleParams{
				ConversationID: conversationID,
				UserID:         mid,
				Role:           RoleMember,
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
			MessageType:    MessageTypeSystem,
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
		MessageType:    MessageTypeSystem,
		Content:        string(s.buildSystemMessage("member_joined", operatorID, operatorName, newMemberIDs)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	s.invalidateConversationMembers(ctx, conversationID)
	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s ConversationService) RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}

	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	operatorRole, err := s.memberRole(ctx, conversationID, operatorID)
	if err != nil {
		return err
	}
	if operatorRole != RoleOwner && operatorRole != RoleAdmin {
		return ErrNotAdmin
	}
	for _, uid := range removeMemberIDs {
		if uid <= 0 {
			return errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
		}
		role, err := s.memberRole(ctx, conversationID, uid)
		if err != nil {
			return err
		}
		if role == RoleOwner || uid == operatorID {
			return ErrNotOwner
		}
		if operatorRole == RoleAdmin && role != RoleMember {
			return ErrNotAdmin
		}
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
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.invalidateConversationMembers(ctx, conversationID)
	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
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

	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	if userID == conv.CreatorID {
		return ErrNotOwner
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
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.invalidateConversationMembers(ctx, conversationID)
	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
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

	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	if operatorID != conv.CreatorID {
		return ErrNotOwner
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
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return err
	}

	s.invalidateConversationAll(ctx, conversationID)
	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
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

	if conv.ConversationType != ConversationTypeGroup {
		return model.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isAdminOrOwner(ctx, conversationID, operatorID)
	if err != nil {
		return model.Conversation{}, err
	}
	if !ok {
		return model.Conversation{}, ErrNotAdmin
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
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return model.Conversation{}, err
	}

	s.invalidateConversation(ctx, conversationID)
	s.publishConversationEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(s.buildSystemMessage(eventType, operatorID, operatorName, nil)),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	})

	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s ConversationService) GrantGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	ok, err := s.isOwner(ctx, conversationID, operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotOwner
	}
	if targetUserID == operatorID {
		return errorx.NewCodeError(errorx.CodeBadInput, "owner cannot grant admin to self")
	}
	targetRole, err := s.memberRole(ctx, conversationID, targetUserID)
	if err != nil {
		return err
	}
	if targetRole == RoleOwner {
		return ErrNotOwner
	}
	if targetRole == RoleAdmin {
		return nil
	}
	if targetRole != RoleMember {
		return ErrNotMember
	}
	rows, err := s.store.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: targetUserID, Role: RoleAdmin})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.invalidateConversationMembers(ctx, conversationID)
	return nil
}

func (s ConversationService) RevokeGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	ok, err := s.isOwner(ctx, conversationID, operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotOwner
	}
	if targetUserID == operatorID {
		return errorx.NewCodeError(errorx.CodeBadInput, "owner cannot revoke self")
	}
	targetRole, err := s.memberRole(ctx, conversationID, targetUserID)
	if err != nil {
		return err
	}
	if targetRole == RoleOwner {
		return ErrNotOwner
	}
	if targetRole == RoleMember {
		return nil
	}
	if targetRole != RoleAdmin {
		return ErrNotMember
	}
	rows, err := s.store.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: targetUserID, Role: RoleMember})
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.invalidateConversationMembers(ctx, conversationID)
	return nil
}

func (s ConversationService) TransferGroupOwner(ctx context.Context, conversationID, operatorID, targetUserID int64) (model.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return model.Conversation{}, err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return model.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isOwner(ctx, conversationID, operatorID)
	if err != nil {
		return model.Conversation{}, err
	}
	if !ok {
		return model.Conversation{}, ErrNotOwner
	}
	if targetUserID == operatorID {
		return model.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "new owner must be a different member")
	}
	targetRole, err := s.memberRole(ctx, conversationID, targetUserID)
	if err != nil {
		return model.Conversation{}, err
	}
	if targetRole == RoleOwner {
		return model.Conversation{}, ErrNotOwner
	}
	if targetRole != RoleMember && targetRole != RoleAdmin {
		return model.Conversation{}, ErrNotMember
	}

	if s.pool != nil {
		err = s.InTx(ctx, func(tx *model.Queries) error {
			if rows, err := tx.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: operatorID, Role: RoleAdmin}); err != nil {
				return err
			} else if rows == 0 {
				return ErrNotMember
			}
			if rows, err := tx.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: targetUserID, Role: RoleOwner}); err != nil {
				return err
			} else if rows == 0 {
				return ErrNotMember
			}
			return tx.UpdateConversationCreator(ctx, model.UpdateConversationCreatorParams{ID: conversationID, CreatorID: targetUserID})
		})
	} else {
		if rows, err := s.store.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: operatorID, Role: RoleAdmin}); err != nil {
			return model.Conversation{}, err
		} else if rows == 0 {
			return model.Conversation{}, ErrNotMember
		}
		if rows, err := s.store.UpdateConversationMemberRole(ctx, model.UpdateConversationMemberRoleParams{ConversationID: conversationID, UserID: targetUserID, Role: RoleOwner}); err != nil {
			return model.Conversation{}, err
		} else if rows == 0 {
			return model.Conversation{}, ErrNotMember
		}
		err = s.store.UpdateConversationCreator(ctx, model.UpdateConversationCreatorParams{ID: conversationID, CreatorID: targetUserID})
	}
	if err != nil {
		return model.Conversation{}, err
	}
	s.invalidateConversationAll(ctx, conversationID)

	return s.GetConversationByID(ctx, conversationID)
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
			Name:     r.Name,
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
