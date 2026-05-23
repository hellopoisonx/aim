package service

import (
	"context"
	"errors"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"

	"github.com/jackc/pgx/v5"
)

// UserTypeBot is the value stored in user_info.user_type for Bot identities.
const UserTypeBot = "bot"

// BotIdentity is the resolved view of a Bot Token, used by BotAuth.
type BotIdentity struct {
	BotUserID  int64
	TokenID    int64
	Scopes     []string
	Nickname   string
	Avatar     string
	UserStatus int16
}

// BotWebhookConfig is the service-level view of bot_webhooks (no plaintext secret).
type BotWebhookConfig struct {
	BotUserID int64
	URL       string
	Events    []string
	Enabled   bool
	UpdatedAt time.Time
}

// BotConversationItem is a thin projection of a conversation a bot belongs to.
type BotConversationItem struct {
	ConversationID   int64
	ConversationType string
	Name             string
	Avatar           string
	CreatedAt        time.Time
}

// BotQuerier is the slice of model.Querier the BotService needs.
// Defined as an interface so logic-level tests can mock it.
type BotQuerier interface {
	GetBotTokenByHash(ctx context.Context, tokenHash string) (model.GetBotTokenByHashRow, error)
	ListEnabledActionsByToken(ctx context.Context, tokenID int64) ([]string, error)
	GetEnabledActionByWebhookEvent(ctx context.Context, event string) (string, error)
	GetUserInfoByID(ctx context.Context, id int64) (model.UserInfo, error)
	GetConversationsByUserID(ctx context.Context, userID int64) ([]model.GetConversationsByUserIDRow, error)
	GetBotWebhook(ctx context.Context, botUserID int64) (model.BotWebhook, error)
	UpsertBotWebhook(ctx context.Context, arg model.UpsertBotWebhookParams) (model.BotWebhook, error)
	DeleteBotWebhook(ctx context.Context, botUserID int64) (int64, error)
}

// BotService implements BotService RPC business logic.
type BotService struct {
	queries BotQuerier
}

// NewBotService constructs a BotService backed by sqlc Querier.
func NewBotService(queries BotQuerier) *BotService {
	return &BotService{queries: queries}
}

// ValidateBotToken loads the row matching the plaintext token's SHA-256
// hash and applies all per-token gating rules:
//   - format must look like a bot token
//   - row must exist with matching hash
//   - revoked_at must be NULL
//   - expires_at must be in the future (or NULL)
//   - owner user_type must be "bot"
//   - owner status must equal 1 (enabled)
func (s *BotService) ValidateBotToken(ctx context.Context, plaintext string) (BotIdentity, error) {
	parsed, err := bottoken.ParsePlaintext(plaintext)
	if err != nil {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token")
	}

	row, err := s.queries.GetBotTokenByHash(ctx, bottoken.Hash(parsed))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token")
		}

		return BotIdentity{}, err
	}

	if row.RevokedAt.Valid {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token revoked")
	}

	if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenRevoked, "bot token expired")
	}

	if row.UserType != UserTypeBot {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotDisabled, "user is not a bot")
	}

	if row.UserStatus != 1 {
		return BotIdentity{}, errorx.NewCodeError(errorx.CodeBotDisabled, "bot is disabled")
	}

	actions, err := s.queries.ListEnabledActionsByToken(ctx, row.ID)
	if err != nil {
		return BotIdentity{}, err
	}

	validActions := make([]string, 0, len(actions))
	for _, action := range actions {
		action = botperm.NormalizeGrant(action)
		if !botperm.IsValidAction(action) {
			logx.WithContext(ctx).Errorf("ignoring invalid bot action from database: token_id=%d action=%q", row.ID, action)
			continue
		}

		validActions = append(validActions, action)
	}

	return BotIdentity{
		BotUserID:  row.BotUserID,
		TokenID:    row.ID,
		Scopes:     validActions,
		Nickname:   row.Nickname,
		Avatar:     row.Avatar,
		UserStatus: row.UserStatus,
	}, nil
}

