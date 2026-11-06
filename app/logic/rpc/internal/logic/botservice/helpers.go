package botservicelogic

import (
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// deadlineFromMillis converts the wire's unix-millisecond expiry; 0 means no
// expiry.
func deadlineFromMillis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}

	return time.UnixMilli(ms)
}

// requireBot returns the bot service after validating the bot user id.
func requireBot(c *svc.ServiceContext, botUserID int64) (*service.BotService, error) {
	botSvc, err := requireBotService(c)
	if err != nil {
		return nil, err
	}

	if botUserID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	return botSvc, nil
}

// requireBotService returns the configured bot service without inspecting an id.
func requireBotService(c *svc.ServiceContext) (*service.BotService, error) {
	if c.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	return c.BotService, nil
}

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
