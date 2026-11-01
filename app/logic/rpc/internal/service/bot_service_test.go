package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/stretchr/testify/require"
)

type fakeBotQuerier struct {
	tokenRow       dao.BotTokenWithUser
	tokenErr       error
	users          map[int64]orm.UserInfo
	convs          map[int64][]orm.Conversation
	webhook        *orm.BotWebhook
	upsertErr      error
	deleteRows     int64
	deleteErr      error
	tokenActions   []string
	eventAction    string
	eventActionErr error
}

func (f *fakeBotQuerier) GetBotTokenByHash(ctx context.Context, _ string) (dao.BotTokenWithUser, error) {
	if f.tokenErr != nil {
		return dao.BotTokenWithUser{}, f.tokenErr
	}
	return f.tokenRow, nil
}

func (f *fakeBotQuerier) ListEnabledActionsByToken(_ context.Context, _ int64) ([]string, error) {
	return f.tokenActions, nil
}

func (f *fakeBotQuerier) GetEnabledActionByWebhookEvent(_ context.Context, _ string) (string, error) {
	return f.eventAction, f.eventActionErr
}

func (f *fakeBotQuerier) GetUserInfoByID(_ context.Context, id int64) (orm.UserInfo, error) {
	u, ok := f.users[id]
	if !ok {
		return orm.UserInfo{}, dao.ErrNotFound
	}
	return u, nil
}

func (f *fakeBotQuerier) GetConversationsByUserID(_ context.Context, id int64) ([]orm.Conversation, error) {
	return f.convs[id], nil
}

func (f *fakeBotQuerier) GetBotWebhook(_ context.Context, _ int64) (orm.BotWebhook, error) {
	if f.webhook == nil {
		return orm.BotWebhook{}, dao.ErrNotFound
	}
	return *f.webhook, nil
}

func (f *fakeBotQuerier) UpsertBotWebhook(_ context.Context, botUserID int64, url, secretHash string, events []string, enabled bool) (orm.BotWebhook, error) {
	if f.upsertErr != nil {
		return orm.BotWebhook{}, f.upsertErr
	}
	row := orm.BotWebhook{
		BotUserID:  botUserID,
		Url:        url,
		SecretHash: secretHash,
		Events:     events,
		Enabled:    enabled,
		UpdatedAt:  time.Now(),
	}
	f.webhook = &row
	return row, nil
}

func (f *fakeBotQuerier) DeleteBotWebhook(_ context.Context, _ int64) (int64, error) {
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	return f.deleteRows, nil
}


func TestBotService_ValidateBotToken_Success(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: dao.BotTokenWithUser{
			BotToken: orm.BotToken{
				ID:        42,
				BotUserID: 1001,
				TokenHash: bottoken.Hash(tok),
			},
			UserType:   UserTypeBot,
			UserStatus: 1,
			Nickname:   "alice-bot",
			Avatar:     "https://example/a.png",
		},
		tokenActions: []string{"bot.message.send"},
	}

	identity, err := NewBotService(q, nil).ValidateBotToken(context.Background(), tok)
	require.NoError(t, err)
	require.Equal(t, int64(1001), identity.BotUserID)
	require.Equal(t, int64(42), identity.TokenID)
	// Scopes now come from bot_token_permissions + bot_actions, not bot_tokens.scopes.
	require.Equal(t, []string{"bot.message.send"}, identity.Scopes)
}

func TestBotService_ValidateBotToken_InvalidPlaintext(t *testing.T) {
	_, err := NewBotService(&fakeBotQuerier{}, nil).ValidateBotToken(context.Background(), "garbage")
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenInvalid, ce.Code)
}

func TestBotService_ValidateBotToken_NotFound(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	_, err = NewBotService(&fakeBotQuerier{tokenErr: dao.ErrNotFound}, nil).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenInvalid, ce.Code)
}

func TestBotService_ValidateBotToken_Revoked(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: dao.BotTokenWithUser{
			BotToken: orm.BotToken{
				ID:        42,
				BotUserID: 1001,
				TokenHash: bottoken.Hash(tok),
				RevokedAt: new(time.Now().Add(-time.Hour)),
			},
			UserType:   UserTypeBot,
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q, nil).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenRevoked, ce.Code)
}

func TestBotService_ValidateBotToken_Expired(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: dao.BotTokenWithUser{
			BotToken: orm.BotToken{
				ID:        42,
				BotUserID: 1001,
				TokenHash: bottoken.Hash(tok),
				ExpiresAt: new(time.Now().Add(-time.Minute)),
			},
			UserType:   UserTypeBot,
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q, nil).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenRevoked, ce.Code)
}

