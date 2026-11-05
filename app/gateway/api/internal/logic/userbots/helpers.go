package userbots

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// setUserBotStatus flips a bot's enabled flag for its owner and returns the new state.
func setUserBotStatus(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	logger errorx.ErrorLogger,
	operation string,
	botUserID int64,
	enabled bool,
) (*pb.UserBotInfo, error) {
	ownerUserID, err := authctx.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := svcCtx.LogicBotClient.SetUserBotStatus(ctx, &botservice.SetUserBotStatusReq{
		OwnerUserId: ownerUserID, BotUserId: botUserID, Enabled: enabled,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(logger, operation, err)
	}

	return rpcResp.GetBot(), nil
}

func conversationResponseToType(conv *pb.ConversationResponse) types.ConversationItem {
	return types.ConversationItem{
		ConversationId:   conv.GetId(),
		ConversationType: conv.GetConversationType(),
		IsActive:         conv.GetIsActive(),
		CreatedAt:        conv.GetCreatedAt(),
		MemberIds:        conv.GetMemberIds(),
		Name:             conv.GetName(),
		Avatar:           conv.GetAvatar(),
		CreatorId:        conv.GetCreatorId(),
	}
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
