package api

import (
	"context"
	"net/http"
	"net/url"
)

type InitAttachmentUploadRequest struct {
	ConversationID int64  `json:"conversation_id"`
	Kind           string `json:"kind"`
	OriginalName   string `json:"original_name"`
	Mime           string `json:"mime"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256,omitempty"`
}

type InitAttachmentUploadResponse struct {
	FileID       string            `json:"file_id"`
	Bucket       string            `json:"bucket"`
	ObjectKey    string            `json:"object_key"`
	UploadURL    string            `json:"upload_url"`
	UploadMethod string            `json:"upload_method"`
	Headers      map[string]string `json:"headers,omitempty"`
	ExpiresAt    int64             `json:"expires_at"`
}

type CompleteAttachmentUploadRequest struct {
	SHA256 string `json:"sha256,omitempty"`
}

type AttachmentFileInfo struct {
	FileID             string         `json:"file_id"`
	OwnerID            int64          `json:"owner_id"`
	ConversationID     int64          `json:"conversation_id"`
	Kind               string         `json:"kind"`
	OriginalName       string         `json:"original_name"`
	Mime               string         `json:"mime"`
	Size               int64          `json:"size"`
	SHA256             string         `json:"sha256,omitempty"`
	Status             string         `json:"status"`
	ParseStatus        string         `json:"parse_status"`
	Bucket             string         `json:"bucket"`
	ObjectKey          string         `json:"object_key"`
	ThumbnailObjectKey string         `json:"thumbnail_object_key,omitempty"`
	DurationMS         int64          `json:"duration_ms,omitempty"`
	Width              int            `json:"width,omitempty"`
	Height             int            `json:"height,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type AttachmentDownloadResponse struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

func (c *Client) InitAttachmentUpload(ctx context.Context, req InitAttachmentUploadRequest, token string) (*InitAttachmentUploadResponse, error) {
	var out InitAttachmentUploadResponse
	err := c.do(ctx, http.MethodPost, "/api/attachments/init", req, token, &out)
	return &out, err
}

func (c *Client) CompleteAttachmentUpload(ctx context.Context, fileID string, req CompleteAttachmentUploadRequest, token string) (*AttachmentFileInfo, error) {
	var out AttachmentFileInfo
	err := c.do(ctx, http.MethodPost, "/api/attachments/"+url.PathEscape(fileID)+"/complete", req, token, &out)
	return &out, err
}

func (c *Client) GetAttachment(ctx context.Context, fileID, token string) (*AttachmentFileInfo, error) {
	var out AttachmentFileInfo
	err := c.do(ctx, http.MethodGet, "/api/attachments/"+url.PathEscape(fileID), nil, token, &out)
	return &out, err
}

func (c *Client) GetAttachmentDownload(ctx context.Context, fileID, token string) (*AttachmentDownloadResponse, error) {
	var out AttachmentDownloadResponse
	err := c.do(ctx, http.MethodGet, "/api/attachments/"+url.PathEscape(fileID)+"/download", nil, token, &out)
	return &out, err
}