func TestBotService_ValidateBotToken_NotABot(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: dao.BotTokenWithUser{
			BotToken: orm.BotToken{
				ID:        42,
				BotUserID: 1001,
				TokenHash: bottoken.Hash(tok),
			},
			UserType:   "human",
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q, nil).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotDisabled, ce.Code)
}

func TestBotService_ValidateBotToken_Disabled(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: dao.BotTokenWithUser{
			BotToken: orm.BotToken{
				ID:        42,
				BotUserID: 1001,
				TokenHash: bottoken.Hash(tok),
			},
			UserType:   UserTypeBot,
			UserStatus: 0,
		},
	}
	_, err = NewBotService(q, nil).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotDisabled, ce.Code)
}

func TestBotService_GetBotProfile(t *testing.T) {
	q := &fakeBotQuerier{
		users: map[int64]orm.UserInfo{
			1001: {ID: 1001, Email: "alice-bot@aim", Nickname: "alice-bot", Status: 1, UserType: UserTypeBot},
		},
	}
	info, err := NewBotService(q, nil).GetBotProfile(context.Background(), 1001)
	require.NoError(t, err)
	require.Equal(t, "alice-bot", info.Nickname)

	_, err = NewBotService(q, nil).GetBotProfile(context.Background(), 9999)
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestBotService_ResolveWebhookEventActions(t *testing.T) {
	q := &fakeBotQuerier{eventAction: "bot.webhook.subscribe.message_created"}
	m, err := NewBotService(q, nil).ResolveWebhookEventActions(context.Background(), []string{"message.created"})
	require.NoError(t, err)
	require.Equal(t, "bot.webhook.subscribe.message_created", m["message.created"])
}

func TestBotService_ResolveWebhookEventActions_Unknown(t *testing.T) {
	q := &fakeBotQuerier{eventActionErr: dao.ErrNotFound}
	_, err := NewBotService(q, nil).ResolveWebhookEventActions(context.Background(), []string{"unknown.event"})
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotWebhookInvalid, ce.Code)
}

func TestBotService_SetBotWebhook_RequiresSecretWhenCreating(t *testing.T) {
	q := &fakeBotQuerier{eventAction: "bot.webhook.subscribe.message_created"}
	_, err := NewBotService(q, nil).SetBotWebhook(context.Background(), 1001, "https://x.test", "", []string{"message.created"}, true)
	require.Error(t, err)

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotWebhookInvalid, ce.Code)
}

func TestBotService_SetBotWebhook_KeepsExistingSecret(t *testing.T) {
	q := &fakeBotQuerier{
		eventAction: "bot.webhook.subscribe.message_created",
		webhook: &orm.BotWebhook{
			BotUserID:  1001,
			Url:        "https://old.test",
			SecretHash: "ABCDEF",
			Events:     []string{"message.created"},
			Enabled:    true,
		},
	}
	cfg, err := NewBotService(q, nil).SetBotWebhook(context.Background(), 1001, "https://new.test", "", nil, true)
	require.NoError(t, err)
	require.Equal(t, "https://new.test", cfg.URL)
	require.Equal(t, "ABCDEF", q.webhook.SecretHash)
}

func TestBotService_SetBotWebhook_HashesSecret(t *testing.T) {
	q := &fakeBotQuerier{eventAction: "bot.webhook.subscribe.message_created"}
	plain := "rotateme"
	cfg, err := NewBotService(q, nil).SetBotWebhook(context.Background(), 1001, "https://x.test", plain, []string{"message.created"}, true)
	require.NoError(t, err)
	require.Equal(t, "https://x.test", cfg.URL)
	require.Equal(t, bottoken.Hash(plain), q.webhook.SecretHash)
}

func TestBotService_DeleteBotWebhook(t *testing.T) {
	q := &fakeBotQuerier{deleteRows: 1}
	deleted, err := NewBotService(q, nil).DeleteBotWebhook(context.Background(), 1001)
	require.NoError(t, err)
	require.True(t, deleted)

	q.deleteRows = 0
	deleted, err = NewBotService(q, nil).DeleteBotWebhook(context.Background(), 1001)
	require.NoError(t, err)
	require.False(t, deleted)

	q.deleteErr = errors.New("boom")
	_, err = NewBotService(q, nil).DeleteBotWebhook(context.Background(), 1001)
	require.Error(t, err)
}
