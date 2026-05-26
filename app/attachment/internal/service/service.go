package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeromicro/go-queue/kq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var ErrNotFound = errors.New("attachment not found")

type Service struct {
	cfg    config.Config
	db     *pgxpool.Pool
	signer S3Signer
	pusher *kq.Pusher
}

type InitUploadRequest struct {
	OwnerID        int64  `json:"owner_id"`
	ConversationID int64  `json:"conversation_id"`
	Kind           string `json:"kind"`
	OriginalName   string `json:"original_name"`
	Mime           string `json:"mime"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256,omitempty"`
}

type InitUploadResponse struct {
	FileID       string            `json:"file_id"`
	Bucket       string            `json:"bucket"`
	ObjectKey    string            `json:"object_key"`
	UploadURL    string            `json:"upload_url"`
	UploadMethod string            `json:"upload_method"`
	Headers      map[string]string `json:"headers,omitempty"`
	ExpiresAt    int64             `json:"expires_at"`
}

type CompleteUploadRequest struct {
	UserID string `json:"-"`
	SHA256 string `json:"sha256,omitempty"`
}

type FileInfo struct {
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

type DownloadResponse struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt int64             `json:"expires_at"`
}

func New(ctx context.Context, cfg config.Config) (*Service, error) {
	var pool *pgxpool.Pool
	var err error
	if cfg.Postgres.DataSource != "" {
		pool, err = pgxpool.New(ctx, cfg.Postgres.DataSource)
		if err != nil {
			return nil, err
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, err
		}
	}
	var pusher *kq.Pusher
	if len(cfg.Kafka.Brokers) > 0 && cfg.Kafka.Topic != "" {
		pusher = kq.NewPusher(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	}
	return &Service{
		cfg: cfg,
		db:  pool,
		signer: S3Signer{
			Endpoint:       cfg.Seaweed.Endpoint,
			PublicEndpoint: cfg.Seaweed.PublicEndpoint,
			Region:         cfg.Seaweed.Region,
			AccessKey:      cfg.Seaweed.AccessKey,
			SecretKey:      cfg.Seaweed.SecretKey,
			Bucket:         cfg.Seaweed.Bucket,
		},
		pusher: pusher,
	}, nil
}

func (s *Service) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Service) InitUpload(ctx context.Context, req InitUploadRequest) (*InitUploadResponse, error) {
	if req.OwnerID <= 0 || req.ConversationID <= 0 {
		return nil, fmt.Errorf("owner_id and conversation_id are required")
	}
	if !sharedattachment.ValidKind(req.Kind) {
		return nil, fmt.Errorf("unsupported kind")
	}
	if req.Size <= 0 || req.Size > s.cfg.Seaweed.MaxSize() {
		return nil, fmt.Errorf("size must be between 1 and %d bytes", s.cfg.Seaweed.MaxSize())
	}
	if !allowedMime(req.Kind, req.Mime) {
		return nil, fmt.Errorf("mime %q is not allowed for %s", req.Mime, req.Kind)
	}
	fileID := uuid.New()
	key := objectKey(req.ConversationID, fileID.String(), "original")
	url, headers, err := s.signer.Presign(http.MethodPut, key, s.cfg.Seaweed.UploadTTL(), map[string]string{"content-type": req.Mime})
	if err != nil {
		return nil, err
	}
	if s.db != nil {
		_, err = s.db.Exec(ctx, `INSERT INTO attachment_files(file_id, owner_id, conversation_id, kind, original_name, mime, size_bytes, sha256, status, parse_status)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending','pending')`, fileID, req.OwnerID, req.ConversationID, req.Kind, req.OriginalName, req.Mime, req.Size, nilIfEmpty(req.SHA256))
		if err != nil {
			return nil, err
		}
		_, err = s.db.Exec(ctx, `INSERT INTO attachment_objects(file_id, object_role, bucket, object_key, mime, size_bytes, sha256)
VALUES ($1,'original',$2,$3,$4,$5,$6)`, fileID, s.cfg.Seaweed.Bucket, key, req.Mime, req.Size, nilIfEmpty(req.SHA256))
		if err != nil {
			return nil, err
		}
	}
	return &InitUploadResponse{FileID: fileID.String(), Bucket: s.cfg.Seaweed.Bucket, ObjectKey: key, UploadURL: url, UploadMethod: http.MethodPut, Headers: headers, ExpiresAt: time.Now().Add(s.cfg.Seaweed.UploadTTL()).UnixMilli()}, nil
}

