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

	"github.com/hellopoisonx/aim/app/data_parsing/internal/config"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/parser"
	"github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeromicro/go-queue/kq"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Worker struct {
	cfg    config.Config
	db     *pgxpool.Pool
	parser parser.Parser
	pusher *kq.Pusher
}

func New(ctx context.Context, cfg config.Config) (*Worker, error) {
	var pool *pgxpool.Pool
	var err error
	if cfg.Postgres.DataSource != "" {
		pool, err = pgxpool.New(ctx, cfg.Postgres.DataSource)
		if err != nil {
			return nil, err
		}
	}
	var pusher *kq.Pusher
	if len(cfg.ParsedProducer.Brokers) > 0 && cfg.ParsedProducer.Topic != "" {
		pusher = kq.NewPusher(cfg.ParsedProducer.Brokers, cfg.ParsedProducer.Topic)
	}
	return &Worker{cfg: cfg, db: pool, parser: parser.DefaultParser{}, pusher: pusher}, nil
}

func (w *Worker) Close() {
	if w.db != nil {
		w.db.Close()
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
	if w.db != nil {
		_, err = w.db.Exec(ctx, `INSERT INTO attachment_objects(file_id, object_role, bucket, object_key, mime, size_bytes, width, height, duration_ms)
VALUES ($1,'thumbnail',$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (file_id, object_role) DO UPDATE SET object_key=EXCLUDED.object_key, mime=EXCLUDED.mime, size_bytes=EXCLUDED.size_bytes, width=EXCLUDED.width, height=EXCLUDED.height, duration_ms=EXCLUDED.duration_ms`, event.FileID, event.Bucket, thumbKey, res.ThumbMIME, len(res.Thumbnail), res.ThumbWidth, res.ThumbHeight, res.DurationMS)
		if err != nil {
			return w.fail(ctx, event, err)
		}
		_, err = w.db.Exec(ctx, `INSERT INTO attachment_parse_results(file_id, duration_ms, width, height, metadata)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (file_id) DO UPDATE SET duration_ms=EXCLUDED.duration_ms, width=EXCLUDED.width, height=EXCLUDED.height, metadata=EXCLUDED.metadata, error=NULL, updated_at=NOW()`, event.FileID, res.DurationMS, res.Width, res.Height, meta)
		if err != nil {
			return w.fail(ctx, event, err)
		}
		_, err = w.db.Exec(ctx, `UPDATE attachment_files SET parse_status='ready', updated_at=NOW() WHERE file_id=$1`, event.FileID)
		if err != nil {
			return w.fail(ctx, event, err)
		}
	}

	return w.publish(ctx, events.AttachmentParsedEvent{TraceContextFields: tracing.InjectTraceContext(ctx), FileID: event.FileID, Kind: event.Kind, ParseStatus: attachment.ParseStatusReady, ThumbnailObjectKey: thumbKey, DurationMS: res.DurationMS, Width: res.Width, Height: res.Height, Metadata: res.Metadata, ParsedAt: time.Now().UnixMilli()})
}

func (w *Worker) fail(ctx context.Context, event events.AttachmentUploadedEvent, cause error) error {
	logx.WithContext(ctx).Errorf("parse attachment %s failed: %v", event.FileID, cause)
	if w.db != nil {
		_, _ = w.db.Exec(ctx, `UPDATE attachment_files SET parse_status='failed', updated_at=NOW() WHERE file_id=$1`, event.FileID)
		_, _ = w.db.Exec(ctx, `INSERT INTO attachment_parse_results(file_id, error, metadata) VALUES ($1,$2,'{}') ON CONFLICT (file_id) DO UPDATE SET error=EXCLUDED.error, updated_at=NOW()`, event.FileID, cause.Error())
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
