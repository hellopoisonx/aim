package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service/rbac"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/snowflake"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	shareduser "github.com/hellopoisonx/aim/app/shared/user"

	"github.com/zeromicro/go-zero/core/logx"
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
	ErrConversationNotFound = errorx.NewCodeError(errorx.CodeConversationNotFound, "conversation not found")
	ErrNotMember            = errorx.NewCodeError(errorx.CodeConversationNotMember, "not a member of the conversation")
	ErrConversationExists   = errorx.NewCodeError(errorx.CodeConversationExists, "conversation already exists")
	ErrNotGroupConversation = errorx.NewCodeError(errorx.CodeConversationNotGroup, "not a group conversation")
	ErrNotOwner             = errorx.NewCodeError(errorx.CodeConversationNotOwner, "not the group owner")
	ErrNotAdmin             = errorx.NewCodeError(errorx.CodeConversationNotAdmin, "not a group admin")
)

// Actor is the member performing an operation on another member.
type Actor int64

// Target is the member an operation performed by an Actor applies to.
type Target int64

type ConversationQuerier interface {
	CreateConversation(ctx context.Context, conversationType sharedconversation.Type, creatorID int64, memberIDs []int64, name string, avatar string) (orm.Conversation, error)
	GetConversationByID(ctx context.Context, id int64) (orm.Conversation, error)
	GetConversationHistory(ctx context.Context, conversationID int64, cursor sharedconversation.MessageCursor, limit int32) ([]orm.Message, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error)
	GetUserConversations(ctx context.Context, userID int64) ([]orm.Conversation, error)
	AddGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, newMemberIDs []int64) (orm.Conversation, error)
	RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error
	LeaveGroup(ctx context.Context, conversationID, userID int64, userName string) error
	DismissGroup(ctx context.Context, conversationID, operatorID int64) error
	UpdateGroupInfo(ctx context.Context, conversationID, operatorID int64, operatorName string, name, avatar *string) (orm.Conversation, error)
	GrantGroupAdmin(ctx context.Context, conversationID int64, actor Actor, target Target) error
	RevokeGroupAdmin(ctx context.Context, conversationID int64, actor Actor, target Target) error
	TransferGroupOwner(ctx context.Context, conversationID int64, actor Actor, target Target) (orm.Conversation, error)
	GetConversationMembersDetail(ctx context.Context, conversationID int64) ([]MemberDetail, error)
	UpdateReadReceipt(ctx context.Context, conversationID int64, actor Actor, lastReadMessageID int64) (ReadState, error)
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
	ListMessagesByConversation(ctx context.Context, conversationID int64, cursor sharedconversation.MessageCursor, limit int32) ([]orm.Message, error)
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
	// GetUserType resolves the principal type (human/bot) that prefixes an RBAC
	// subject.
	GetUserType(ctx context.Context, id int64) (string, error)
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

type ConversationService struct {
	store        ConversationStore
	txRunner     ConversationTxRunner
	idGen        *snowflake.Snowflake
	eventTopic   string
	convCache    *sharedcache.TypedCache[orm.Conversation]
	membersCache *sharedcache.TypedCache[[]orm.ConversationMember]
	authz        rbac.Authorizer
}

type ConversationServiceOption func(*ConversationService)

func WithRBAC(authz rbac.Authorizer) ConversationServiceOption {
	return func(s *ConversationService) {
		s.authz = authz
	}
}

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