func (s *Service) CompleteUpload(ctx context.Context, userID int64, fileID string, req CompleteUploadRequest) (*FileInfo, error) {
	id, err := uuid.Parse(fileID)
	if err != nil {
		return nil, err
	}
	if s.db != nil {
		pending, err := s.getPendingOriginal(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if err := s.verifyObject(ctx, pending.ObjectKey, pending.Size); err != nil {
			return nil, err
		}
		cmd, err := s.db.Exec(ctx, `UPDATE attachment_files SET status='uploaded', uploaded_at=NOW(), updated_at=NOW(), sha256=COALESCE(NULLIF($3,''), sha256) WHERE file_id=$1 AND owner_id=$2 AND status='pending'`, id, userID, req.SHA256)
		if err != nil {
			return nil, err
		}
		if cmd.RowsAffected() == 0 {
			return nil, ErrNotFound
		}
	}
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	_ = s.publishUploaded(ctx, info)
	return info, nil
}

func (s *Service) getPendingOriginal(ctx context.Context, userID int64, fileID uuid.UUID) (*FileInfo, error) {
	row := s.db.QueryRow(ctx, `SELECT f.file_id::text, f.owner_id, f.conversation_id, f.kind, f.original_name, f.mime, f.size_bytes, COALESCE(f.sha256,''), f.status, f.parse_status, o.bucket, o.object_key
FROM attachment_files f
JOIN attachment_objects o ON o.file_id=f.file_id AND o.object_role='original'
WHERE f.file_id=$1 AND f.owner_id=$2 AND f.status='pending'`, fileID, userID)
	info := FileInfo{}
	if err := row.Scan(&info.FileID, &info.OwnerID, &info.ConversationID, &info.Kind, &info.OriginalName, &info.Mime, &info.Size, &info.SHA256, &info.Status, &info.ParseStatus, &info.Bucket, &info.ObjectKey); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *Service) verifyObject(ctx context.Context, key string, expectedSize int64) error {
	ctx, span := otel.Tracer("github.com/hellopoisonx/aim/app/attachment").Start(
		ctx,
		"attachment.seaweed.verify_object",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("seaweed.object_key", key)),
	)
	defer span.End()

	url, headers, err := s.signer.PresignInternal(http.MethodHead, key, time.Minute, nil)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("uploaded object not found: status %d", resp.StatusCode)
		tracing.RecordSpanError(span, err)
		return err
	}
	if resp.ContentLength > 0 && expectedSize > 0 && resp.ContentLength != expectedSize {
		err := fmt.Errorf("uploaded object size mismatch")
		tracing.RecordSpanError(span, err)
		return err
	}
	return nil
}

