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

	sharedkafka "github.com/hellopoisonx/aim/app/shared/kafka"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	"github.com/hellopoisonx/aim/app/attachment/model/dao"
	"github.com/hellopoisonx/aim/app/attachment/model/orm"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/outbox"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Domain errors carry their biz code so the RPC server can return them through
// errorx.ToGRPCError without inspecting error strings.
var (
	ErrNotFound        = errorx.NewCodeError(errorx.CodeNotFound, "attachment not found")
	ErrNotUploaded     = errorx.NewCodeError(errorx.CodeBadInput, "file is not uploaded")
	ErrReferenceDenied = errorx.NewCodeError(errorx.CodeForbidden, "attachment reference denied")
)

type Service struct {
	cfg          config.Config
	store        *dao.Store
	signer       s3signer.S3Signer
	outboxPoller *outbox.Poller
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

// CompleteUploadRequest carries the client-reported checksum for the uploaded
// object; the caller identity is passed separately as an int64 user id.
type CompleteUploadRequest struct {
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
	var store *dao.Store
	if cfg.Postgres.DataSource != "" {
		db, err := gormdb.Open(cfg.Postgres.DataSource)
		if err != nil {
			return nil, err
		}
		store = dao.New(db)
	}

	svc := &Service{
		cfg:   cfg,
		store: store,
		signer: s3signer.S3Signer{
			Endpoint:       cfg.Seaweed.Endpoint,
			PublicEndpoint: cfg.Seaweed.PublicEndpoint,
			Region:         cfg.Seaweed.Region,
			AccessKey:      cfg.Seaweed.AccessKey,
			SecretKey:      cfg.Seaweed.SecretKey,
			Bucket:         cfg.Seaweed.Bucket,
		},
	}

	// Initialize outbox Poller if Kafka is configured
	if store != nil && len(cfg.Kafka.Brokers) > 0 && cfg.Kafka.Topic != "" {
		outboxStore := outbox.NewGORMStore(store.DB())
		publisher := newOutboxPublisher(cfg.Kafka.Brokers, cfg.Kafka.Topic)
		svc.outboxPoller = outbox.NewPoller(outboxStore, publisher, outbox.Config{})
	}

	return svc, nil
}

func (s *Service) OutboxPoller() *outbox.Poller {
	return s.outboxPoller
}

// newOutboxPublisher creates an outbox.PublisherFunc using kq.Pusher.
func newOutboxPublisher(brokers []string, topic string) outbox.PublisherFunc {
	pusher := sharedkafka.NewPusher(brokers, topic)
	return func(ctx context.Context, _, key string, payload []byte) error {
		return pusher.PushWithKey(ctx, key, string(payload))
	}
}

func (s *Service) Close() {
	if s.store == nil {
		return
	}
	if db := s.store.DB(); db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (s *Service) InitUpload(ctx context.Context, req InitUploadRequest) (*InitUploadResponse, error) {
	if req.OwnerID <= 0 || req.ConversationID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "owner_id and conversation_id are required")
	}
	if !sharedattachment.ValidKind(req.Kind) {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "unsupported kind")
	}
	if req.Size <= 0 || req.Size > s.cfg.Seaweed.MaxSize() {
		return nil, errorx.NewCodeErrorf(errorx.CodeBadInput, "size must be between 1 and %d bytes", s.cfg.Seaweed.MaxSize())
	}
	if !allowedMime(req.Kind, req.Mime) {
		return nil, errorx.NewCodeErrorf(errorx.CodeBadInput, "mime %q is not allowed for %s", req.Mime, req.Kind)
	}
	fileID := uuid.New()
	key := objectKey(req.ConversationID, fileID.String(), "original")
	url, headers, err := s.signer.Presign(http.MethodPut, key, s.cfg.Seaweed.UploadTTL(), map[string]string{"content-type": req.Mime})
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		var sha256 *string
		if req.SHA256 != "" {
			sha256 = &req.SHA256
		}
		file := orm.AttachmentFile{
			FileID:         fileID,
			OwnerID:        req.OwnerID,
			ConversationID: req.ConversationID,
			Kind:           req.Kind,
			OriginalName:   req.OriginalName,
			Mime:           req.Mime,
			SizeBytes:      req.Size,
			Sha256:         sha256,
			Status:         sharedattachment.UploadStatusPending,
			ParseStatus:    sharedattachment.ParseStatusPending,
		}
		if err := s.store.CreateAttachmentFile(ctx, file); err != nil {
			return nil, err
		}
		object := orm.AttachmentObject{
			FileID:     fileID,
			ObjectRole: "original",
			Bucket:     s.cfg.Seaweed.Bucket,
			ObjectKey:  key,
			Mime:       req.Mime,
			SizeBytes:  &req.Size,
			Sha256:     sha256,
		}
		if err := s.store.CreateAttachmentObject(ctx, object); err != nil {
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
	if s.store != nil {
		pending, err := s.getPendingOriginal(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if err := s.verifyObject(ctx, pending.ObjectKey, pending.Size); err != nil {
			return nil, err
		}

		// Use transaction: update status + insert outbox record atomically
		err = s.store.WithTx(ctx, func(tx *dao.Store) error {
			rows, err := tx.MarkUploaded(ctx, id, userID, req.SHA256, sharedattachment.KindFile)
			if err != nil {
				return err
			}
			if rows == 0 {
				return ErrNotFound
			}

			// Insert outbox event atomically if enabled
			if s.outboxPoller != nil {
				return s.insertOutboxEvent(ctx, tx, pending)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Fast path: wake the poller immediately
		if s.outboxPoller != nil {
			s.outboxPoller.Wake()
		}
	}
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// insertOutboxEvent inserts an AttachmentUploadedEvent into the outbox table
// using the given transaction.
func (s *Service) insertOutboxEvent(ctx context.Context, tx *dao.Store, info *FileInfo) error {
	e := events.AttachmentUploadedEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		FileID:             info.FileID,
		OwnerID:            info.OwnerID,
		ConversationID:     info.ConversationID,
		Kind:               info.Kind,
		ObjectKey:          info.ObjectKey,
		Bucket:             info.Bucket,
		OriginalName:       info.OriginalName,
		Mime:               info.Mime,
		Size:               info.Size,
		SHA256:             info.SHA256,
		UploadedAt:         time.Now().UnixMilli(),
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}

	_, err = tx.InsertOutboxRecord(ctx, s.cfg.Kafka.Topic, info.FileID, payload)
	return err
}

func (s *Service) getPendingOriginal(ctx context.Context, userID int64, fileID uuid.UUID) (*FileInfo, error) {
	row, err := s.store.GetPendingOriginal(ctx, fileID, userID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &FileInfo{
		FileID:         row.FileID.String(),
		OwnerID:        row.OwnerID,
		ConversationID: row.ConversationID,
		Kind:           row.Kind,
		OriginalName:   row.OriginalName,
		Mime:           row.Mime,
		Size:           row.SizeBytes,
		SHA256:         row.Sha256,
		Status:         row.Status,
		ParseStatus:    row.ParseStatus,
		Bucket:         row.Bucket,
		ObjectKey:      row.ObjectKey,
	}, nil
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
		err := errorx.NewCodeErrorf(errorx.CodeBadInput, "uploaded object not found: status %d", resp.StatusCode)
		tracing.RecordSpanError(span, err)
		return err
	}
	if resp.ContentLength > 0 && expectedSize > 0 && resp.ContentLength != expectedSize {
		err := errorx.NewCodeError(errorx.CodeBadInput, "uploaded object size mismatch")
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
	if s.store == nil {
		return nil, ErrNotFound
	}
	row, err := s.store.GetFile(ctx, id, userID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	info := &FileInfo{
		FileID:             row.FileID.String(),
		OwnerID:            row.OwnerID,
		ConversationID:     row.ConversationID,
		Kind:               row.Kind,
		OriginalName:       row.OriginalName,
		Mime:               row.Mime,
		Size:               row.SizeBytes,
		SHA256:             row.Sha256,
		Status:             row.Status,
		ParseStatus:        row.ParseStatus,
		Bucket:             row.Bucket,
		ObjectKey:          row.ObjectKey,
		ThumbnailObjectKey: row.ThumbnailObjectKey,
		DurationMS:         row.DurationMS,
		Width:              int(row.Width),
		Height:             int(row.Height),
	}
	_ = json.Unmarshal(row.Metadata, &info.Metadata)
	return info, nil
}

func (s *Service) Download(ctx context.Context, userID int64, fileID string) (*DownloadResponse, error) {
	info, err := s.GetFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if info.Status != sharedattachment.UploadStatusUploaded {
		return nil, ErrNotUploaded
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
	if info.Status != sharedattachment.UploadStatusUploaded || info.OwnerID != userID || info.ConversationID != conversationID || info.Kind != kind {
		return nil, ErrReferenceDenied
	}
	return info, nil
}

func objectKey(conversationID int64, fileID, role string) string {
	now := time.Now().UTC()
	return fmt.Sprintf("attachments/%04d/%02d/%d/%s/%s", now.Year(), now.Month(), conversationID, fileID, role)
}

func allowedMime(kind, mime string) bool {
	mime = strings.TrimSpace(strings.ToLower(mime))
	if mime == "" || !strings.Contains(mime, "/") {
		return false
	}
	switch kind {
	case sharedattachment.KindImage:
		return strings.HasPrefix(mime, "image/")
	case sharedattachment.KindVideo:
		return strings.HasPrefix(mime, "video/")
	case sharedattachment.KindAudio:
		return strings.HasPrefix(mime, "audio/")
	case sharedattachment.KindFile:
		return !strings.HasPrefix(mime, "image/") && !strings.HasPrefix(mime, "video/") && !strings.HasPrefix(mime, "audio/")
	default:
		return false
	}
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
