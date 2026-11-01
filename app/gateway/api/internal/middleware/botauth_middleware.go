package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/httperr"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// botAuthScheme is the value the BotAuth middleware expects in front of
// the plaintext token in the `Authorization` header.
const botAuthScheme = "Bot"

// BotAuthMiddleware authenticates Bot OpenAPI calls.
//
// It expects an `Authorization: Bot <token>` header. The token is resolved
// through the logic BotService gRPC, which performs hash lookup, expiry,
// revocation and bot-status checks server-side. On success the middleware
// attaches a BotIdentity to the request context.
type BotAuthMiddleware struct {
	client    botservice.BotService
	rpcDeadln time.Duration
}

// NewBotAuthMiddleware returns a middleware that validates Bot tokens via
// the supplied logic gRPC client. Pass nil to register the middleware in
// "always reject" mode (used by tests that do not boot the logic stack).
func NewBotAuthMiddleware(client botservice.BotService) *BotAuthMiddleware {
	return &BotAuthMiddleware{
		client:    client,
		rpcDeadln: 3 * time.Second,
	}
}

// Handle implements rest.Middleware.
func (m *BotAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := extractBotToken(r.Header.Get("Authorization"))
		if err != nil {
			httperr.Write(w, err)
			return
		}

		if m.client == nil {
			httperr.Write(w, errorx.NewCodeError(errorx.CodeInternal, "bot auth not configured"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), m.rpcDeadln)
		defer cancel()

		resp, err := m.client.ValidateBotToken(ctx, &botservice.ValidateBotTokenReq{PlaintextToken: token})
		if err != nil {
			httperr.Write(w, errorx.SanitizeGRPCErrorNoLog(err))
			return
		}

		identityPB := resp.GetIdentity()
		if identityPB == nil || identityPB.GetBotUserId() <= 0 {
			httperr.Write(w, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "invalid bot token"))
			return
		}

		identity := botctx.BotIdentity{
			BotUserID:  identityPB.GetBotUserId(),
			TokenID:    identityPB.GetTokenId(),
			Scopes:     identityPB.GetScopes(),
			Nickname:   identityPB.GetNickname(),
			Avatar:     identityPB.GetAvatar(),
			UserStatus: identityPB.GetUserStatus(),
		}

		next(w, r.WithContext(botctx.WithBotIdentity(r.Context(), identity)))
	}
}

// extractBotToken parses "Bot <token>" out of an Authorization header.
// Returns an error interface (not the concrete *errorx.CodeError) so that
// the typed-nil idiom does not bite callers that test the result with
// `require.NoError`.
func extractBotToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", errorx.NewCodeError(errorx.CodeBotTokenInvalid, "missing Authorization header")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], botAuthScheme) {
		return "", errorx.NewCodeError(errorx.CodeBotTokenInvalid, "Authorization scheme must be 'Bot'")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errorx.NewCodeError(errorx.CodeBotTokenInvalid, "empty bot token")
	}

	return token, nil
}
