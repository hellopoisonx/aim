package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// requireConversationOp validates the conversation id and returns the
// configured conversation service.
func requireConversationOp(convSvc service.ConversationQuerier, conversationID int64) (service.ConversationQuerier, error) {
	if conversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}

	return requireConversationService(convSvc)
}

// requireGroupOp validates the conversation and operator ids shared by the
// group-management handlers.
func requireGroupOp(convSvc service.ConversationQuerier, conversationID, operatorID int64) (service.ConversationQuerier, error) {
	if conversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_id is required")
	}
	if operatorID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "operator_id is required")
	}

	return requireConversationService(convSvc)
}

// requireGroupMembersOp adds the non-empty member_ids rule for the handlers
// that add or remove members.
func requireGroupMembersOp(convSvc service.ConversationQuerier, conversationID, operatorID int64, memberIDs []int64) (service.ConversationQuerier, error) {
	svc, err := requireGroupOp(convSvc, conversationID, operatorID)
	if err != nil {
		return nil, err
	}
	if len(memberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	return svc, nil
}

func requireConversationService(convSvc service.ConversationQuerier) (service.ConversationQuerier, error) {
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	return convSvc, nil
}

func conversationMemberIDs(ctx context.Context, convSvc service.ConversationQuerier, conversationID int64) ([]int64, error) {
	members, err := convSvc.GetConversationMembers(ctx, conversationID)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	memberIDs := make([]int64, len(members))
	for i, m := range members {
		memberIDs[i] = m.UserID
	}

	return memberIDs, nil
}
