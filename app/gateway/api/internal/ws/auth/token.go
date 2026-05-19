// Package wsauth provides JWT token extraction and validation for WebSocket connections.
package wsauth

import (
	"strings"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/jwt"
)

// ExtractToken extracts a JWT token from the Authorization header.
// Returns the token string and nil error if found, or empty string and an error if not found.
func ExtractToken(authHeader string) (string, *errorx.CodeError) {
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1]), nil
		}

		return "", errorx.NewCodeError(errorx.CodeTokenInvalid, "invalid authorization")
	}

	return "", errorx.NewCodeError(errorx.CodeTokenInvalid, "missing token")
}

// ValidateToken validates a JWT token string and returns the claims.
// Returns an error if validation fails.
func ValidateToken(tokenString string, secretKey string) (*jwt.Claims, *errorx.CodeError) {
	if tokenString == "" {
		return nil, errorx.NewCodeError(errorx.CodeTokenInvalid, "missing token")
	}

	// Simple JWT validation without a full JWT manager
	// We create a temporary manager to validate
	manager := jwt.NewManager(secretKey)

	claims, err := manager.ValidateAccessToken(tokenString)
	if err != nil {
		// Determine specific error type
		if strings.Contains(err.Error(), "token is expired") ||
			strings.Contains(err.Error(), "expired") {
			return nil, errorx.NewCodeError(errorx.CodeTokenExpired, "token expired")
		}

		return nil, errorx.NewCodeError(errorx.CodeTokenInvalid, "invalid token")
	}

	return claims, nil
}

// ExtractAndValidate combines extraction and validation.
func ExtractAndValidate(authHeader, secretKey string) (*jwt.Claims, *errorx.CodeError) {
	token, err := ExtractToken(authHeader)
	if err != nil {
		return nil, err
	}

	claims, err := ValidateToken(token, secretKey)
	if err != nil {
		return nil, err
	}

	return claims, nil
}
