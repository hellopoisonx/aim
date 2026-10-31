// Package service implements the attachment domain: presigned uploads, object
// verification, download authorization and the transactional
// attachment.uploaded event.
//
// The package is split by concern: service.go holds wiring, upload.go the write
// path, download.go the read path and content.go the message-content helpers.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/hellopoisonx/aim/app/attachment/internal/config"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/zeromicro/go-zero/core/logx"
)

// Domain errors carry their biz code so the RPC server can return them through
// errorx.ToGRPCError without inspecting error strings.
var (
	ErrNotFound        = errorx.NewCodeError(errorx.CodeNotFound, "attachment not found")
	ErrNotUploaded     = errorx.NewCodeError(errorx.CodeBadInput, "file is not uploaded")
	ErrReferenceDenied = errorx.NewCodeError(errorx.CodeForbidden, "attachment reference denied")
)

type Service struct {
	cfg      config.Config
	store    *dao.Store
	signer   s3signer.S3Signer
	producer *rocketmq.Producer
}

// attachmentProducerGroup is the broker-visible producer group of attachment.
const attachmentProducerGroup = "aim-attachment-producer"

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

	// Transactional attachment.uploaded publishing, when configured. A broker
	// that refuses the first connection does not block startup: uploads then
	// fail loudly instead of the whole service crash-looping.
	if store != nil && cfg.UploadedProducer.IsConfigured() {
		producer, err := rocketmq.NewProducer(cfg.UploadedProducer.EndpointConf, attachmentProducerGroup, cfg.UploadedProducer.Topic)
		if err != nil {
			return nil, err
		}
		producer.WithJournalChecker(store.DB())
		if err := producer.Start(); err != nil {
			logx.WithContext(ctx).Errorf("failed to start rocketmq attachment producer: %v", err)
		}
		svc.producer = producer
	}

	return svc, nil
}

func (s *Service) Close() {
	if s.producer != nil {
		if err := s.producer.GracefulStop(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to stop rocketmq attachment producer: %v", err)
		}
	}
	if s.store == nil {
		return
	}
	if db := s.store.DB(); db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func objectKey(conversationID int64, fileID, role string) string {
	now := time.Now().UTC()
	return fmt.Sprintf("attachments/%04d/%02d/%d/%s/%s", now.Year(), now.Month(), conversationID, fileID, role)
}
