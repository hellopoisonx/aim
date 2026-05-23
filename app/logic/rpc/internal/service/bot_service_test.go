package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

type fakeBotQuerier struct {
	tokenRow   model.GetBotTokenByHashRow
	tokenErr   error
	users      map[int64]model.UserInfo
	convs      map[int64][]model.GetConversationsByUserIDRow
	webhook    *model.BotWebhook
	upsertErr  error
	deleteRows int64
	deleteErr  error
}

func (f *fakeBotQuerier) GetBotTokenByHash(ctx context.Context, _ string) (model.GetBotTokenByHashRow, error) {
	if f.tokenErr != nil {
		return model.GetBotTokenByHashRow{}, f.tokenErr
	}
	return f.tokenRow, nil
}

func (f *fakeBotQuerier) GetUserInfoByID(_ context.Context, id int64) (model.UserInfo, error) {
	u, ok := f.users[id]
	if !ok {
		return model.UserInfo{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeBotQuerier) GetConversationsByUserID(_ context.Context, id int64) ([]model.GetConversationsByUserIDRow, error) {
	return f.convs[id], nil
}

func (f *fakeBotQuerier) GetBotWebhook(_ context.Context, _ int64) (model.BotWebhook, error) {
	if f.webhook == nil {
		return model.BotWebhook{}, pgx.ErrNoRows
	}
	return *f.webhook, nil
}

func (f *fakeBotQuerier) UpsertBotWebhook(_ context.Context, arg model.UpsertBotWebhookParams) (model.BotWebhook, error) {
	if f.upsertErr != nil {
		return model.BotWebhook{}, f.upsertErr
	}
	row := model.BotWebhook{
		BotUserID:  arg.BotUserID,
		Url:        arg.Url,
		SecretHash: arg.SecretHash,
		Events:     arg.Events,
		Enabled:    arg.Enabled,
		UpdatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
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
		tokenRow: model.GetBotTokenByHashRow{
			ID:         42,
			BotUserID:  1001,
			TokenHash:  bottoken.Hash(tok),
			Scopes:     []string{"messages:send"},
			UserType:   UserTypeBot,
			UserStatus: 1,
			Nickname:   "alice-bot",
			Avatar:     "https://example/a.png",
		},
	}

	identity, err := NewBotService(q).ValidateBotToken(context.Background(), tok)
	require.NoError(t, err)
	require.Equal(t, int64(1001), identity.BotUserID)
	require.Equal(t, int64(42), identity.TokenID)
	require.Equal(t, []string{"messages:send"}, identity.Scopes)
}

func TestBotService_ValidateBotToken_InvalidPlaintext(t *testing.T) {
	_, err := NewBotService(&fakeBotQuerier{}).ValidateBotToken(context.Background(), "garbage")
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenInvalid, ce.Code)
}

func TestBotService_ValidateBotToken_NotFound(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	_, err = NewBotService(&fakeBotQuerier{tokenErr: pgx.ErrNoRows}).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenInvalid, ce.Code)
}

func TestBotService_ValidateBotToken_Revoked(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: model.GetBotTokenByHashRow{
			ID:         42,
			BotUserID:  1001,
			TokenHash:  bottoken.Hash(tok),
			RevokedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
			UserType:   UserTypeBot,
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenRevoked, ce.Code)
}

func TestBotService_ValidateBotToken_Expired(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: model.GetBotTokenByHashRow{
			ID:         42,
			BotUserID:  1001,
			TokenHash:  bottoken.Hash(tok),
			ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			UserType:   UserTypeBot,
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotTokenRevoked, ce.Code)
}

func TestBotService_ValidateBotToken_NotABot(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: model.GetBotTokenByHashRow{
			ID:         42,
			BotUserID:  1001,
			TokenHash:  bottoken.Hash(tok),
			UserType:   "human",
			UserStatus: 1,
		},
	}
	_, err = NewBotService(q).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotDisabled, ce.Code)
}

func TestBotService_ValidateBotToken_Disabled(t *testing.T) {
	tok, err := bottoken.Generate()
	require.NoError(t, err)

	q := &fakeBotQuerier{
		tokenRow: model.GetBotTokenByHashRow{
			ID:         42,
			BotUserID:  1001,
			TokenHash:  bottoken.Hash(tok),
			UserType:   UserTypeBot,
			UserStatus: 0,
		},
	}
	_, err = NewBotService(q).ValidateBotToken(context.Background(), tok)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotDisabled, ce.Code)
}

func TestBotService_GetBotProfile(t *testing.T) {
	q := &fakeBotQuerier{
		users: map[int64]model.UserInfo{
			1001: {ID: 1001, Email: "alice-bot@aim", Nickname: "alice-bot", Status: 1, UserType: UserTypeBot},
		},
	}
	info, err := NewBotService(q).GetBotProfile(context.Background(), 1001)
	require.NoError(t, err)
	require.Equal(t, "alice-bot", info.Nickname)

	_, err = NewBotService(q).GetBotProfile(context.Background(), 9999)
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestBotService_SetBotWebhook_RequiresSecretWhenCreating(t *testing.T) {
	q := &fakeBotQuerier{}
	_, err := NewBotService(q).SetBotWebhook(context.Background(), 1001, "https://x.test", "", []string{"message.created"}, true)
	require.Error(t, err)
	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotWebhookInvalid, ce.Code)
}

func TestBotService_SetBotWebhook_KeepsExistingSecret(t *testing.T) {
	q := &fakeBotQuerier{
		webhook: &model.BotWebhook{
			BotUserID:  1001,
			Url:        "https://old.test",
			SecretHash: "ABCDEF",
			Events:     []string{"message.created"},
			Enabled:    true,
		},
	}
	cfg, err := NewBotService(q).SetBotWebhook(context.Background(), 1001, "https://new.test", "", nil, true)
	require.NoError(t, err)
	require.Equal(t, "https://new.test", cfg.URL)
	require.Equal(t, "ABCDEF", q.webhook.SecretHash)
}

func TestBotService_SetBotWebhook_HashesSecret(t *testing.T) {
	q := &fakeBotQuerier{}
	plain := "rotateme"
	cfg, err := NewBotService(q).SetBotWebhook(context.Background(), 1001, "https://x.test", plain, []string{"message.created"}, true)
	require.NoError(t, err)
	require.Equal(t, "https://x.test", cfg.URL)
	require.Equal(t, bottoken.Hash(plain), q.webhook.SecretHash)
}

func TestBotService_DeleteBotWebhook(t *testing.T) {
	q := &fakeBotQuerier{deleteRows: 1}
	deleted, err := NewBotService(q).DeleteBotWebhook(context.Background(), 1001)
	require.NoError(t, err)
	require.True(t, deleted)

	q.deleteRows = 0
	deleted, err = NewBotService(q).DeleteBotWebhook(context.Background(), 1001)
	require.NoError(t, err)
	require.False(t, deleted)

	q.deleteErr = errors.New("boom")
	_, err = NewBotService(q).DeleteBotWebhook(context.Background(), 1001)
	require.Error(t, err)
}
