package service

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/jackc/pgx/v5"
)

const DefaultTemporaryConversationMessageLimit int64 = 10

// temporaryConversationMessageLimit 保留为默认值别名，供现有测试使用。
const temporaryConversationMessageLimit = DefaultTemporaryConversationMessageLimit

type DatabasePermissionChecker struct {
	queries                           model.Querier
	temporaryConversationMessageLimit int64
}

func NewDatabasePermissionChecker(queries model.Querier) *DatabasePermissionChecker {
	return NewDatabasePermissionCheckerWithLimit(queries, DefaultTemporaryConversationMessageLimit)
}

// NewDatabasePermissionCheckerWithLimit 允许调用方自定义临时会话消息上限。
// limit <= 0 表示不限制（仅供开发/压测使用）。
func NewDatabasePermissionCheckerWithLimit(queries model.Querier, limit int64) *DatabasePermissionChecker {
	return &DatabasePermissionChecker{queries: queries, temporaryConversationMessageLimit: limit}
}

func (c *DatabasePermissionChecker) CheckMessagePermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	// 1. Get conversation
	conv, err := c.queries.GetConversation(ctx, check.ConversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PermissionDecision{Allowed: false, Code: CodeNotFound, Reason: "conversation not found"}, nil
		}

		return PermissionDecision{}, err
	}

	if !conv.IsActive {
		return PermissionDecision{Allowed: false, Code: CodeNotFound, Reason: "conversation is not active"}, nil
	}

	// 2. Check rules based on conversation type
	if conv.ConversationType == "group" {
		return c.checkGroupPermission(ctx, check)
	}

	return c.checkDirectPermission(ctx, check)
}

func (c *DatabasePermissionChecker) checkGroupPermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	// Check membership
	member, err := c.queries.GetMember(ctx, model.GetMemberParams{
		ConversationID: check.ConversationID,
		UserID:         check.SenderID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is not a member of this group"}, nil
		}

		return PermissionDecision{}, err
	}

	// Filter mentions to only include conversation members
	filtered, err := c.filterMentionsToMembers(ctx, check)
	if err != nil {
		return PermissionDecision{}, err
	}

	// Check mute status
	if member.IsMuted {
		// Check if mute has expired
		if member.MutedUntil.Valid && member.MutedUntil.Time.Before(time.Now()) {
			// Mute expired, allow
			return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
		}

		if !member.MutedUntil.Valid {
			// Permanent mute
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}

		if member.MutedUntil.Time.After(time.Now()) {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}
	}

	return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
}

func (c *DatabasePermissionChecker) checkDirectPermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	members, err := c.queries.GetConversationMembers(ctx, check.ConversationID)
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
	friendships, err := c.queries.GetFriendshipBidirectional(ctx, model.GetFriendshipBidirectionalParams{
		UserID:   check.SenderID,
		FriendID: peerID,
	})
	if err != nil {
		return PermissionDecision{}, err
	}

	hasAccepted := false
	hasBlocked := false

	for _, f := range friendships {
		switch f.Status {
		case "blocked":
			hasBlocked = true
		case "accepted":
			hasAccepted = true
		}
	}

	if hasBlocked {
		return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "blocked"}, nil
	}

	if !hasAccepted {
		count, err := c.queries.CountMessagesByConversation(ctx, check.ConversationID)
		if err != nil {
			return PermissionDecision{}, err
		}

		if c.temporaryConversationMessageLimit > 0 && count >= c.temporaryConversationMessageLimit {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "temporary conversation message limit reached"}, nil
		}

		return PermissionDecision{Allowed: true, Code: CodeOK, Reason: "temporary conversation", FilteredMentions: filtered}, nil
	}

	return PermissionDecision{Allowed: true, Code: CodeOK, FilteredMentions: filtered}, nil
}

func directConversationPeerID(members []model.GetConversationMembersRow, senderID int64) (peerID int64, senderIsMember bool, ok bool) {
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

	memberRows, err := c.queries.GetConversationMembers(ctx, check.ConversationID)
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

func memberIDsFromGetConversationMembersRow(rows []model.GetConversationMembersRow) []int64 {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	return ids
}
