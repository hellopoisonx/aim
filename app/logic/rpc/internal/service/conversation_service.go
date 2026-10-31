package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/hellopoisonx/aim/app/shared/tracing"

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

// Conversation vocabulary (types, member roles) lives in app/shared/conversation
// so the DAO and the service agree on one set of literals.
const (
	ConversationTypeDirect = sharedconversation.TypeDirect
	ConversationTypeGroup  = sharedconversation.TypeGroup

	RoleMember = sharedconversation.RoleMember
	RoleAdmin  = sharedconversation.RoleAdmin
	RoleOwner  = sharedconversation.RoleOwner

	MessageTypeSystem = sharedconversation.MessageTypeSystem
)

// Domain errors carry their biz code directly so every RPC handler can return
// them through errorx.ToGRPCError without a hand-written converter.
var (
	ErrConversationNotFound = errorx.NewCodeError(CodeConversationNotFound, "conversation not found")
	ErrNotMember            = errorx.NewCodeError(CodeNotMember, "not a member of the conversation")
	ErrConversationExists   = errorx.NewCodeError(CodeConversationExists, "conversation already exists")
	ErrNotGroupConversation = errorx.NewCodeError(CodeNotGroupConversation, "not a group conversation")
	ErrNotOwner             = errorx.NewCodeError(CodeNotOwner, "not the group owner")
	ErrNotAdmin             = errorx.NewCodeError(CodeNotAdmin, "not a group admin")
)

type ConversationQuerier interface {
	CreateConversation(ctx context.Context, conversationType sharedconversation.Type, creatorID int64, memberIDs []int64, name string, avatar string) (orm.Conversation, error)
	GetConversationByID(ctx context.Context, id int64) (orm.Conversation, error)
	GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]orm.Message, error)
	GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error)
	GetUserConversations(ctx context.Context, userID int64) ([]orm.Conversation, error)
	AddGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, newMemberIDs []int64) (orm.Conversation, error)
	RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error
	LeaveGroup(ctx context.Context, conversationID, userID int64, userName string) error
	DismissGroup(ctx context.Context, conversationID, operatorID int64) error
	UpdateGroupInfo(ctx context.Context, conversationID, operatorID int64, operatorName string, name, avatar *string) (orm.Conversation, error)
	GrantGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error
	RevokeGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error
	TransferGroupOwner(ctx context.Context, conversationID, operatorID, targetUserID int64) (orm.Conversation, error)
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
	CreateConversation(ctx context.Context, id int64, conversationType sharedconversation.Type, name, avatar string, creatorID int64) (orm.Conversation, error)
	GetConversation(ctx context.Context, id int64) (orm.Conversation, error)
	AddConversationMembers(ctx context.Context, conversationID, userID int64) (int64, error)
	AddConversationMemberWithRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error)
	GetConversationsByUserID(ctx context.Context, userID int64) ([]orm.Conversation, error)
	ListMessagesByConversation(ctx context.Context, conversationID int64, cursorCreatedAt time.Time, cursorID int64, limit int32) ([]orm.Message, error)
	ListMessagesByConversationInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error)
	CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error)
	GetDirectConversationByMembers(ctx context.Context, userID, peerID int64) (orm.Conversation, error)
	RemoveConversationMembers(ctx context.Context, conversationID int64, userIDs []int64) (int64, error)
	UpdateConversation(ctx context.Context, id int64, name, avatar *string) error
	UpdateConversationCreator(ctx context.Context, conversationID, creatorID int64) error
	UpdateConversationMemberRole(ctx context.Context, conversationID, userID int64, role sharedconversation.Role) (int64, error)
	DeactivateConversation(ctx context.Context, id int64) error
	GetConversationCreator(ctx context.Context, id int64) (int64, error)
	IsConversationMember(ctx context.Context, conversationID, userID int64) (bool, error)
	GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]dao.ConversationMemberDetail, error)
	InsertMessage(ctx context.Context, message orm.Message) error
	UpsertConversationReadState(ctx context.Context, conversationID, userID, lastReadMessageID int64) (orm.ConversationReadState, error)
	ListConversationReadStates(ctx context.Context, conversationID int64) ([]orm.ConversationReadState, error)
}

