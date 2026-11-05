package botservicelogic

import (
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/stretchr/testify/require"
)

func TestUserBotToProto(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)
	updated := time.UnixMilli(1_700_000_060_000)

	got := userBotToProto(service.UserBotInfo{
		BotUserID:   7,
		OwnerUserID: 3,
		Email:       "bot@example.com",
		Nickname:    "bot",
		Avatar:      "a.png",
		Status:      1,
		CreatedAt:   created,
		UpdatedAt:   updated,
	})

	require.Equal(t, &pb.UserBotInfo{
		BotUserId:   7,
		OwnerUserId: 3,
		Email:       "bot@example.com",
		Nickname:    "bot",
		Avatar:      "a.png",
		Status:      1,
		CreatedAt:   created.UnixMilli(),
		UpdatedAt:   updated.UnixMilli(),
	}, got)
}

func TestUserBotTokenToProto(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)
	expires := time.UnixMilli(1_700_086_400_000)
	revoked := time.UnixMilli(1_700_043_200_000)

	tests := []struct {
		name  string
		token service.UserBotTokenInfo
		want  *pb.UserBotTokenInfo
	}{
		{
			name: "maps every field including timestamps",
			token: service.UserBotTokenInfo{
				TokenID:   21,
				BotUserID: 7,
				Name:      "ci",
				Actions:   []string{"bot.message.send"},
				ExpiresAt: expires,
				RevokedAt: revoked,
				CreatedAt: created,
			},
			want: &pb.UserBotTokenInfo{
				TokenId:   21,
				BotUserId: 7,
				Name:      "ci",
				Actions:   []string{"bot.message.send"},
				ExpiresAt: expires.UnixMilli(),
				RevokedAt: revoked.UnixMilli(),
				CreatedAt: created.UnixMilli(),
			},
		},
		{
			name: "zero expires and revoked timestamps become zero, not the unix epoch",
			token: service.UserBotTokenInfo{
				TokenID:   22,
				BotUserID: 7,
				Name:      "",
				CreatedAt: created,
			},
			want: &pb.UserBotTokenInfo{
				TokenId:   22,
				BotUserId: 7,
				CreatedAt: created.UnixMilli(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, userBotTokenToProto(tt.token))
		})
	}
}

func TestBotActionToProto(t *testing.T) {
	require.Equal(t, &pb.BotActionInfo{
		Id:          5,
		Action:      "bot.message.send",
		Description: "send messages",
	}, botActionToProto(service.BotActionInfo{
		ID:          5,
		Action:      "bot.message.send",
		Description: "send messages",
	}))
}

func TestBotEventToProto(t *testing.T) {
	require.Equal(t, &pb.BotEventInfo{
		Event:       "message.created",
		Action:      "bot.webhook.subscribe.message_created",
		Description: "a message was created",
	}, botEventToProto(service.BotEventInfo{
		Event:       "message.created",
		Action:      "bot.webhook.subscribe.message_created",
		Description: "a message was created",
	}))
}
