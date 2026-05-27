package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

const (
	TopicAttachmentUploaded = "aim.attachment.uploaded"
	TopicAttachmentParsed   = "aim.attachment.parsed"
)

// AttachmentUploadedEvent is published after AIM confirms a client-direct
// SeaweedFS media upload and the attachment becomes eligible for parsing.
type AttachmentUploadedEvent struct {
	tracing.TraceContextFields

	FileID         string `json:"file_id"`
	OwnerID        int64  `json:"owner_id"`
	ConversationID int64  `json:"conversation_id"`
	Kind           string `json:"kind"`
	ObjectKey      string `json:"object_key"`
	Bucket         string `json:"bucket"`
	OriginalName   string `json:"original_name"`
	Mime           string `json:"mime"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256,omitempty"`
	UploadedAt     int64  `json:"uploaded_at"`
}

// AttachmentParsedEvent is published when data_parsing has extracted media
// metadata and written any thumbnail/derived objects back to SeaweedFS.
type AttachmentParsedEvent struct {
	tracing.TraceContextFields

	FileID             string         `json:"file_id"`
	Kind               string         `json:"kind"`
	ParseStatus        string         `json:"parse_status"`
	ThumbnailObjectKey string         `json:"thumbnail_object_key,omitempty"`
	ThumbnailFileID    string         `json:"thumbnail_file_id,omitempty"`
	DurationMS         int64          `json:"duration_ms,omitempty"`
	Width              int            `json:"width,omitempty"`
	Height             int            `json:"height,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	Error              string         `json:"error,omitempty"`
	ParsedAt           int64          `json:"parsed_at"`
}