// ConversationTxRunner starts a transaction whose callback receives a store
// bound to that transaction.
type ConversationTxRunner interface {
	WithTx(ctx context.Context, fn func(tx ConversationStore) error) error
}

// TransactionalConversationTxRunner additionally publishes an event atomically
// with the callback: the message becomes visible only when the transaction
// commits. It is implemented when RocketMQ transactional publishing is
// configured.
type TransactionalConversationTxRunner interface {
	WithEventTx(ctx context.Context, message rocketmq.TransactionalMessage, fn func(tx ConversationStore) error) error
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
	txRunner     ConversationTxRunner
	idGen        *tools.Snowflake
	eventTopic   string
	convCache    *sharedcache.TypedCache[orm.Conversation]
	membersCache *sharedcache.TypedCache[[]orm.ConversationMember]
}

type ConversationServiceOption func(*ConversationService)

func WithConversationCaches(convCache *sharedcache.TypedCache[orm.Conversation], membersCache *sharedcache.TypedCache[[]orm.ConversationMember]) ConversationServiceOption {
	return func(s *ConversationService) {
		s.convCache = convCache
		s.membersCache = membersCache
	}
}

// WithTransactionalEvents publishes group management notifications on topic as
// RocketMQ transactional messages. It only takes effect together with a
// [TransactionalConversationTxRunner].
func WithTransactionalEvents(topic string) ConversationServiceOption {
	return func(s *ConversationService) {
		s.eventTopic = topic
	}
}

