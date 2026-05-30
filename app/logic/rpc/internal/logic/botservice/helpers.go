package botservicelogic

import (
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
)

func userBotToProto(b service.UserBotInfo) *pb.UserBotInfo {
	return &pb.UserBotInfo{
		BotUserId:   b.BotUserID,
		OwnerUserId: b.OwnerUserID,
		Email:       b.Email,
		Nickname:    b.Nickname,
		Avatar:      b.Avatar,
		Status:      int32(b.Status),
		CreatedAt:   b.CreatedAt.UnixMilli(),
		UpdatedAt:   b.UpdatedAt.UnixMilli(),
	}
}

func userBotTokenToProto(t service.UserBotTokenInfo) *pb.UserBotTokenInfo {
	var expiresAt int64
	if !t.ExpiresAt.IsZero() {
		expiresAt = t.ExpiresAt.UnixMilli()
	}
	var revokedAt int64
	if !t.RevokedAt.IsZero() {
		revokedAt = t.RevokedAt.UnixMilli()
	}
	return &pb.UserBotTokenInfo{
		TokenId:   t.TokenID,
		BotUserId: t.BotUserID,
		Name:      t.Name,
		Actions:   t.Actions,
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
		CreatedAt: t.CreatedAt.UnixMilli(),
	}
}

func botActionToProto(a service.BotActionInfo) *pb.BotActionInfo {
	return &pb.BotActionInfo{Id: a.ID, Action: a.Action, Description: a.Description}
}

func botEventToProto(e service.BotEventInfo) *pb.BotEventInfo {
	return &pb.BotEventInfo{Event: e.Event, Action: e.Action, Description: e.Description}
}
