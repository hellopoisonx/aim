package userbots

import (
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func sanitizeError(logger errorx.ErrorLogger, operation string, err error) error {
	return errorx.SanitizeGRPCError(logger, operation, err)
}

func userBotToType(b *pb.UserBotInfo) types.UserBotInfo {
	if b == nil {
		return types.UserBotInfo{}
	}
	return types.UserBotInfo{
		BotUserId:   b.GetBotUserId(),
		OwnerUserId: b.GetOwnerUserId(),
		Email:       b.GetEmail(),
		Nickname:    b.GetNickname(),
		Avatar:      b.GetAvatar(),
		Status:      b.GetStatus(),
		CreatedAt:   b.GetCreatedAt(),
		UpdatedAt:   b.GetUpdatedAt(),
	}
}

func userBotTokenToType(t *pb.UserBotTokenInfo) types.UserBotTokenInfo {
	if t == nil {
		return types.UserBotTokenInfo{}
	}
	return types.UserBotTokenInfo{
		TokenId:   t.GetTokenId(),
		BotUserId: t.GetBotUserId(),
		Name:      t.GetName(),
		Actions:   t.GetActions(),
		ExpiresAt: t.GetExpiresAt(),
		RevokedAt: t.GetRevokedAt(),
		CreatedAt: t.GetCreatedAt(),
	}
}

func botActionToType(a *pb.BotActionInfo) types.BotActionItem {
	if a == nil {
		return types.BotActionItem{}
	}
	return types.BotActionItem{
		Id:          a.GetId(),
		Action:      a.GetAction(),
		Description: a.GetDescription(),
	}
}

func botEventToType(e *pb.BotEventInfo) types.BotEventItem {
	if e == nil {
		return types.BotEventItem{}
	}
	return types.BotEventItem{
		Event:       e.GetEvent(),
		Action:      e.GetAction(),
		Description: e.GetDescription(),
	}
}