func NewConversationService(store ConversationStore, idGen *snowflake.Snowflake, txRunner ConversationTxRunner, opts ...ConversationServiceOption) *ConversationService {
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

// withStore runs fn in a transaction when one is configured, otherwise against
// the plain store, so callers write the mutation once.
func (s *ConversationService) withStore(ctx context.Context, fn func(store ConversationStore) error) error {
	if s.txRunner == nil {
		return fn(s.store)
	}
	return s.txRunner.WithTx(ctx, fn)
}

// inTxWithEvent runs fn inside a database transaction and publishes payload as
// a transactional event in the same commit. Without a transactional runner it
// degrades to a plain transaction and no event is published.
//
// The payload must therefore be complete before the transaction starts: the
// half message body is fixed at send time.
func (s *ConversationService) inTxWithEvent(ctx context.Context, payload events.ConversationEvent, fn func(tx ConversationStore) error) error {
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

// insertSystemMessageTx writes the group system message and publishes its
// conversation event in one transaction. contentTargetIDs are named in the
// message body, notifyTargetIDs receive the event.
func (s *ConversationService) insertSystemMessageTx(ctx context.Context, conversationID, operatorID int64, operatorName, event string, contentTargetIDs, notifyTargetIDs []int64, mutate func(ConversationStore) error) error {
	messageID, err := s.idGen.NextID()
	if err != nil {
		return err
	}

	content := s.buildSystemMessage(event, operatorID, operatorName, contentTargetIDs)
	return s.inTxWithEvent(ctx, events.ConversationEvent{
		MessageID:      messageID,
		ConversationID: conversationID,
		SenderID:       0,
		MessageType:    MessageTypeSystem,
		Content:        string(content),
		TargetUserIDs:  notifyTargetIDs,
		Timestamp:      time.Now().UnixMilli(),
	}, func(tx ConversationStore) error {
		if err := mutate(tx); err != nil {
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

func (s *ConversationService) requireAct(ctx context.Context, conversationID, userID int64, act string) error {
	if s.authz == nil {
		return s.requireStoredRole(ctx, conversationID, userID, act)
	}

	userType, err := s.userType(ctx, userID)
	if err != nil {
		return err
	}

	subject, err := rbac.Subject(userType, userID)
	if err != nil {
		return err
	}

	allowed, err := s.authz.Authorize(ctx, subject, rbac.Domain(conversationID), rbac.ObjectConversation, act)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}

	if act == rbac.ActionGroupOwn {
		return ErrNotOwner
	}

	return ErrNotAdmin
}

func (s *ConversationService) requireStoredRole(ctx context.Context, conversationID, userID int64, act string) error {
	role, err := s.memberRole(ctx, conversationID, userID)
	if err != nil {
		return err
	}

	switch act {
	case rbac.ActionGroupOwn:
		if role == RoleOwner {
			return nil
		}
		return ErrNotOwner
	case rbac.ActionGroupManage:
		if role == RoleOwner || role == RoleAdmin {
			return nil
		}
		return ErrNotAdmin
	default:
		if role == RoleOwner || role == RoleAdmin || role == RoleMember {
			return nil
		}
		return ErrNotMember
	}
}

func (s *ConversationService) isBot(ctx context.Context, userID int64) (bool, error) {
	userType, err := s.userType(ctx, userID)
	if err != nil {
		return false, err
	}

	return userType == shareduser.TypeBot, nil
}

func (s *ConversationService) rbacSubject(ctx context.Context, userID int64) (string, error) {
	userType, err := s.userType(ctx, userID)
	if err != nil {
		return "", err
	}

	return rbac.Subject(userType, userID)
}

// userType resolves a principal type, failing instead of defaulting when the
// stored value is outside the closed set.
func (s *ConversationService) userType(ctx context.Context, userID int64) (shareduser.Type, error) {
	raw, err := s.store.GetUserType(ctx, userID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return "", ErrNotMember
		}
		return "", err
	}

	return shareduser.ParseType(raw)
}

// roleSync selects how the post-commit step treats the conversation's RBAC
// rules.
type roleSync int

const (
	// rolesFromMembers rebuilds every assignment from conversation_members.
	rolesFromMembers roleSync = iota
	// rolesReset drops every assignment, for a conversation that no longer
	// grants anything.
	rolesReset
)

// postCommit applies the effects that cannot join the write transaction, once
// it has committed: cache invalidation followed by one authorizer rebuild.
//
// Failure policy: the committed write is authoritative, so these effects never
// fail the call; a failure is logged once with the conversation id. Cache
// entries self-heal on the next read through their TTL, and a failed rebuild
// leaves the conversation's rules stale or incomplete only until the next
// successful mutation, which rebuilds all of them from conversation_members.
func (s *ConversationService) postCommit(ctx context.Context, conversationID int64, roles roleSync) {
	if s.convCache != nil {
		_ = s.convCache.Del(ctx, sharedcache.ConversationKey(conversationID))
	}
	if s.membersCache != nil {
		_ = s.membersCache.Del(ctx, sharedcache.ConversationMembersKey(conversationID))
	}

	if s.authz == nil {
		return
	}

	if roles == rolesReset {
		if err := s.authz.ClearConversation(ctx, rbac.Domain(conversationID)); err != nil {
			logx.WithContext(ctx).Errorf("post-commit: clear rbac roles of conversation %d: %v", conversationID, err)
		}
		return
	}

	if err := s.syncRoles(ctx, conversationID); err != nil {
		logx.WithContext(ctx).Errorf("post-commit: sync rbac roles of conversation %d: %v", conversationID, err)
	}
}

// syncRoles replaces the conversation's assignments with the roles stored in
// conversation_members, the source of truth for who holds which role.
func (s *ConversationService) syncRoles(ctx context.Context, conversationID int64) error {
	members, err := s.store.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return err
	}

	if err := s.authz.ClearConversation(ctx, rbac.Domain(conversationID)); err != nil {
		return err
	}

	var firstErr error
	for _, member := range members {
		role := member.Role
		if role == "" {
			role = RoleMember
		}
		subject, err := s.rbacSubject(ctx, member.UserID)
		if err != nil {
			firstErr = errors.Join(firstErr, err)
			continue
		}
		if err := s.authz.AssignRole(ctx, subject, rbac.Domain(conversationID), string(role)); err != nil {
			firstErr = errors.Join(firstErr, err)
		}
	}

	return firstErr
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

	s.postCommit(ctx, conv.ID, rolesFromMembers)

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

const (
	defaultConversationHistoryLimit int32 = 50
	maxConversationHistoryLimit     int32 = 100
)

// NormalizeHistoryLimit resolves a caller-supplied page size to the one policy
// every history entry point uses: default 50, capped at 100.
func NormalizeHistoryLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultConversationHistoryLimit
	}
	if limit > maxConversationHistoryLimit {
		return maxConversationHistoryLimit
	}
	return limit
}

func (s *ConversationService) GetConversationHistory(ctx context.Context, conversationID int64, cursor sharedconversation.MessageCursor, limit int32) ([]orm.Message, error) {
	limit = NormalizeHistoryLimit(limit)

	_, err := s.getConversationRow(ctx, conversationID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	messages, err := s.store.ListMessagesByConversation(ctx, conversationID, cursor, limit)
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
	if err := s.requireAct(ctx, conversationID, operatorID, rbac.ActionGroupManage); err != nil {
		return orm.Conversation{}, err
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

	err = s.insertSystemMessageTx(ctx, conversationID, operatorID, operatorName, "member_joined", newMemberIDs, targetUserIDs, func(tx ConversationStore) error {
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

		return nil
	})
	if err != nil {
		return orm.Conversation{}, err
	}

	s.postCommit(ctx, conversationID, rolesFromMembers)

	return s.GetConversationByID(ctx, conversationID)
}

func (s *ConversationService) RemoveGroupMembers(ctx context.Context, conversationID, operatorID int64, operatorName string, removeMemberIDs []int64) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}

	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	if err := s.requireAct(ctx, conversationID, operatorID, rbac.ActionGroupManage); err != nil {
		return err
	}
	operatorRole, err := s.memberRole(ctx, conversationID, operatorID)
	if err != nil {
		return err
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

	err = s.insertSystemMessageTx(ctx, conversationID, operatorID, operatorName, "member_removed", removeMemberIDs, memberUserIDs(members), func(tx ConversationStore) error {
		_, err := tx.RemoveConversationMembers(ctx, conversationID, removeMemberIDs)
		return err
	})
	if err != nil {
		return err
	}

	s.postCommit(ctx, conversationID, rolesFromMembers)

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

	err = s.insertSystemMessageTx(ctx, conversationID, userID, userName, "member_left", []int64{userID}, memberUserIDs(members), func(tx ConversationStore) error {
		_, err := tx.RemoveConversationMembers(ctx, conversationID, []int64{userID})
		return err
	})
	if err != nil {
		return err
	}

	s.postCommit(ctx, conversationID, rolesFromMembers)
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
	if err := s.requireAct(ctx, conversationID, operatorID, rbac.ActionGroupOwn); err != nil {
		return err
	}

	// Everyone in the group is notified before it is deactivated.
	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return err
	}

	err = s.insertSystemMessageTx(ctx, conversationID, operatorID, "", "group_dismissed", nil, memberUserIDs(members), func(tx ConversationStore) error {
		return tx.DeactivateConversation(ctx, conversationID)
	})
	if err != nil {
		return err
	}

	s.postCommit(ctx, conversationID, rolesReset)
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
	if err := s.requireAct(ctx, conversationID, operatorID, rbac.ActionGroupManage); err != nil {
		return orm.Conversation{}, err
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

	// A combined rename and avatar change is announced as the rename; the
	// avatar travels in the conversation row the client refetches.
	eventType := "group_avatar_changed"
	if name != nil {
		eventType = "group_renamed"
	}

	members, err := s.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}

	err = s.insertSystemMessageTx(ctx, conversationID, operatorID, operatorName, eventType, nil, memberUserIDs(members), func(tx ConversationStore) error {
		return tx.UpdateConversation(ctx, conversationID, name, avatar)
	})
	if err != nil {
		return orm.Conversation{}, err
	}

	s.postCommit(ctx, conversationID, rolesFromMembers)
	return s.GetConversationByID(ctx, conversationID)
}

func (s *ConversationService) GrantGroupAdmin(ctx context.Context, conversationID int64, actor Actor, target Target) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	if err := s.requireAct(ctx, conversationID, int64(actor), rbac.ActionGroupOwn); err != nil {
		return err
	}
	if int64(target) == int64(actor) {
		return errorx.NewCodeError(errorx.CodeBadInput, "owner cannot grant admin to self")
	}
	targetRole, err := s.memberRole(ctx, conversationID, int64(target))
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
	rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, int64(target), RoleAdmin)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.postCommit(ctx, conversationID, rolesFromMembers)
	return nil
}

