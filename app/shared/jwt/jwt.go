package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	// AccessTokenTTL is 5 minutes
	AccessTokenTTL = 5 * time.Minute
)

// Claims represents JWT claims with user_id and device_id
type Claims struct {
	UserID   int64  `json:"user_id"`
	DeviceID string `json:"device_id"`
	jwt.RegisteredClaims
}

// Manager handles JWT token operations
type Manager struct {
	secretKey []byte
}

// NewManager creates a new JWT Manager
func NewManager(secretKey string) *Manager {
	return &Manager{
		secretKey: []byte(secretKey),
	}
}

// GenerateAccessToken generates a new JWT access token
func (m *Manager) GenerateAccessToken(userID int64, deviceID string) (string, int64, error) {
	expiresAt := time.Now().Add(AccessTokenTTL)
	claims := &Claims{
		UserID:   userID,
		DeviceID: deviceID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", 0, err
	}

	return signedToken, expiresAt.Unix(), nil
}

// ValidateAccessToken validates and parses a JWT access token
func (m *Manager) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return m.secretKey, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
