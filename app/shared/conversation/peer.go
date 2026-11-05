package conversation

import "github.com/hellopoisonx/aim/app/shared/errorx"

// RequireDirectPeer validates the member_ids of a direct conversation request:
// exactly one peer that is positive and is not the creator.
func RequireDirectPeer(memberIDs []int64, creatorID int64) error {
	if len(memberIDs) != 1 {
		return errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id")
	}

	if memberIDs[0] <= 0 {
		return errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
	}

	if memberIDs[0] == creatorID {
		return errorx.NewCodeError(errorx.CodeBadInput, "direct conversation peer must not be creator")
	}

	return nil
}
