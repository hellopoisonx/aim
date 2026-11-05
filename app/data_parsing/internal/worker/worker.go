package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/shared/pgtypes"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/config"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/parser"
	"github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/dao"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/orm"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/gormdb"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/hellopoisonx/aim/app/shared/s3signer"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"gorm.io/gorm"
)

const dataParsingTracerName = "github.com/hellopoisonx/aim/app/data_parsing"

// attachmentStore is the subset of dao.Store the worker writes through.
type attachmentStore interface {
	UpsertAttachmentObject(ctx context.Context, object orm.AttachmentObject) error
	UpsertAttachmentParseResult(ctx context.Context, result orm.AttachmentParseResult) error
	SetParseStatus(ctx context.Context, fileID uuid.UUID, status string) (int64, error)
	SetParseResultError(ctx context.Context, fileID uuid.UUID, message string) error
}

// publisher is the subset of *rocketmq.Producer the worker drives.
type publisher interface {
	Start() error
	GracefulStop() error
	Publish(ctx context.Context, m rocketmq.Message) error
	SendTransactional(ctx context.Context, db *gorm.DB, m rocketmq.TransactionalMessage, localTx func(ctx context.Context, tx *gorm.DB) error) error
}

type Worker struct {
	cfg      config.Config
	db       *gorm.DB
	store    attachmentStore
	newStore func(*gorm.DB) attachmentStore
	parser   parser.Parser
	producer publisher
}

func New(ctx context.Context, cfg config.Config) (*Worker, error) {
	db, err := gormdb.OpenOptional(cfg.Postgres.DataSource)
	if err != nil {
		return nil, err
	}
	var store attachmentStore
	newStore := func(db *gorm.DB) attachmentStore { return dao.New(db) }
	if db != nil {
		store = newStore(db)
	}
	// Parsed events are published with the parse-result writes, so the
	// transaction check-back must resolve them from this database.
	producer, err := rocketmq.NewProducer(cfg.ParsedProducer, rocketmq.WithJournalChecker(db))
	if err != nil {
		return nil, err
	}
	var pub publisher
	if producer != nil {
		pub = producer
		// An unreachable broker must not crash-loop the consumer: parsing keeps
		// running and publishes fail loudly instead.
		if err := producer.Start(); err != nil {
			logx.WithContext(ctx).Errorf("failed to start rocketmq parsed producer: %v", err)
		}
	}
	return &Worker{cfg: cfg, db: db, store: store, newStore: newStore, parser: parser.DefaultParser{}, producer: pub}, nil
}

