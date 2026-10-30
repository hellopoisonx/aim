package middleware

import (
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/httperr"
	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
)

// AuthMiddleware validates JWT tokens on protected REST endpoints.
// On success, it injects authctx.Identity into the request context so downstream
// logic can extract UserID and DeviceID via authctx.IdentityFromContext.
type AuthMiddleware struct {
	secretKey string
}

// NewAuthMiddleware creates an AuthMiddleware that validates tokens
// signed with the given secret key.
func NewAuthMiddleware(secretKey string) *AuthMiddleware {
	return &AuthMiddleware{secretKey: secretKey}
}

// Handle implements rest.Middleware. It extracts and validates the Bearer
// token from the Authorization header, injects the parsed identity into
// the request context, and forwards to the next handler.
// If the token is missing, invalid, or expired, it responds with 401
// and a JSON body matching the project error envelope: {code, msg}.
func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		claims, err := wsauth.ExtractAndValidate(authHeader, m.secretKey)
		if err != nil {
			httperr.Write(w, err)
			return
		}

		identity := authctx.Identity{
			UserID:   claims.UserID,
			DeviceID: claims.DeviceID,
		}

		ctx := authctx.WithIdentity(r.Context(), identity)
		next(w, r.WithContext(ctx))
	}
}