func (s *Service) GetFile(ctx context.Context, userID int64, fileID string) (*FileInfo, error) {
	id, err := uuid.Parse(fileID)
	if err != nil {
		return nil, err
	}
	if s.db == nil {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(ctx, `SELECT f.file_id::text, f.owner_id, f.conversation_id, f.kind, f.original_name, f.mime, f.size_bytes, COALESCE(f.sha256,''), f.status, f.parse_status, o.bucket, o.object_key,
COALESCE(t.object_key,''), COALESCE(r.duration_ms,0), COALESCE(r.width,0), COALESCE(r.height,0), COALESCE(r.metadata,'{}'::jsonb)
FROM attachment_files f
JOIN attachment_objects o ON o.file_id=f.file_id AND o.object_role='original'
LEFT JOIN attachment_objects t ON t.file_id=f.file_id AND t.object_role='thumbnail'
LEFT JOIN attachment_parse_results r ON r.file_id=f.file_id
WHERE f.file_id=$1 AND (f.owner_id=$2 OR f.status='uploaded')`, id, userID)
	var rawMeta []byte
	info := FileInfo{}
	if err := row.Scan(&info.FileID, &info.OwnerID, &info.ConversationID, &info.Kind, &info.OriginalName, &info.Mime, &info.Size, &info.SHA256, &info.Status, &info.ParseStatus, &info.Bucket, &info.ObjectKey, &info.ThumbnailObjectKey, &info.DurationMS, &info.Width, &info.Height, &rawMeta); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(rawMeta, &info.Metadata)
	return &info, nil
}

func (s *Service) Download(ctx context.Context, userID int64, fileID string) (*DownloadResponse, error) {
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if info.Status != "uploaded" {
		return nil, fmt.Errorf("file is not uploaded")
	}
	url, headers, err := s.signer.Presign(http.MethodGet, info.ObjectKey, s.cfg.Seaweed.GetDownloadTTL(), nil)
	if err != nil {
		return nil, err
	}
	return &DownloadResponse{URL: url, Headers: headers, ExpiresAt: time.Now().Add(s.cfg.Seaweed.GetDownloadTTL()).UnixMilli()}, nil
}

func (s *Service) ValidateReference(ctx context.Context, userID, conversationID int64, fileID, kind string) (*FileInfo, error) {
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if info.Status != "uploaded" || info.OwnerID != userID || info.ConversationID != conversationID || info.Kind != kind {
		return nil, fmt.Errorf("attachment reference denied")
	}
	return info, nil
}

func (s *Service) publishUploaded(ctx context.Context, info *FileInfo) error {
	if s.pusher == nil || info == nil {
		return nil
	}
	ctx, span := tracing.StartKafkaProducerSpan(ctx, "attachment.kafka.attachment_uploaded.publish", attribute.String("attachment.file_id", info.FileID))
	defer span.End()

	e := events.AttachmentUploadedEvent{TraceContextFields: tracing.InjectTraceContext(ctx), FileID: info.FileID, OwnerID: info.OwnerID, ConversationID: info.ConversationID, Kind: info.Kind, ObjectKey: info.ObjectKey, Bucket: info.Bucket, OriginalName: info.OriginalName, Mime: info.Mime, Size: info.Size, SHA256: info.SHA256, UploadedAt: time.Now().UnixMilli()}
	b, err := json.Marshal(e)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	err = s.pusher.PushWithKey(ctx, info.FileID, string(b))
	if err != nil {
		tracing.RecordSpanError(span, err)
	}
	return err
}

func objectKey(conversationID int64, fileID, role string) string {
	now := time.Now().UTC()
	return fmt.Sprintf("attachments/%04d/%02d/%d/%s/%s", now.Year(), now.Month(), conversationID, fileID, role)
}

func allowedMime(kind, mime string) bool {
	mime = strings.ToLower(mime)
	switch kind {
	case sharedattachment.KindImage:
		return strings.HasPrefix(mime, "image/")
	case sharedattachment.KindVideo:
		return strings.HasPrefix(mime, "video/")
	case sharedattachment.KindAudio:
		return strings.HasPrefix(mime, "audio/")
	default:
		return false
	}
}

func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func ContentFromFile(info *FileInfo) (string, error) {
	name := info.OriginalName
	if name == "" {
		name = filepath.Base(info.ObjectKey)
	}
	meta, _ := json.Marshal(info.Metadata)
	c := sharedattachment.Content{Schema: sharedattachment.ContentSchemaV1, FileID: info.FileID, Kind: info.Kind, Original: sharedattachment.OriginalObject{Name: name, Mime: info.Mime, Size: info.Size, SHA256: info.SHA256}, ThumbnailFileID: info.ThumbnailObjectKey, ParseStatus: info.ParseStatus, DurationMS: info.DurationMS, Width: info.Width, Height: info.Height, Metadata: bytes.TrimSpace(meta)}
	return c.Marshal()
}
