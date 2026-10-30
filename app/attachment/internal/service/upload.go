package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Upload lifecycle: presigned PUT, verification and the uploaded event.

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
