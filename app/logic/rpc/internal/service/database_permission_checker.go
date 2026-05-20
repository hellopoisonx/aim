package service

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/jackc/pgx/v5"
)

const temporaryConversationMessageLimit int64 = 10

type DatabasePermissionChecker struct {
	queries model.Querier
}

func NewDatabasePermissionChecker(queries model.Querier) *DatabasePermissionChecker {
	return &DatabasePermissionChecker{queries: queries}
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

	// Check mute status
	if member.IsMuted {
		// Check if mute has expired
		if member.MutedUntil.Valid && member.MutedUntil.Time.Before(time.Now()) {
			// Mute expired, allow
			return PermissionDecision{Allowed: true, Code: CodeOK}, nil
		}

		if !member.MutedUntil.Valid {
			// Permanent mute
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}

		if member.MutedUntil.Time.After(time.Now()) {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}, nil
		}
	}

	return PermissionDecision{Allowed: true, Code: CodeOK}, nil
}

func (c *DatabasePermissionChecker) checkDirectPermission(ctx context.Context, check PermissionCheck) (PermissionDecision, error) {
	// Check friendship (bidirectional)
	friendships, err := c.queries.GetFriendshipBidirectional(ctx, model.GetFriendshipBidirectionalParams{
		UserID:   check.SenderID,
		FriendID: check.ConversationID,
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

		if count >= temporaryConversationMessageLimit {
			return PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "temporary conversation message limit reached"}, nil
		}

		return PermissionDecision{Allowed: true, Code: CodeOK, Reason: "temporary conversation"}, nil
	}

	return PermissionDecision{Allowed: true, Code: CodeOK}, nil
}