func (s *ConversationService) RevokeGroupAdmin(ctx context.Context, conversationID int64, actor Actor, target Target) error {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return ErrNotGroupConversation
	}
	if err := s.requireAct(ctx, conversationID, int64(actor), rbac.ActionGroupOwn); err != nil {
		return err
	}
	if int64(target) == int64(actor) {
		return errorx.NewCodeError(errorx.CodeBadInput, "owner cannot revoke self")
	}
	targetRole, err := s.memberRole(ctx, conversationID, int64(target))
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
	rows, err := s.store.UpdateConversationMemberRole(ctx, conversationID, int64(target), RoleMember)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotMember
	}
	s.postCommit(ctx, conversationID, rolesFromMembers)
	return nil
}

func (s *ConversationService) TransferGroupOwner(ctx context.Context, conversationID int64, actor Actor, target Target) (orm.Conversation, error) {
	conv, err := s.GetConversationByID(ctx, conversationID)
	if err != nil {
		return orm.Conversation{}, err
	}
	if conv.ConversationType != ConversationTypeGroup {
		return orm.Conversation{}, ErrNotGroupConversation
	}
	if err := s.requireAct(ctx, conversationID, int64(actor), rbac.ActionGroupOwn); err != nil {
		return orm.Conversation{}, err
	}
	if int64(target) == int64(actor) {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "new owner must be a different member")
	}
	targetRole, err := s.memberRole(ctx, conversationID, int64(target))
	if err != nil {
		return orm.Conversation{}, err
	}
	if targetRole == RoleOwner {
		return orm.Conversation{}, ErrNotOwner
	}
	if targetRole != RoleMember && targetRole != RoleAdmin {
		return orm.Conversation{}, ErrNotMember
	}

	isBot, err := s.isBot(ctx, int64(target))
	if err != nil {
		return orm.Conversation{}, err
	}
	if isBot {
		return orm.Conversation{}, errorx.NewCodeError(errorx.CodeBadInput, "bot cannot be group owner")
	}

	err = s.withStore(ctx, func(store ConversationStore) error {
		if rows, err := store.UpdateConversationMemberRole(ctx, conversationID, int64(actor), RoleAdmin); err != nil {
			return err
		} else if rows == 0 {
			return ErrNotMember
		}
		if rows, err := store.UpdateConversationMemberRole(ctx, conversationID, int64(target), RoleOwner); err != nil {
			return err
		} else if rows == 0 {
			return ErrNotMember
		}
		return store.UpdateConversationCreator(ctx, conversationID, int64(target))
	})
	if err != nil {
		return orm.Conversation{}, err
	}
	s.postCommit(ctx, conversationID, rolesFromMembers)

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

func (s *ConversationService) UpdateReadReceipt(ctx context.Context, conversationID int64, actor Actor, lastReadMessageID int64) (ReadState, error) {
	if conversationID <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required and must be positive")
	}
	if actor <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}
	if lastReadMessageID <= 0 {
		return ReadState{}, errorx.NewCodeError(errorx.CodeBadInput, "last_read_message_id is required and must be positive")
	}

	if _, err := s.GetConversationByID(ctx, conversationID); err != nil {
		return ReadState{}, err
	}

	isMember, err := s.IsMember(ctx, conversationID, int64(actor))
	if err != nil {
		return ReadState{}, err
	}
	if !isMember {
		return ReadState{}, ErrNotMember
	}

	row, err := s.store.UpsertConversationReadState(ctx, conversationID, int64(actor), lastReadMessageID)
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
