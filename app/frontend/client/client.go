package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

// GatewayURL is the base URL for the REST API gateway.
// It should be configured before use.
var GatewayURL = "http://localhost:8888"

// Envelope represents the standard response envelope from the gateway.
type Envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Body json.RawMessage `json:"body,omitempty"`
}

// EnvelopeError returns an error when the envelope code is non-zero.
func (e *Envelope) EnvelopeError() error {
	if e.Code == 0 {
		return nil
	}

	return errorx.NewCodeError(e.Code, e.Msg)
}

// RegisterRequest mirrors gateway.api RegisterRequest.
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Username string `json:"username,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	DeviceId string `json:"device_id" validate:"required"`
}

// RegisterResponse mirrors gateway.api RegisterResponse.
type RegisterResponse struct {
	UserId int64 `json:"user_id"`
}

// LoginRequest mirrors gateway.api LoginRequest.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	DeviceId string `json:"device_id" validate:"required"`
}

// LoginResponse mirrors gateway.api LoginResponse.
type LoginResponse struct {
	UserId       int64  `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// RefreshRequest mirrors gateway.api RefreshRequest.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshResponse mirrors gateway.api RefreshResponse.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// LogoutResponse mirrors gateway.api LogoutResponse.
type LogoutResponse struct {
	Success bool `json:"success"`
}

// RESTClient handles HTTP communication with the gateway.
type RESTClient struct {
	httpClient *http.Client
	gateway    string
}

// NewRESTClient creates a new REST client with default timeouts.
func NewRESTClient(gateway string) *RESTClient {
	gateway = strings.TrimRight(gateway, "/")
	if gateway == "" {
		gateway = GatewayURL
	}

	return &RESTClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		gateway: gateway,
	}
}

// doRequest performs an HTTP request with context and unmarshals the envelope.
func (c *RESTClient) doRequest(ctx context.Context, method, path string, body any, resp any, accessToken string) error {
	var (
		req *http.Request
		err error
	)

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		req, err = http.NewRequestWithContext(ctx, method, c.gateway+path, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.gateway+path, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
	}

	req.Header.Set("Content-Type", "application/json")

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := httpResp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	var envelope Envelope
	if err := json.NewDecoder(httpResp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}

	if err := envelope.EnvelopeError(); err != nil {
		return err
	}

	if resp != nil && len(envelope.Body) > 0 {
		if err := json.Unmarshal(envelope.Body, resp); err != nil {
			return fmt.Errorf("unmarshal body: %w", err)
		}
	}

	return nil
}

// Register calls POST /api/auth/register.
func (c *RESTClient) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/register", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Login calls POST /api/auth/login.
func (c *RESTClient) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	var resp LoginResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/login", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Refresh calls POST /api/auth/refresh.
func (c *RESTClient) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	var resp RefreshResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/refresh", req, &resp, ""); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Logout calls POST /api/auth/logout with Bearer access token.
func (c *RESTClient) Logout(ctx context.Context, accessToken string) (*LogoutResponse, error) {
	var resp LogoutResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/auth/logout", nil, &resp, accessToken); err != nil {
		return nil, err
	}

	return &resp, nil
}