// GetBotProfile loads the bot's user_info row and rejects when the row is
// not actually a bot (defence in depth — the BotAuth middleware should
// have caught this already).
func (s *BotService) GetBotProfile(ctx context.Context, botUserID int64) (model.UserInfo, error) {
	info, err := s.queries.GetUserInfoByID(ctx, botUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.UserInfo{}, ErrUserNotFound
		}

		return model.UserInfo{}, err
	}

	if info.UserType != UserTypeBot {
		return model.UserInfo{}, errorx.NewCodeError(errorx.CodeBotDisabled, "user is not a bot")
	}

	return info, nil
}

// ListBotConversations returns every conversation the bot is a member of.
func (s *BotService) ListBotConversations(ctx context.Context, botUserID int64) ([]BotConversationItem, error) {
	rows, err := s.queries.GetConversationsByUserID(ctx, botUserID)
	if err != nil {
		return nil, err
	}

	out := make([]BotConversationItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, BotConversationItem{
			ConversationID:   r.ID,
			ConversationType: r.ConversationType,
			Name:             r.Name,
			Avatar:           r.Avatar,
			CreatedAt:        r.CreatedAt.Time,
		})
	}

	return out, nil
}

// GetBotWebhook returns the current webhook config (without secret).
// Returns (zero, nil) when no webhook is configured.
func (s *BotService) GetBotWebhook(ctx context.Context, botUserID int64) (BotWebhookConfig, bool, error) {
	row, err := s.queries.GetBotWebhook(ctx, botUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BotWebhookConfig{}, false, nil
		}

		return BotWebhookConfig{}, false, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt.Time,
	}, true, nil
}

// ResolveWebhookEventActions maps webhook events to the enabled actions
// required to subscribe to them. Unknown or disabled mappings return
// CodeBotWebhookInvalid.
func (s *BotService) ResolveWebhookEventActions(ctx context.Context, events []string) (map[string]string, error) {
	if len(events) == 0 {
		events = []string{botperm.WebhookEventMessageCreated}
	}

	out := make(map[string]string, len(events))
	for _, event := range events {
		if event == "" {
			return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "webhook event is required")
		}

		action, err := s.queries.GetEnabledActionByWebhookEvent(ctx, event)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "unsupported webhook event: "+event)
			}

			return nil, err
		}

		if !botperm.IsValidAction(action) {
			logx.WithContext(ctx).Errorf("invalid webhook event action from database: event=%q action=%q", event, action)
			return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "invalid webhook event action")
		}

		out[event] = action
	}

	return out, nil
}

// SetBotWebhook upserts the configuration. The plaintext secret is hashed
// before persistence; pass empty `secret` only when the row already exists
// and the caller wants to keep the previous secret. If the row does not
// exist and `secret` is empty, the function returns CodeBotWebhookInvalid.
func (s *BotService) SetBotWebhook(ctx context.Context, botUserID int64, url string, secret string, events []string, enabled bool) (BotWebhookConfig, error) {
	if url == "" {
		return BotWebhookConfig{}, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "webhook url is required")
	}

	if _, err := s.ResolveWebhookEventActions(ctx, events); err != nil {
		return BotWebhookConfig{}, err
	}

	if len(events) == 0 {
		events = []string{botperm.WebhookEventMessageCreated}
	}

	var secretHash string
	if secret != "" {
		secretHash = bottoken.Hash(secret)
	} else {
		existing, err := s.queries.GetBotWebhook(ctx, botUserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return BotWebhookConfig{}, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "secret is required when creating webhook")
			}

			return BotWebhookConfig{}, err
		}
		secretHash = existing.SecretHash
	}

	row, err := s.queries.UpsertBotWebhook(ctx, model.UpsertBotWebhookParams{
		BotUserID:  botUserID,
		Url:        url,
		SecretHash: secretHash,
		Events:     events,
		Enabled:    enabled,
	})
	if err != nil {
		return BotWebhookConfig{}, err
	}

	return BotWebhookConfig{
		BotUserID: row.BotUserID,
		URL:       row.Url,
		Events:    append([]string{}, row.Events...),
		Enabled:   row.Enabled,
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// DeleteBotWebhook removes the row and returns true when a row was deleted.
func (s *BotService) DeleteBotWebhook(ctx context.Context, botUserID int64) (bool, error) {
	rows, err := s.queries.DeleteBotWebhook(ctx, botUserID)
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}