func (w *Worker) Close() {
	if w.producer != nil {
		if err := w.producer.GracefulStop(); err != nil {
			logx.WithContext(context.Background()).Errorf("failed to stop rocketmq parsed producer: %v", err)
		}
	}
	if w.db != nil {
		if sqlDB, err := w.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (w *Worker) Consume(ctx context.Context, key string, value string) (err error) {
	var event events.AttachmentUploadedEvent
	if err := json.Unmarshal([]byte(value), &event); err != nil {
		// Undecodable payloads cannot be repaired by redelivery.
		logx.WithContext(ctx).Errorf(
			"dropping undecodable attachment uploaded event key=%s: %v payload=%q", key, err, value,
		)
		return nil
	}
	ctx = tracing.ExtractTraceContext(ctx, event.TraceContextFields)
	ctx, span := tracing.StartMQConsumerSpan(
		ctx,
		"data_parsing.rocketmq.attachment_uploaded.consume",
		attribute.String("messaging.rocketmq.message_key", key),
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
	// The parser always yields a thumbnail, so upload, DB row and event key agree.
	thumbKey := fmt.Sprintf("attachments/derived/%s/thumbnail.png", event.FileID)
	if err := w.putObject(ctx, thumbKey, res.ThumbMIME, res.Thumbnail); err != nil {
		return w.fail(ctx, event, err)
	}
	meta, _ := json.Marshal(res.Metadata)
	parsed := events.AttachmentParsedEvent{
		FileID:             event.FileID,
		Kind:               event.Kind,
		ParseStatus:        attachment.ParseStatusReady,
		ThumbnailObjectKey: thumbKey,
		DurationMS:         res.DurationMS,
		Width:              res.Width,
		Height:             res.Height,
		Metadata:           &res.Metadata,
		ParsedAt:           time.Now().UnixMilli(),
	}

	if w.db == nil {
		return w.publish(ctx, parsed)
	}

	fileID, err := uuid.Parse(event.FileID)
	if err != nil {
		return w.fail(ctx, event, err)
	}

	if w.producer != nil {
		ctx, span := tracing.StartMQProducerSpan(
			ctx,
			"data_parsing.rocketmq.attachment_parsed.publish",
			attribute.String("attachment.file_id", event.FileID),
		)
		defer span.End()

		parsed.TraceContextFields = tracing.InjectTraceContext(ctx)
		body, err := json.Marshal(parsed)
		if err != nil {
			tracing.RecordSpanError(span, err)
			return err
		}

		err = w.producer.SendTransactional(ctx, w.db, rocketmq.TransactionalMessage{
			Topic:    w.cfg.ParsedProducer.Topic,
			EventUID: "attachment.parsed:" + fileID.String(),
			Key:      fileID.String(),
			Body:     body,
		}, func(ctx context.Context, tx *gorm.DB) error {
			return w.persistParseResult(ctx, w.newStore(tx), fileID, event.Bucket, thumbKey, res, meta)
		})
		if err != nil {
			tracing.RecordSpanError(span, err)
		}

		return err
	}

	if err := w.persistParseResult(ctx, w.store, fileID, event.Bucket, thumbKey, res, meta); err != nil {
		return w.fail(ctx, event, err)
	}

	return w.publish(ctx, parsed)
}

// persistParseResult writes the parse output on store, which is either a
// transaction supplied by SendTransactional or the worker's own handle.
func (w *Worker) persistParseResult(ctx context.Context, store attachmentStore, fileID uuid.UUID, bucket, thumbKey string, res *parser.Result, meta []byte) error {
	size := int64(len(res.Thumbnail))
	thumbWidth := clampInt32(res.ThumbWidth)
	thumbHeight := clampInt32(res.ThumbHeight)
	if err := store.UpsertAttachmentObject(ctx, orm.AttachmentObject{
		FileID:     fileID,
		ObjectRole: "thumbnail",
		Bucket:     bucket,
		ObjectKey:  thumbKey,
		Mime:       res.ThumbMIME,
		SizeBytes:  &size,
		Width:      &thumbWidth,
		Height:     &thumbHeight,
		DurationMs: &res.DurationMS,
	}); err != nil {
		return err
	}

	width := clampInt32(res.Width)
	height := clampInt32(res.Height)
	if err := store.UpsertAttachmentParseResult(ctx, orm.AttachmentParseResult{
		FileID:     fileID,
		DurationMs: &res.DurationMS,
		Width:      &width,
		Height:     &height,
		Metadata:   pgtypes.JSON(meta),
	}); err != nil {
		return err
	}

	_, err := store.SetParseStatus(ctx, fileID, attachment.ParseStatusReady)
	return err
}

// clampInt32 narrows a parser-reported dimension to the int32 column range.
// Media dimensions are far below the limit in practice; clamping keeps a
// corrupt header from wrapping into a negative size.
func clampInt32(v int) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	default:
		return int32(v)
	}
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
	// The failure is already recorded on the attachment and announced downstream;
	// returning nil acknowledges the message instead of replaying the same
	// unparsable object forever.
	return nil
}

func (w *Worker) publish(ctx context.Context, event events.AttachmentParsedEvent) error {
	if w.producer == nil {
		return nil
	}
	ctx, span := tracing.StartMQProducerSpan(ctx, "data_parsing.rocketmq.attachment_parsed.publish", attribute.String("attachment.file_id", event.FileID))
	defer span.End()

	event.TraceContextFields = tracing.InjectTraceContext(ctx)
	b, err := json.Marshal(event)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	err = w.producer.Publish(ctx, rocketmq.Message{
		Topic: w.cfg.ParsedProducer.Topic,
		Key:   event.FileID,
		Body:  b,
	})
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

// seaweedHTTPTimeout bounds one object transfer; http.DefaultClient has none, so
// a stalled endpoint would wedge the consumer goroutine.
const seaweedHTTPTimeout = 30 * time.Second

var seaweedHTTPClient = &http.Client{Timeout: seaweedHTTPTimeout}

// doObject signs, sends and status-checks one object request. On success the
// caller owns the response body and must close it.
func (w *Worker) doObject(ctx context.Context, method, key, mime string, body []byte) (*http.Response, error) {
	url := w.objectURL(key)
	var signedHeaders map[string]string
	if w.cfg.Seaweed.AccessKey != "" && w.cfg.Seaweed.SecretKey != "" {
		var signHeaders map[string]string
		if mime != "" {
			signHeaders = map[string]string{"content-type": mime}
		}
		signer := s3signer.S3Signer{Endpoint: w.cfg.Seaweed.Endpoint, Region: "us-east-1", AccessKey: w.cfg.Seaweed.AccessKey, SecretKey: w.cfg.Seaweed.SecretKey, Bucket: w.cfg.Seaweed.Bucket}
		signed, headers, err := signer.PresignInternal(method, key, time.Minute, signHeaders)
		if err != nil {
			return nil, err
		}
		url = signed
		signedHeaders = headers
	}
	var payload io.Reader
	if body != nil {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return nil, err
	}
	for k, v := range signedHeaders {
		req.Header.Set(k, v)
	}
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := seaweedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s object status %d", strings.ToLower(method), resp.StatusCode)
	}
	return resp, nil
}

func (w *Worker) getObject(ctx context.Context, key string) ([]byte, error) {
	ctx, span := tracing.StartClientSpan(ctx, dataParsingTracerName, "data_parsing.seaweed.get_object", attribute.String("seaweed.object_key", key))
	defer span.End()

	resp, err := w.doObject(ctx, http.MethodGet, key, "", nil)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	return io.ReadAll(resp.Body)
}

func (w *Worker) putObject(ctx context.Context, key, mime string, data []byte) error {
	ctx, span := tracing.StartClientSpan(ctx, dataParsingTracerName, "data_parsing.seaweed.put_object", attribute.String("seaweed.object_key", key))
	defer span.End()

	resp, err := w.doObject(ctx, http.MethodPut, key, mime, data)
	if err != nil {
		tracing.RecordSpanError(span, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	return nil
}
