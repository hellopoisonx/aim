package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/data_parsing/internal/config"
	parserpkg "github.com/hellopoisonx/aim/app/data_parsing/internal/parser"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/attachmentstore/orm"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testBucket = "aim-test"

var errStage = errors.New("stage failed")

// fakeParser stands in for the media parser and records whether it ran.
type fakeParser struct {
	called bool
	res    *parserpkg.Result
	err    error
}

func (p *fakeParser) Parse(_ context.Context, _ string, _ string, _ []byte) (*parserpkg.Result, error) {
	p.called = true
	if p.err != nil {
		return nil, p.err
	}

	return p.res, nil
}

// fakeObjects serves the SeaweedFS S3 side of the pipeline over HTTP.
type fakeObjects struct {
	server  *httptest.Server
	data    map[string][]byte
	puts    map[string][]byte
	putMime map[string]string
	getCode int
	putCode int
}

func newFakeObjects(t *testing.T) *fakeObjects {
	t.Helper()

	f := &fakeObjects{data: map[string][]byte{}, puts: map[string][]byte{}, putMime: map[string]string{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/"+testBucket+"/")
		switch r.Method {
		case http.MethodGet:
			if f.getCode != 0 {
				w.WriteHeader(f.getCode)
				return
			}
			body, ok := f.data[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
		case http.MethodPut:
			if f.putCode != 0 {
				w.WriteHeader(f.putCode)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read PUT body: %v", err)
			}
			f.puts[key] = body
			f.putMime[key] = r.Header.Get("Content-Type")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.server.Close)

	return f
}

// fakeStore records the writes the worker makes, with an optional error per method.
type fakeStore struct {
	objects      []orm.AttachmentObject
	results      []orm.AttachmentParseResult
	statuses     []string
	resultErrors []string

	objectErr error
	resultErr error
	statusErr error
	errorErr  error
}

func (s *fakeStore) UpsertAttachmentObject(_ context.Context, object orm.AttachmentObject) error {
	if s.objectErr != nil {
		return s.objectErr
	}
	s.objects = append(s.objects, object)

	return nil
}

func (s *fakeStore) UpsertAttachmentParseResult(_ context.Context, result orm.AttachmentParseResult) error {
	if s.resultErr != nil {
		return s.resultErr
	}
	s.results = append(s.results, result)

	return nil
}

func (s *fakeStore) SetParseStatus(_ context.Context, _ uuid.UUID, status string) (int64, error) {
	if s.statusErr != nil {
		return 0, s.statusErr
	}
	s.statuses = append(s.statuses, status)

	return 1, nil
}

func (s *fakeStore) SetParseResultError(_ context.Context, _ uuid.UUID, message string) error {
	if s.errorErr != nil {
		return s.errorErr
	}
	s.resultErrors = append(s.resultErrors, message)

	return nil
}

// fakeProducer records what the worker publishes and can fail either path.
type fakeProducer struct {
	published  []rocketmq.Message
	committed  []rocketmq.TransactionalMessage
	publishErr error
	sendErr    error
}

func (p *fakeProducer) Start() error        { return nil }
func (p *fakeProducer) GracefulStop() error { return nil }

func (p *fakeProducer) Publish(_ context.Context, m rocketmq.Message) error {
	if p.publishErr != nil {
		return p.publishErr
	}
	p.published = append(p.published, m)

	return nil
}

func (p *fakeProducer) SendTransactional(ctx context.Context, _ *gorm.DB, m rocketmq.TransactionalMessage, localTx func(ctx context.Context, tx *gorm.DB) error) error {
	if p.sendErr != nil {
		return p.sendErr
	}
	// The journal transaction itself is rocketmq's concern; the worker only
	// needs its callback invoked once.
	if err := localTx(ctx, nil); err != nil {
		return err
	}
	p.committed = append(p.committed, m)

	return nil
}

var _ attachmentStore = (*fakeStore)(nil)

var _ publisher = (*fakeProducer)(nil)

func testWorker(objects *fakeObjects, p parserpkg.Parser) *Worker {
	return &Worker{
		cfg:    config.Config{Seaweed: config.SeaweedConf{Endpoint: objects.server.URL, Bucket: testBucket}, ParsedProducer: rocketmq.ProducerConf{Topic: "aim-attachment-parsed"}},
		parser: p,
	}
}

func uploadEvent(t *testing.T, fileID string) string {
	t.Helper()

	payload, err := json.Marshal(events.AttachmentUploadedEvent{
		FileID:    fileID,
		Kind:      sharedattachment.KindImage,
		Mime:      "image/png",
		ObjectKey: "uploads/pic.png",
		Bucket:    testBucket,
	})
	require.NoError(t, err)

	return string(payload)
}

func parseResult() *parserpkg.Result {
	return &parserpkg.Result{
		Thumbnail:   []byte("thumbnail-png"),
		ThumbMIME:   "image/png",
		ThumbWidth:  512,
		ThumbHeight: 256,
		Width:       1024,
		Height:      768,
		DurationMS:  1500,
		Metadata:    sharedattachment.MediaMetadata{Mime: "image/png", Format: "png"},
	}
}

func TestConsumeSkipsGenericFileWithoutParsing(t *testing.T) {
	p := &fakeParser{}
	w := &Worker{parser: p}
	event := events.AttachmentUploadedEvent{FileID: "file-id", Kind: sharedattachment.KindFile, Mime: "application/pdf"}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	err = w.Consume(context.Background(), event.FileID, string(payload))
	require.NoError(t, err)
	require.False(t, p.called)
}

func TestConsumeDropsUndecodablePayload(t *testing.T) {
	p := &fakeParser{}
	w := &Worker{parser: p}

	require.NoError(t, w.Consume(context.Background(), "key", "{not json"))
	require.False(t, p.called)
}

func TestConsumeStoresAndPublishesParseResult(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	store := &fakeStore{}
	producer := &fakeProducer{}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.store = store
	w.newStore = func(*gorm.DB) attachmentStore { return store }
	w.db = &gorm.DB{}
	w.producer = producer

	require.NoError(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)))

	thumbKey := "attachments/derived/" + fileID + "/thumbnail.png"
	require.Equal(t, []byte("thumbnail-png"), objects.puts[thumbKey])
	require.Equal(t, "image/png", objects.putMime[thumbKey])

	require.Len(t, store.objects, 1)
	thumb := store.objects[0]
	require.Equal(t, "thumbnail", thumb.ObjectRole)
	require.Equal(t, thumbKey, thumb.ObjectKey)
	require.Equal(t, testBucket, thumb.Bucket)
	require.Equal(t, "image/png", thumb.Mime)
	require.EqualValues(t, len("thumbnail-png"), *thumb.SizeBytes)
	require.EqualValues(t, 512, *thumb.Width)
	require.EqualValues(t, 256, *thumb.Height)
	require.EqualValues(t, 1500, *thumb.DurationMs)

	require.Len(t, store.results, 1)
	require.EqualValues(t, 1024, *store.results[0].Width)
	require.EqualValues(t, 768, *store.results[0].Height)
	require.JSONEq(t, `{"mime":"image/png","format":"png"}`, string(store.results[0].Metadata))
	require.Equal(t, []string{sharedattachment.ParseStatusReady}, store.statuses)

	require.Len(t, producer.committed, 1)
	message := producer.committed[0]
	require.Equal(t, "aim-attachment-parsed", message.Topic)
	require.Equal(t, "attachment.parsed:"+fileID, message.EventUID)
	require.Equal(t, fileID, message.Key)

	var published events.AttachmentParsedEvent
	require.NoError(t, json.Unmarshal(message.Body, &published))
	require.Equal(t, fileID, published.FileID)
	require.Equal(t, sharedattachment.KindImage, published.Kind)
	require.Equal(t, sharedattachment.ParseStatusReady, published.ParseStatus)
	require.Equal(t, thumbKey, published.ThumbnailObjectKey)
	require.EqualValues(t, 1024, published.Width)
	require.EqualValues(t, 768, published.Height)
	require.EqualValues(t, 1500, published.DurationMS)
	require.Empty(t, published.Error)
	require.NotNil(t, published.Metadata)
	require.Equal(t, "png", published.Metadata.Format)
}

// failureCases asserts the shared contract of every stage that gives up: the
// attachment is marked failed, the reason reaches the published event, and the
// message is acknowledged so it is not replayed forever.
func assertFailurePublished(t *testing.T, store *fakeStore, producer *fakeProducer, wantReason string) {
	t.Helper()

	require.Equal(t, []string{sharedattachment.ParseStatusFailed}, store.statuses)
	require.Len(t, store.resultErrors, 1)
	require.Contains(t, store.resultErrors[0], wantReason)

	require.Len(t, producer.published, 1)
	var published events.AttachmentParsedEvent
	require.NoError(t, json.Unmarshal(producer.published[0].Body, &published))
	require.Equal(t, sharedattachment.ParseStatusFailed, published.ParseStatus)
	require.Contains(t, published.Error, wantReason)
	require.Empty(t, producer.committed)
}

func TestConsumeDownloadFailurePublishesFailure(t *testing.T) {
	objects := newFakeObjects(t)
	objects.getCode = http.StatusInternalServerError
	store := &fakeStore{}
	producer := &fakeProducer{}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.store = store
	w.producer = producer

	require.NoError(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)))
	assertFailurePublished(t, store, producer, "get object status 500")
	require.Empty(t, store.objects, "a failed download stores nothing")
}