func NewConversationService(store ConversationStore, idGen *tools.Snowflake, txRunner ConversationTxRunner, opts ...ConversationServiceOption) *ConversationService {
	svc := &ConversationService{store: store, txRunner: txRunner, idGen: idGen}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func (s *ConversationService) InTx(ctx context.Context, fn func(tx ConversationStore) error) error {
	if s.txRunner == nil {
		return fmt.Errorf("database transaction runner not available")
	}
	return s.txRunner.WithTx(ctx, fn)
}

// inTxWithEvent runs fn inside a database transaction and publishes payload as
// a transactional event in the same commit. Without a transactional runner it
// degrades to a plain transaction and no event is published.
//
// The payload must therefore be complete before the transaction starts: the
// half message body is fixed at send time.
func (s *ConversationService) inTxWithEvent(ctx context.Context, payload conversationEventPayload, fn func(tx ConversationStore) error) error {
	runner, ok := s.txRunner.(TransactionalConversationTxRunner)
	if !ok || s.eventTopic == "" {
		return s.InTx(ctx, fn)
	}

	payload.TraceContextFields = tracing.InjectTraceContext(ctx)
	body, err := json.Marshal(payload)
	if err != nil {
		logx.WithContext(ctx).Errorf("failed to marshal conversation event: %v", err)
		return err
	}

	return runner.WithEventTx(ctx, rocketmq.TransactionalMessage{
		Topic:    s.eventTopic,
		EventUID: conversationEventUID(payload.MessageID),
		Key:      strconv.FormatInt(payload.ConversationID, 10),
		Body:     body,
	}, fn)
}

// conversationEventUID is the deterministic event identity used by the
// transaction check-back.
func conversationEventUID(messageID int64) string {
	return "conversation.event:" + strconv.FormatInt(messageID, 10)
}

// memberUserIDs projects member rows onto their user ids.
func memberUserIDs(members []orm.ConversationMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return ids
}

func (s *ConversationService) getConversationRow(ctx context.Context, id int64) (orm.Conversation, error) {
	if s.convCache == nil {
		return s.store.GetConversation(ctx, id)
	}

	return s.convCache.Take(ctx, sharedcache.ConversationKey(id), func() (orm.Conversation, error) {
		return s.store.GetConversation(ctx, id)
	})
}

func (s *ConversationService) getConversationMemberRows(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	if s.membersCache == nil {
		return s.store.GetConversationMembers(ctx, conversationID)
	}

	return s.membersCache.Take(ctx, sharedcache.ConversationMembersKey(conversationID), func() ([]orm.ConversationMember, error) {
		return s.store.GetConversationMembers(ctx, conversationID)
	})
}

func (s *ConversationService) invalidateConversation(ctx context.Context, conversationID int64) {
	if s.convCache != nil {
		_ = s.convCache.Del(ctx, sharedcache.ConversationKey(conversationID))
	}
}

func (s *ConversationService) invalidateConversationMembers(ctx context.Context, conversationID int64) {
	if s.membersCache != nil {
		_ = s.membersCache.Del(ctx, sharedcache.ConversationMembersKey(conversationID))
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

func (s *ConversationService) memberRole(ctx context.Context, conversationID, userID int64) (sharedconversation.Role, error) {
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

func (s *ConversationService) isOwner(ctx context.Context, conversationID, userID int64) (bool, error) {
	role, err := s.memberRole(ctx, conversationID, userID)
	if err != nil {
		return false, err
	}
	return role == RoleOwner, nil
}

func (s *ConversationService) isAdminOrOwner(ctx context.Context, conversationID, userID int64) (bool, error) {
	role, err := s.memberRole(ctx, conversationID, userID)
	if err != nil {
		return false, err
	}
	return role == RoleOwner || role == RoleAdmin, nil
}

func (s *ConversationService) CreateConversation(ctx context.Context, conversationType sharedconversation.Type, creatorID int64, memberIDs []int64, name string, avatar string) (orm.Conversation, error) {
	if creatorID <= 0 {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "creator_id is required and must be positive")
	}

	if conversationType != ConversationTypeDirect && conversationType != ConversationTypeGroup {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name = strings.TrimSpace(name)

	if name == "" && conversationType == ConversationTypeGroup {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
	}

	avatar = strings.TrimSpace(avatar)
	if avatar == "" {
		avatar = sharedconversation.DefaultAvatar
	}

	if len(memberIDs) == 0 {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	if conversationType == ConversationTypeDirect && len(memberIDs) != 1 {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id")
	}

	deduped := make([]int64, 0, len(memberIDs)+1)
	seen := make(map[int64]struct{}, len(memberIDs)+1)
	found := false
	for _, id := range memberIDs {
		if id <= 0 {
			return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
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
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation must have exactly 2 members")
	}

	if conversationType == ConversationTypeDirect {
		existing, err := s.store.GetDirectConversationByMembers(ctx, memberIDs[0], memberIDs[1])
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, dao.ErrNotFound) {
			return orm.Conversation{}, err
		}
	}

	conversationID, err := s.idGen.NextID()
	if err != nil {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeInternal, "failed to generate conversation id")
	}

	conv, err := s.store.CreateConversation(ctx, conversationID, conversationType, name, avatar, creatorID)
	if err != nil {
		return orm.Conversation{}, err
	}

	for _, memberID := range memberIDs {
		role := RoleMember
		if memberID == creatorID {
			role = RoleOwner
		}
		_, err := s.store.AddConversationMemberWithRole(ctx, conv.ID, memberID, role)
		if err != nil {
			return orm.Conversation{}, err
		}
	}

	s.invalidateConversationAll(ctx, conv.ID)

	return conv, nil
}

func (s *ConversationService) GetConversationByID(ctx context.Context, id int64) (orm.Conversation, error) {
	row, err := s.getConversationRow(ctx, id)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return orm.Conversation{}, ErrConversationNotFound
		}
		return orm.Conversation{}, err
	}

	return row, nil
}

func (s *ConversationService) GetConversationHistory(ctx context.Context, conversationID int64, cursorCreatedAt int64, cursorID int64, limit int32) ([]orm.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if cursorCreatedAt == 0 && cursorID == 0 {
		return s.GetConversationHistoryInitial(ctx, conversationID, limit)
	}

	_, err := s.getConversationRow(ctx, conversationID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversation(ctx, conversationID, time.UnixMilli(cursorCreatedAt), cursorID, limit)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (s *ConversationService) GetConversationHistoryInitial(ctx context.Context, conversationID int64, limit int32) ([]orm.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	_, err := s.getConversationRow(ctx, conversationID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversationInitial(ctx, conversationID, limit)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

func (s *ConversationService) GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	_, err := s.getConversationRow(ctx, conversationID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	rows, err := s.getConversationMemberRows(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *ConversationService) IsMember(ctx context.Context, conversationID int64, userID int64) (bool, error) {
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

func (s *ConversationService) GetUserConversations(ctx context.Context, userID int64) ([]orm.Conversation, error) {
	rows, err := s.store.GetConversationsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if rows == nil {
		return []orm.Conversation{}, nil
	}

	return rows, nil
}

func (s *ConversationService) AddGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, newMemberIDs []int64) (orm.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}

	if conv.ConversationType != ConversationTypeGroup {
		return orm.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isAdminOrOwner(ctx, conversationID, operatorID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if !ok {
		return orm.Conversation{}, ErrNotAdmin
	}

	for _, mid := range newMemberIDs {
		if mid <= 0 {
			return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
		}
	}

	// The notification body is fixed before the transaction opens, so the
	// recipient list is derived from the members visible right now plus the
	// requested ones; the writes below still re-read the members under the
	// transaction to decide what to insert.
	currentMembers, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}
	targetUserIDs := memberUserIDs(currentMembers)
	seen := make(map[int64]struct{}, len(targetUserIDs))
	for _, id := range targetUserIDs {
		seen[id] = struct{}{}
	}
	for _, mid := range newMemberIDs {
		if _, ok := seen[mid]; ok {
			continue
		}
		seen[mid] = struct{}{}
		targetUserIDs = append(targetUserIDs, mid)
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return orm.Conversation{}, err
	}

	content := s.buildSystemMessage("member_joined", operatorID, operatorName, newMemberIDs)
	err = s.inTxWithEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  targetUserIDs,
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
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
			if _, err := tx.AddConversationMemberWithRole(ctx, conversationID, mid, RoleMember); err != nil {
				return err
			}
		}

		return tx.InsertMessage(ctx, orm.Message{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return orm.Conversation{}, err
	}

	s.invalidateConversationMembers(ctx, conversationID)
	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s *ConversationService) RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error {
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

	// Recipients are everyone still in the group before the removal commits.
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return err
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return err
	}

	content := s.buildSystemMessage("member_removed", operatorID, operatorName, removeMemberIDs)
	err = s.inTxWithEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  memberUserIDs(members),
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
		if _, err := tx.RemoveConversationMembers(ctx, conversationID, removeMemberIDs); err != nil {
			return err
		}

		return tx.InsertMessage(ctx, orm.Message{
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
	return nil
}

func (s *ConversationService) LeaveGroup(ctx context.Context, conversationID, userID int64, userName string) error {
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

	// Recipients are everyone still in the group before the leave commits.
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return err
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return err
	}

	content := s.buildSystemMessage("member_left", userID, userName, []int64{userID})
	err = s.inTxWithEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  memberUserIDs(members),
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
		if _, err := tx.RemoveConversationMembers(ctx, conversationID, []int64{userID}); err != nil {
			return err
		}

		return tx.InsertMessage(ctx, orm.Message{
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
	return nil
}

func (s *ConversationService) DismissGroup(ctx context.Context, conversationID, operatorID int64) error {
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

	// Everyone in the group is notified before it is deactivated.
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return err
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return err
	}

	content := s.buildSystemMessage("group_dismissed", operatorID, "", nil)
	err = s.inTxWithEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  memberUserIDs(members),
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
		if err := tx.DeactivateConversation(ctx, conversationID); err != nil {
			return err
		}

		return tx.InsertMessage(ctx, orm.Message{
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
	return nil
}

func (s *ConversationService) UpdateGroupInfo(ctx context.Context, conversationID, operatorID int64, operatorName string, name, avatar *string) (orm.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}

	if conv.ConversationType != ConversationTypeGroup {
		return orm.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isAdminOrOwner(ctx, conversationID, operatorID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if !ok {
		return orm.Conversation{}, ErrNotAdmin
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

	eventType := "group_renamed"
	if avatar != nil {
		eventType = "group_avatar_changed"
	}

	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}

	messageID, err := s.idGen.NextID()
	if err != nil {
		return orm.Conversation{}, err
	}

	content := s.buildSystemMessage(eventType, operatorID, operatorName, nil)
	err = s.inTxWithEvent(ctx, conversationEventPayload{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  memberUserIDs(members),
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
		if err := tx.UpdateConversation(ctx, conversationID, name, avatar); err != nil {
			return err
		}

		return tx.InsertMessage(ctx, orm.Message{
			ID:             messageID,
			ConversationID: conversationID,
			SenderID:       0,
			MessageType:    MessageTypeSystem,
			Content:        content,
		})
	})
	if err != nil {
		return orm.Conversation{}, err
	}

	s.invalidateConversation(ctx, conversationID)
	conv, _ = s.GetConversationByID(ctx, conversationID)
	return conv, nil
}

func (s *ConversationService) GrantGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error {
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
	rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, targetUserID, RoleAdmin)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.invalidateConversationMembers(ctx, conversationID)
	return nil
}

func (s *ConversationService) RevokeGroupAdmin(ctx context.Context, conversationID, operatorID, targetUserID int64) error {
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
	rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, targetUserID, RoleMember)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.invalidateConversationMembers(ctx, conversationID)
	return nil
}

func (s *ConversationService) TransferGroupOwner(ctx context.Context, conversationID, operatorID, targetUserID int64) (orm.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return orm.Conversation{}, ErrNotGroupConversation
	}
	ok, err := s.isOwner(ctx, conversationID, operatorID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if !ok {
		return orm.Conversation{}, ErrNotOwner
	}
	if targetUserID == operatorID {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "new owner must be a different member")
	}
	targetRole, err := s.memberRole(ctx, conversationID, targetUserID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if targetRole == RoleOwner {
		return orm.Conversation{}, ErrNotOwner
	}
	if targetRole != RoleMember && targetRole != RoleAdmin {
		return orm.Conversation{}, ErrNotMember
	}

	if s.txRunner != nil {
		err = s.InTx(ctx, func(tx ConversationStore) error {
			if rows, err := tx.UpdateConversationMemberRole(ctx, conversationID, operatorID, RoleAdmin); err != nil {
				return err
			} else if rows == 0 {
				return ErrNotMember
			}
			if rows, err := tx.UpdateConversationMemberRole(ctx, conversationID, targetUserID, RoleOwner); err != nil {
				return err
			} else if rows == 0 {
				return ErrNotMember
			}
			return tx.UpdateConversationCreator(ctx, conversationID, targetUserID)
		})
	} else {
		if rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, operatorID, RoleAdmin); err != nil {
			return orm.Conversation{}, err
		} else if rows == 0 {
			return orm.Conversation{}, ErrNotMember
		}
		if rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, targetUserID, RoleOwner); err != nil {
			return orm.Conversation{}, err
		} else if rows == 0 {
			return orm.Conversation{}, ErrNotMember
		}
		err = s.store.UpdateConversationCreator(ctx, conversationID, targetUserID)
	}
	if err != nil {
		return orm.Conversation{}, err
	}
	s.invalidateConversationAll(ctx, conversationID)

	return s.GetConversationByID(ctx, conversationID)
}

func (s *ConversationService) GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]MemberDetail, error) {
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
			JoinedAt: r.JoinedAt.UnixMilli(),
		}
	}

	return details, nil
}

func (s *ConversationService) UpdateReadReceipt(ctx context.Context, conversationID, userID, lastReadMessageID int64) (ReadState, error) {
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

	row, err := s.store.UpsertConversationReadState(ctx, conversationID, userID, lastReadMessageID)
	if err != nil {
		return ReadState{}, err
	}

	return ReadState{
		ConversationID:    row.ConversationID,
		UserID:            row.UserID,
		LastReadMessageID: row.LastReadMessageID,
		UpdatedAt:         row.UpdatedAt.UnixMilli(),
	}, nil
}

func (s *ConversationService) ListConversationReadStates(ctx context.Context, conversationID int64) ([]ReadState, error) {
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
			UpdatedAt:         r.UpdatedAt.UnixMilli(),
		}
	}

	return states, nil
}
