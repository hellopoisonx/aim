package botsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const defaultUserAgent = "aim-bot-sdk-go/0.1"

type Client struct {
	baseURL    *url.URL
	basePath   string
	token      string
	httpClient *http.Client
	userAgent  string
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

func WithBasePath(basePath string) Option {
	return func(c *Client) {
		if basePath != "" {
			c.basePath = basePath
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			copy := *c.httpClient
			copy.Timeout = timeout
			c.httpClient = &copy
		}
	}
}

func NewClient(baseURL, token string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("baseURL is required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("token is required")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("baseURL must include scheme and host")
	}

	client := &Client{
		baseURL:    parsed,
		basePath:   "/api/bot/v1",
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		userAgent:  defaultUserAgent,
	}
	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

func (c *Client) GetMe(ctx context.Context) (*GetMeResponse, error) {
	return doJSON[GetMeResponse](ctx, c, http.MethodGet, "/me", nil, nil)
}

func (c *Client) ListConversations(ctx context.Context) (*ListConversationsResponse, error) {
	return doJSON[ListConversationsResponse](ctx, c, http.MethodGet, "/conversations", nil, nil)
}

func (c *Client) GetConversationHistory(ctx context.Context, req GetConversationHistoryRequest) (*GetConversationHistoryResponse, error) {
	q := make(url.Values)
	if req.CursorCreatedAt > 0 {
		q.Set("cursor_created_at", strconv.FormatInt(req.CursorCreatedAt, 10))
	}
	if req.CursorID != "" {
		q.Set("cursor_id", req.CursorID)
	}
	if req.Limit > 0 {
		q.Set("limit", strconv.FormatInt(int64(req.Limit), 10))
	}
	return doJSON[GetConversationHistoryResponse](ctx, c, http.MethodGet, "/conversations/"+url.PathEscape(req.ConversationID)+"/history", q, nil)
}

func (c *Client) GetConversationMembers(ctx context.Context, conversationID string) (*GetConversationMembersResponse, error) {
	return doJSON[GetConversationMembersResponse](ctx, c, http.MethodGet, "/conversations/"+url.PathEscape(conversationID)+"/members", nil, nil)
}

func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*SendMessageResponse, error) {
	return doJSON[SendMessageResponse](ctx, c, http.MethodPost, "/messages", nil, req)
}

func (c *Client) DownloadAttachment(ctx context.Context, fileID string) (*DownloadAttachmentResponse, error) {
	return doJSON[DownloadAttachmentResponse](ctx, c, http.MethodGet, "/attachments/"+url.PathEscape(fileID)+"/download", nil, nil)
}

func (c *Client) MarkRead(ctx context.Context, req MarkReadRequest) (*MarkReadResponse, error) {
	body := struct {
		LastReadMessageID string `json:"last_read_message_id"`
	}{LastReadMessageID: req.LastReadMessageID}
	return doJSON[MarkReadResponse](ctx, c, http.MethodPost, "/conversations/"+url.PathEscape(req.ConversationID)+"/read-receipt", nil, body)
}

func (c *Client) ListReadStates(ctx context.Context, conversationID string) (*ListReadStatesResponse, error) {
	return doJSON[ListReadStatesResponse](ctx, c, http.MethodGet, "/conversations/"+url.PathEscape(conversationID)+"/read-states", nil, nil)
}

func (c *Client) GetWebhook(ctx context.Context) (*GetWebhookResponse, error) {
	return doJSON[GetWebhookResponse](ctx, c, http.MethodGet, "/webhook", nil, nil)
}

func (c *Client) SetWebhook(ctx context.Context, req SetWebhookRequest) (*SetWebhookResponse, error) {
	return doJSON[SetWebhookResponse](ctx, c, http.MethodPut, "/webhook", nil, req)
}

func (c *Client) DeleteWebhook(ctx context.Context) (*DeleteWebhookResponse, error) {
	return doJSON[DeleteWebhookResponse](ctx, c, http.MethodDelete, "/webhook", nil, nil)
}

func doJSON[T any](ctx context.Context, c *Client, method string, relPath string, query url.Values, body any) (*T, error) {
	if c == nil {
		return nil, errors.New("client is nil")
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.urlFor(relPath, query), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope envelope[json.RawMessage]
	if len(data) > 0 {
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Code:       envelope.Code,
			Message:    envelope.Message,
			Body:       append([]byte(nil), data...),
		}
	}

	var out T
	if len(envelope.Body) == 0 || string(envelope.Body) == "null" {
		return &out, nil
	}
	if err := json.Unmarshal(envelope.Body, &out); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	return &out, nil
}

func (c *Client) urlFor(relPath string, query url.Values) string {
	u := *c.baseURL
	base := strings.TrimRight(c.basePath, "/")
	rel := strings.TrimLeft(relPath, "/")
	u.Path = path.Join(u.Path, base, rel)
	if strings.HasSuffix(relPath, "/") && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawQuery = query.Encode()
	return u.String()
}

type envelope[T any] struct {
	Code    int32  `json:"code"`
	Message string `json:"msg"`
	Body    T      `json:"body"`
}

type APIError struct {
	StatusCode int
	Code       int32
	Message    string
	Body       []byte
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != 0 || e.Message != "" {
		return fmt.Sprintf("aim bot api error: status=%d code=%d msg=%s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("aim bot api error: status=%d", e.StatusCode)
}

func IsCode(err error, code int32) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
