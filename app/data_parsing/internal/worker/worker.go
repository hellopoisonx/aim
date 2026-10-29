package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/attachment/model/dao"
	"github.com/hellopoisonx/aim/app/attachment/model/orm"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/config"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/parser"
	"github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/datatypes"
)

type Worker struct {
	cfg    config.Config
	store  *dao.Store
	parser parser.Parser
	pusher *kq.Pusher
}

func New(ctx context.Context, cfg config.Config) (*Worker, error) {
	var store *dao.Store
	if cfg.Postgres.DataSource != "" {
		db, err := gormdb.Open(cfg.Postgres.DataSource)
		if err != nil {
			return nil, err
		}
		store = dao.New(db)
	}
	var pusher *kq.Pusher
	if len(cfg.ParsedProducer.Brokers) > 0 && cfg.ParsedProducer.Topic != "" {
		pusher = kq.NewPusher(cfg.ParsedProducer.Brokers, cfg.ParsedProducer.Topic)
	}
	return &Worker{cfg: cfg, store: store, parser: parser.DefaultParser{}, pusher: pusher}, nil
}

func (w *Worker) Close() {
	if w.store == nil {
		return
	}
	if db := w.store.DB(); db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (w *Worker) Consume(ctx context.Context, key string, value string) (err error) {
	var event events.AttachmentUploadedEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		return err
	}
	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)
	ctx, span := tracing.StartKafkaConsumerSpan(
		ctx,
		"data_parsing.kafka.attachment_uploaded.consume",
		attribute.String("messaging.kafka.message_key", key),
		attribute.String("attachment.file_id", event.FileID),
	)
	defer func() {
		if err != nil {
			tracing.RecordSpanError(span, err)
		}
		span.End()
	}()

	if !attachment.RequiresDataParsing(event.Kind) {
		if w.store != nil {
			fileID, parseErr := uuid.Parse(event.FileID)
			if parseErr != nil {
				return parseErr
			}
			_, err = w.store.SetParseStatus(ctx, fileID, attachment.ParseStatusReady)
		}
		return err
	}

	data, err := w.getObject(ctx, event.ObjectKey)
	if err != nil {
		return w.fail(ctx, event, err)
	}
	res, err := w.parser.Parse(ctx, event.Kind, event.Mime, data)
	if err != nil {
		return w.fail(ctx, event, err)
	}
	thumbKey := fmt.Sprintf("attachments/derived/%s/thumbnail.png", event.FileID)
	if len(res.Thumbnail) > 0 {
		if err := w.putObject(ctx, thumbKey, res.ThumbMIME, res.Thumbnail); err != nil {
			return w.fail(ctx, event, err)
		}
	}
	meta, _ := json.Marshal(res.Metadata)
	if w.store != nil {
		fileID, parseErr := uuid.Parse(event.FileID)
		if parseErr != nil {
			return w.fail(ctx, event, parseErr)
		}
		size := int64(len(res.Thumbnail))
		thumbWidth := int32(res.ThumbWidth)
		thumbHeight := int32(res.ThumbHeight)
		width := int32(res.Width)
		height := int32(res.Height)
		if err = w.store.UpsertAttachmentObject(ctx, orm.AttachmentObject{
			FileID:     fileID,
			ObjectRole: "thumbnail",
			Bucket:     event.Bucket,
			ObjectKey:  thumbKey,
			Mime:       res.ThumbMIME,
			SizeBytes:  &size,
			Width:      &thumbWidth,
			Height:     &thumbHeight,
			DurationMs: &res.DurationMS,
		}); err != nil {
			return w.fail(ctx, event, err)
		}
		if err = w.store.UpsertAttachmentParseResult(ctx, orm.AttachmentParseResult{
			FileID:     fileID,
			DurationMs: &res.DurationMS,
			Width:      &width,
			Height:     &height,
			Metadata:   datatypes.JSON(meta),
		}); err != nil {
			return w.fail(ctx, event, err)
		}
		if _, err = w.store.SetParseStatus(ctx, fileID, attachment.ParseStatusReady); err != nil {
			return w.fail(ctx, event, err)
		}
	}

	return w.publish(ctx, events.AttachmentParsedEvent{TraceContextFields: tracing.InjectTraceContext(ctx), FileID: event.FileID, Kind: event.Kind, ParseStatus: attachment.ParseStatusReady, ThumbnailObjectKey: thumbKey, DurationMS: res.DurationMS, Width: res.Width, Height: res.Height, Metadata: res.Metadata, ParsedAt: time.Now().UnixMilli()})
}

