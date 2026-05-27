package attachment

import (
	"encoding/json"
	"fmt"
)

const (
	// ContentSchemaV1 is the stable schema marker stored in message content.
	ContentSchemaV1 = "aim.attachment.v1"

	KindImage = "image"
	KindVideo = "video"
	KindAudio = "audio"
	KindFile  = "file"

	ParseStatusPending = "pending"
	ParseStatusReady   = "ready"
	ParseStatusFailed  = "failed"
)

// Content is the JSON shape carried by message.content for image/video/audio/file
// chat attachments. The value is serialized as a string on protobuf/WebSocket
// boundaries, and should be stored as a JSON object in PostgreSQL JSONB.
type Content struct {
	Schema          string          `json:"schema"`
	FileID          string          `json:"file_id"`
	Kind            string          `json:"kind"`
	Original        OriginalObject  `json:"original"`
	ThumbnailFileID string          `json:"thumbnail_file_id,omitempty"`
	ParseStatus     string          `json:"parse_status"`
	DurationMS      int64           `json:"duration_ms,omitempty"`
	Width           int             `json:"width,omitempty"`
	Height          int             `json:"height,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

// OriginalObject describes the user-uploaded source object.
type OriginalObject struct {
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

// IsAttachmentMessageType reports whether message_type should carry Content.
func IsAttachmentMessageType(messageType string) bool {
	switch messageType {
	case KindImage, KindVideo, KindAudio, KindFile:
		return true
	default:
		return false
	}
}

// ValidKind reports whether kind is a supported chat attachment kind.
func ValidKind(kind string) bool {
	return IsAttachmentMessageType(kind)
}

// RequiresDataParsing reports whether an uploaded attachment kind should enter
// the asynchronous media parsing pipeline.
func RequiresDataParsing(kind string) bool {
	switch kind {
	case KindImage, KindVideo, KindAudio:
		return true
	default:
		return false
	}
}

// ParseContent decodes and validates an aim.attachment.v1 JSON content string.
func ParseContent(raw string) (*Content, error) {
	var c Content
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("attachment content must be JSON object: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Marshal serializes content to the canonical message.content representation.
func (c Content) Marshal() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Validate enforces the product boundary for chat attachments.
func (c Content) Validate() error {
	if c.Schema != ContentSchemaV1 {
		return fmt.Errorf("unsupported attachment schema %q", c.Schema)
	}
	if c.FileID == "" {
		return fmt.Errorf("file_id is required")
	}
	if !ValidKind(c.Kind) {
		return fmt.Errorf("unsupported attachment kind %q", c.Kind)
	}
	if c.Original.Size <= 0 {
		return fmt.Errorf("original.size must be positive")
	}
	if c.Original.Mime == "" {
		return fmt.Errorf("original.mime is required")
	}
	if c.ParseStatus == "" {
		return fmt.Errorf("parse_status is required")
	}
	return nil
}
