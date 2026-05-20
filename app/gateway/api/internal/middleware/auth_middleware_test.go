package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gjwt "github.com/golang-jwt/jwt/v4"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecretKey = "test-secret-key"

func TestAuthMiddleware(t *testing.T) {
	is := assert.New(t)

	validToken, _, err := jwt.NewManager(testSecretKey).GenerateAccessToken(42, "device-abc")
	require.NoError(t, err)

	validTokenDifferentUser, _, err := jwt.NewManager(testSecretKey).GenerateAccessToken(99, "device-xyz")
	require.NoError(t, err)

	expiredToken, _, err := generateExpiredToken(testSecretKey, 42, "device-abc")
	require.NoError(t, err)

	wrongSecretToken, _, err := jwt.NewManager("wrong-secret").GenerateAccessToken(42, "device-abc")
	require.NoError(t, err)

	tests := []struct {
		name           string
		authHeader     string
		nextHandler    func(t *testing.T) http.HandlerFunc
		checkResponse  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:       "missing Authorization header",
			authHeader: "",
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("missing token", errResp.Msg)
			},
		},
		{
			name:       "invalid Bearer format - Basic auth",
			authHeader: "Basic abc123",
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("invalid authorization", errResp.Msg)
			},
		},
		{
			name:       "invalid Bearer format - no space",
			authHeader: "Bearerabc123",
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("invalid authorization", errResp.Msg)
			},
		},
		{
			name:       "empty Bearer token",
			authHeader: "Bearer ",
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("missing token", errResp.Msg)
			},
		},
		{
			name:       "expired token",
			authHeader: "Bearer " + expiredToken,
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenExpired, errResp.Code)
				is.Equal("token expired", errResp.Msg)
			},
		},
		{
			name:       "invalid token - wrong secret",
			authHeader: "Bearer " + wrongSecretToken,
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("invalid token", errResp.Msg)
			},
		},
		{
			name:       "invalid token - malformed",
			authHeader: "Bearer not.a.valid.token",
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called")
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusUnauthorized, rr.Code)
				is.Contains(rr.Header().Get("Content-Type"), "application/json")

				var errResp struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				}
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				is.NoError(err)
				is.Equal(errorx.CodeTokenInvalid, errResp.Code)
				is.Equal("invalid token", errResp.Msg)
			},
		},
		{
			name:       "valid token - passes through to next handler",
			authHeader: "Bearer " + validToken,
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					identity, ok := ws.IdentityFromContext(r.Context())
					is.True(ok, "identity should be present in context")
					is.Equal(int64(42), identity.UserID)
					is.Equal("device-abc", identity.DeviceID)

					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte("success"))
					is.NoError(err)
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusOK, rr.Code)
				is.Equal("success", rr.Body.String())
			},
		},
		{
			name:       "valid token - different user identity",
			authHeader: "Bearer " + validTokenDifferentUser,
			nextHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					identity, ok := ws.IdentityFromContext(r.Context())
					is.True(ok, "identity should be present in context")
					is.Equal(int64(99), identity.UserID)
					is.Equal("device-xyz", identity.DeviceID)

					w.WriteHeader(http.StatusOK)
				}
			},
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				is.Equal(http.StatusOK, rr.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewAuthMiddleware(testSecretKey)
			nextHandler := tt.nextHandler(t)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler := middleware.Handle(nextHandler)
			handler(rr, req)

			tt.checkResponse(t, rr)
		})
	}
}

func TestAuthMiddleware_WriteAuthError(t *testing.T) {
	is := assert.New(t)

	middleware := NewAuthMiddleware(testSecretKey)
	nextHandler := func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	rr := httptest.NewRecorder()
	handler := middleware.Handle(nextHandler)
	handler(rr, req)

	is.Equal(http.StatusUnauthorized, rr.Code)
	is.Equal("application/json", rr.Header().Get("Content-Type"))

	var errResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	is.NoError(err)
	is.Equal(errorx.CodeTokenInvalid, errResp.Code)
	is.Equal("invalid token", errResp.Msg)
}

func TestNewAuthMiddleware(t *testing.T) {
	is := assert.New(t)

	middleware := NewAuthMiddleware("my-secret")
	is.NotNil(middleware)
}

// generateExpiredToken creates a token that is already expired
func generateExpiredToken(secretKey string, userID int64, deviceID string) (string, int64, error) {
	// Create an expired token by manipulating time
	expiresAt := time.Now().Add(-1 * time.Hour) // expired 1 hour ago

	claims := &jwt.Claims{
		UserID:   userID,
		DeviceID: deviceID,
		RegisteredClaims: gjwt.RegisteredClaims{
			ExpiresAt: gjwt.NewNumericDate(expiresAt),
			IssuedAt:  gjwt.NewNumericDate(expiresAt.Add(-5 * time.Minute)),
			NotBefore: gjwt.NewNumericDate(expiresAt.Add(-5 * time.Minute)),
		},
	}

	token := gjwt.NewWithClaims(gjwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secretKey))
	if err != nil {
		return "", 0, err
	}

	return signedToken, expiresAt.Unix(), nil
}