func (w *Worker) fail(ctx context.Context, event events.AttachmentUploadedEvent, cause error) error {
	logx.WithContext(ctx).Errorf("parse attachment %s failed: %v", event.FileID, cause)
	if w.store != nil {
		if fileID, parseErr := uuid.Parse(event.FileID); parseErr == nil {
			_, _ = w.store.SetParseStatus(ctx, fileID, attachment.ParseStatusFailed)
			_ = w.store.SetParseResultError(ctx, fileID, cause.Error())
		}
	}
	_ = w.publish(ctx, events.AttachmentParsedEvent{TraceContextFields: tracing.InjectTraceContext(ctx), FileID: event.FileID, Kind: event.Kind, ParseStatus: attachment.ParseStatusFailed, Error: cause.Error(), ParsedAt: time.Now().UnixMilli()})
	return cause
}

func (w *Worker) publish(ctx context.Context, event events.AttachmentParsedEvent) error {
	if w.pusher == nil {
		return nil
	}
	ctx, span := tracing.StartKafkaProducerSpan(ctx, "data_parsing.kafka.attachment_parsed.publish", attribute.String("attachment.file_id", event.FileID))
	defer span.End()

	event.TraceContextFields = tracing.InjectTraceContext(ctx)
	b, err := json.Marshal(event)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	err = w.pusher.PushWithKey(ctx, event.FileID, string(b))
	if err != nil {
		tracing.RecordSpanError(span, err)
	}
	return err
}

func (w *Worker) objectURL(key string) string {
	endpoint := strings.TrimRight(w.cfg.Seaweed.Endpoint, "/")
	bucket := w.cfg.Seaweed.Bucket
	return endpoint + "/" + bucket + "/" + strings.TrimLeft(key, "/")
}

func (w *Worker) getObject(ctx context.Context, key string) ([]byte, error) {
	ctx, span := otel.Tracer("github.com/hellopoisonx/aim/app/data_parsing").Start(
		ctx,
		"data_parsing.seaweed.get_object",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("seaweed.object_key", key)),
	)
	defer span.End()

	url := w.objectURL(key)
	if w.cfg.Seaweed.AccessKey != "" && w.cfg.Seaweed.SecretKey != "" {
		signer := s3signer.S3Signer{Endpoint: w.cfg.Seaweed.Endpoint, Region: "us-east-1", AccessKey: w.cfg.Seaweed.AccessKey, SecretKey: w.cfg.Seaweed.SecretKey, Bucket: w.cfg.Seaweed.Bucket}
		signed, _, err := signer.PresignInternal(http.MethodGet, key, time.Minute, nil)
		if err != nil {
			tracing.RecordSpanError(span, err)
			return nil, err
		}
		url = signed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return nil, err
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("get object status %d", resp.StatusCode)
		tracing.RecordSpanError(span, err)
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

func (w *Worker) putObject(ctx context.Context, key, mime string, data []byte) error {
	ctx, span := otel.Tracer("github.com/hellopoisonx/aim/app/data_parsing").Start(
		ctx,
		"data_parsing.seaweed.put_object",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("seaweed.object_key", key)),
	)
	defer span.End()

	url := w.objectURL(key)
	var headers map[string]string
	if w.cfg.Seaweed.AccessKey != "" && w.cfg.Seaweed.SecretKey != "" {
		signer := s3signer.S3Signer{Endpoint: w.cfg.Seaweed.Endpoint, Region: "us-east-1", AccessKey: w.cfg.Seaweed.AccessKey, SecretKey: w.cfg.Seaweed.SecretKey, Bucket: w.cfg.Seaweed.Bucket}
		signed, signedHeaders, err := signer.PresignInternal(http.MethodPut, key, time.Minute, map[string]string{"content-type": mime})
		if err != nil {
			tracing.RecordSpanError(span, err)
			return err
		}
		url = signed
		headers = signedHeaders
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", mime)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("put object status %d", resp.StatusCode)
		tracing.RecordSpanError(span, err)
		return err
	}
	return nil
}
