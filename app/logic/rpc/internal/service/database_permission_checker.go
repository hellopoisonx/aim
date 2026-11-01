package service

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedcache "github.com/hellopoisonx/aim/app/shared/cache"
)

const DefaultTemporaryConversationMessageLimit int64 = 10

// temporaryConversationMessageLimit 保留为默认值别名，供现有测试使用。
//
//nolint:unused // package tests reference this constant when golangci-lint runs with tests enabled.
const temporaryConversationMessageLimit = DefaultTemporaryConversationMessageLimit

// PermissionQuerier is the DAO slice used by the permission checker.
type PermissionQuerier interface {
	GetConversation(ctx context.Context, id int64) (orm.Conversation, error)
	GetConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error)
	GetFriendshipBidirectional(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error)
	CountMessagesByConversation(ctx context.Context, conversationID int64) (int64, error)
	GetUserType(ctx context.Context, id int64) (string, error)
}

type DatabasePermissionChecker struct {
	queries                           PermissionQuerier
	temporaryConversationMessageLimit int64
	convCache                         *sharedcache.TypedCache[orm.Conversation]
	membersCache                      *sharedcache.TypedCache[[]orm.ConversationMember]
	friendshipCache                   *sharedcache.TypedCache[[]orm.Friendship]
	userTypeCache                     *sharedcache.TypedCache[string]
}

type DatabasePermissionCheckerCaches struct {
	Conversation *sharedcache.TypedCache[orm.Conversation]
	Members      *sharedcache.TypedCache[[]orm.ConversationMember]
	Friendship   *sharedcache.TypedCache[[]orm.Friendship]
	UserType     *sharedcache.TypedCache[string]
}

func NewDatabasePermissionChecker(queries PermissionQuerier) *DatabasePermissionChecker {
	return NewDatabasePermissionCheckerWithLimit(queries, DefaultTemporaryConversationMessageLimit)
}

// NewDatabasePermissionCheckerWithLimit 允许调用方自定义临时会话消息上限。
// limit <= 0 表示不限制（仅供开发/压测使用）。
func NewDatabasePermissionCheckerWithLimit(queries PermissionQuerier, limit int64) *DatabasePermissionChecker {
	return NewDatabasePermissionCheckerWithLimitAndCaches(queries, limit, DatabasePermissionCheckerCaches{})
}

func NewDatabasePermissionCheckerWithLimitAndCaches(queries PermissionQuerier, limit int64, caches DatabasePermissionCheckerCaches) *DatabasePermissionChecker {
	return &DatabasePermissionChecker{
		queries:                           queries,
		temporaryConversationMessageLimit: limit,
		convCache:                         caches.Conversation,
		membersCache:                      caches.Members,
		friendshipCache:                   caches.Friendship,
		userTypeCache:                     caches.UserType,
	}
}

func (c *DatabasePermissionChecker) CheckMessagePermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	// 1. Get conversation
	conv, err := c.getConversation(ctx, check.ConversationID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return PermissionDecision{Allowed: false, Code: CodeNotFound, Reason: "conversation not found"}, nil
		}

		return PermissionDecision{}, err
	}

	if !conv.IsActive {
		return PermissionDecision{Allowed: false, Code: CodeNotFound, Reason: "conversation is not active"}, nil
	}

	// 2. Check rules based on conversation type
	if conv.ConversationType == ConversationTypeGroup {
		return c.checkGroupPermission(ctx, check)
	}

	return c.checkDirectPermission(ctx, check)
}

func (c *DatabasePermissionChecker) checkGroupPermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	members, err := c.getConversationMembers(ctx, check.ConversationID)
	if err != nil {
		return PermissionDecision{}, err
	}
	var member orm.ConversationMember
	found := false
	for _, m := range members {
		if m.UserID == check.SenderID {
			member = m
			found = true
			break
		}
	}
	if !found {
		return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is not a member of this group"}, nil
	}

	// Filter mentions to only include conversation members
	filtered, err := c.filterMentionsToMembers(ctx, check)
	if err != nil {
		return PermissionDecision{}, err
	}

	// Check mute status
	if member.IsMuted {
		// Check if mute has expired
		if member.MutedUntil != nil && member.MutedUntil.Before(time.Now()) {
			// Mute expired, allow
			return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
		}

		if member.MutedUntil == nil {
			// Permanent mute
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}

		if member.MutedUntil.After(time.Now()) {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}
	}

	return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
}

