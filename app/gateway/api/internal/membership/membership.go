// Package membership answers "is this caller a member of this conversation?"
// for gateway handlers.
//
// Conversation membership is owned by the logic service, so the gateway cannot
// check it locally; every endpoint that exposes conversation-scoped data must
// call RequireMember before touching that data.
package membership

import (
	"context"
	"slices"

	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// Actor names the caller kind used in the returned error message.
const (
	ActorUser = "user"
	ActorBot  = "bot"
)

// RequireMember returns nil when callerID belongs to conversationID.
//
// It returns a CodeForbidden error when the caller is not a member and a
// sanitized error when the membership lookup itself fails.
func RequireMember(
	ctx context.Context,
	logger errorx.ErrorLogger,
	client conversationservice.ConversationService,
	conversationID, callerID int64,
	actor string,
) error {
	if conversationID <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "conversation id must be positive")
	}

	resp, err := client.GetConversationMembers(ctx, &conversationservice.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return errorx.SanitizeGRPCError(logger, "get conversation members", err)
	}

	if !slices.Contains(resp.GetMemberIds(), callerID) {
		return errorx.NewCodeErrorf(errorx.CodeForbidden, "%s is not a member of this conversation", actor)
	}

	return nil
}
