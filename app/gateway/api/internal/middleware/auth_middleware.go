package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// AuthMiddleware validates JWT tokens on protected REST endpoints.
// On success, it injects ws.Identity into the request context so downstream
// logic can extract UserID and DeviceID via ws.IdentityFromContext.
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

		claims, codeErr := wsauth.ExtractAndValidate(authHeader, m.secretKey)
		if codeErr != nil {
			writeAuthError(w, codeErr)
			return
		}

		identity := ws.Identity{
			UserID:   claims.UserID,
			DeviceID: claims.DeviceID,
		}

		ctx := ws.WithIdentity(r.Context(), identity)
		next(w, r.WithContext(ctx))
	}
}

// writeAuthError writes a JSON error response matching the project envelope.
func writeAuthError(w http.ResponseWriter, codeErr *errorx.CodeError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	_ = json.NewEncoder(w).Encode(struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{
		Code: codeErr.Code,
		Msg:  codeErr.Message,
	})
}