func (c *DatabasePermissionChecker) checkDirectPermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	members, err := c.getConversationMembers(ctx, check.ConversationID)
	if err != nil {
		return PermissionDecision{}, err
	}

	peerID, senderIsMember, validDirectMembers := directConversationPeerID(members, check.SenderID)
	if !senderIsMember {
		return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is not a member of this direct conversation"}, nil
	}

	if !validDirectMembers {
		return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "invalid direct conversation members"}, nil
	}

	// Filter mentions to only include conversation members
	filtered := filterMentionsByMemberIDs(check.Mentions, memberIDsFromGetConversationMembersRow(members))

	// Check friendship (bidirectional) between the direct conversation members.
	friendships, err := c.getFriendships(ctx, check.SenderID, peerID)
	if err != nil {
		return PermissionDecision{}, err
	}

	hasAccepted := false
	hasBlocked := false

	for _, f := range friendships {
		switch f.Status {
		case FriendshipStatusBlocked:
			hasBlocked = true
		case FriendshipStatusAccepted:
			hasAccepted = true
		}
	}

	if hasBlocked {
		return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "blocked"}, nil
	}

	if !hasAccepted {
		if c.temporaryConversationMessageLimit <= 0 {
			return PermissionDecision{Allowed: true, Code: CodeOK, Reason: "temporary conversation", FilteredMentions: filtered}, nil
		}

		count, err := c.queries.CountMessagesByConversation(ctx, check.ConversationID)
		if err != nil {
			return PermissionDecision{}, err
		}

		if count >= c.temporaryConversationMessageLimit {
			hasBot, err := c.directConversationHasBot(ctx, check.SenderID, peerID)
			if err != nil {
				return PermissionDecision{}, err
			}

			if hasBot {
				return PermissionDecision{Allowed: true, Code: CodeOK, Reason: "bot direct conversation", FilteredMentions: filtered}, nil
			}

			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "temporary conversation message limit reached"}, nil
		}

		return PermissionDecision{Allowed: true, Code: CodeOK, Reason: "temporary conversation", FilteredMentions: filtered}, nil
	}

	return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
}

func (c *DatabasePermissionChecker) directConversationHasBot(ctx context.Context, senderID, peerID int64) (bool, error) {
	for _, userID := range []int64{senderID, peerID} {
		userType, err := c.getUserType(ctx, userID)
		if err != nil {
			if errors.Is(err, dao.ErrNotFound) {
				continue
			}

			return false, err
		}

		if userType == UserTypeBot {
			return true, nil
		}
	}

	return false, nil
}

func (c *DatabasePermissionChecker) getConversation(ctx context.Context, conversationID int64) (orm.Conversation, error) {
	if c.convCache == nil {
		return c.queries.GetConversation(ctx, conversationID)
	}

	return c.convCache.Take(ctx, sharedcache.ConversationKey(conversationID), func() (orm.Conversation, error) {
		return c.queries.GetConversation(ctx, conversationID)
	})
}

func (c *DatabasePermissionChecker) getConversationMembers(ctx context.Context, conversationID int64) ([]orm.ConversationMember, error) {
	if c.membersCache == nil {
		return c.queries.GetConversationMembers(ctx, conversationID)
	}

	return c.membersCache.Take(ctx, sharedcache.ConversationMembersKey(conversationID), func() ([]orm.ConversationMember, error) {
		return c.queries.GetConversationMembers(ctx, conversationID)
	})
}

func (c *DatabasePermissionChecker) getFriendships(ctx context.Context, userID, friendID int64) ([]orm.Friendship, error) {
	if c.friendshipCache == nil {
		return c.queries.GetFriendshipBidirectional(ctx, userID, friendID)
	}

	return c.friendshipCache.Take(ctx, sharedcache.FriendshipKey(userID, friendID), func() ([]orm.Friendship, error) {
		return c.queries.GetFriendshipBidirectional(ctx, userID, friendID)
	})
}

func (c *DatabasePermissionChecker) getUserType(ctx context.Context, userID int64) (string, error) {
	if c.userTypeCache == nil {
		return c.queries.GetUserType(ctx, userID)
	}

	return c.userTypeCache.Take(ctx, sharedcache.UserTypeKey(userID), func() (string, error) {
		return c.queries.GetUserType(ctx, userID)
	})
}

func directConversationPeerID(members []orm.ConversationMember, senderID int64) (peerID int64, senderIsMember bool, ok bool) {
	peerCount := 0

	for _, member := range members {
		if member.UserID == senderID {
			senderIsMember = true
			continue
		}

		peerID = member.UserID
		peerCount++
	}

	return peerID, senderIsMember, peerCount == 1
}

// filterMentionsToMembers queries conversation members and filters mentions to only
// include user IDs that belong to the conversation. Non-member mentions are silently
// removed — they remain in the message content as plain text but won't trigger notifications.
func (c *DatabasePermissionChecker) filterMentionsToMembers(ctx context.Context, check PermissionCheck) ([]int64, error) {
	if len(check.Mentions) == 0 {
		return nil, nil
	}

	memberRows, err := c.getConversationMembers(ctx, check.ConversationID)
	if err != nil {
		return nil, err
	}

	memberIDs := memberIDsFromGetConversationMembersRow(memberRows)
	return filterMentionsByMemberIDs(check.Mentions, memberIDs), nil
}

// filterMentionsByMemberIDs returns only those mention IDs that exist in memberIDs.
func filterMentionsByMemberIDs(mentions, memberIDs []int64) []int64 {
	if len(mentions) == 0 {
		return nil
	}

	memberSet := make(map[int64]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		memberSet[id] = struct{}{}
	}

	filtered := make([]int64, 0, len(mentions))
	for _, id := range mentions {
		if _, ok := memberSet[id]; ok {
			filtered = append(filtered, id)
		}
	}

	return filtered
}

func memberIDsFromGetConversationMembersRow(rows []orm.ConversationMember) []int64 {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	return ids
}