func TestConsumeParseFailurePublishesFailure(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	store := &fakeStore{}
	producer := &fakeProducer{}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{err: errors.New("decode image: corrupt")})
	w.store = store
	w.producer = producer

	require.NoError(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)))
	assertFailurePublished(t, store, producer, "decode image: corrupt")
}

func TestConsumeThumbnailUploadFailurePublishesFailure(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	objects.putCode = http.StatusInternalServerError
	store := &fakeStore{}
	producer := &fakeProducer{}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.store = store
	w.producer = producer

	require.NoError(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)))
	assertFailurePublished(t, store, producer, "put object status 500")
	require.Empty(t, store.objects, "a failed thumbnail upload must not be recorded")
}

func TestConsumeStoreFailureFailsTheTransaction(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	store := &fakeStore{objectErr: errStage}
	producer := &fakeProducer{}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.store = store
	w.newStore = func(*gorm.DB) attachmentStore { return store }
	w.db = &gorm.DB{}
	w.producer = producer

	require.ErrorIs(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)), errStage)
	require.Empty(t, producer.committed, "a failed write must not commit the parsed event")
}

func TestConsumeStoreFailureWithoutProducerMarksFailed(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	store := &fakeStore{resultErr: errStage}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.store = store
	w.db = &gorm.DB{}

	require.NoError(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)))
	require.Equal(t, []string{sharedattachment.ParseStatusFailed}, store.statuses)
}

func TestConsumePublishFailureIsReturned(t *testing.T) {
	objects := newFakeObjects(t)
	objects.data["uploads/pic.png"] = []byte("original-bytes")
	producer := &fakeProducer{publishErr: errStage}
	fileID := uuid.NewString()

	w := testWorker(objects, &fakeParser{res: parseResult()})
	w.producer = producer

	require.ErrorIs(t, w.Consume(context.Background(), fileID, uploadEvent(t, fileID)), errStage)
}

func TestClampInt32(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int32
	}{
		{name: "negative", in: -1, want: -1},
		{name: "zero", in: 0, want: 0},
		{name: "typical dimension", in: 1920, want: 1920},
		{name: "above int32 max", in: math.MaxInt32 + 1, want: math.MaxInt32},
		{name: "below int32 min", in: math.MinInt32 - 1, want: math.MinInt32},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, clampInt32(tc.in))
		})
	}
}
