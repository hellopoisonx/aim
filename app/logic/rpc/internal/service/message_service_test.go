package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/snowflake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChecker struct {
	decision PermissionDecision
	err      error
	seen     PermissionCheck
}

func (f *fakeChecker) CheckMessagePermission(_ context.Context, check PermissionCheck) (PermissionDecision, error) {
	f.seen = check
	return f.decision, f.err
}

type fakeIdempotency struct {
	ids    map[string]int64
	setErr error
}

func (f *fakeIdempotency) Check(_ context.Context, key string) (bool, int64, error) {
	id, ok := f.ids[key]
	return ok, id, nil
}

func (f *fakeIdempotency) Set(_ context.Context, key string, messageID int64, _ time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.ids == nil {
		f.ids = make(map[string]int64)
	}
	f.ids[key] = messageID
	return nil
}

type fakePublisher struct {
	keys   []string
	bodies [][]byte
	err    error
}

func (f *fakePublisher) Publish(_ context.Context, key string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	f.bodies = append(f.bodies, value)
	return nil
}

func newMessageFixture(t *testing.T) (*IngestionService, *fakeChecker, *fakeIdempotency, *fakePublisher) {
	t.Helper()

	idGen, err := snowflake.NewSnowflake(1)
	require.NoError(t, err)

	checker := &fakeChecker{decision: PermissionDecision{Allowed: true}}
	idempotency := &fakeIdempotency{ids: make(map[string]int64)}
	publisher := &fakePublisher{}

	attachments := NewGRPCAttachmentValidator(nil)

	return NewMessageService(checker, idempotency, publisher, attachments, idGen), checker, idempotency, publisher
}

func sampleInput() IngestMessageInput {
	return IngestMessageInput{
		SenderID:       1,
		DeviceID:       "device-a",
		ConversationID: 42,
		MessageType:    "text",
		Content:        "hello",
		ClientMsgID:    "client-1",
	}
}

func TestIngestMessagePublishesOnce(t *testing.T) {
	svc, checker, idempotency, publisher := newMessageFixture(t)

	in := sampleInput()
	in.Mentions = []string{"2", "3"}

	result, err := svc.IngestMessage(context.Background(), in)
	require.NoError(t, err)
	assert.NotZero(t, result.MessageID)
	assert.Equal(t, "client-1", result.ClientMsgID)
	assert.Equal(t, []int64{2, 3}, checker.seen.Mentions, "checker must receive parsed mention ids")

	require.Len(t, publisher.bodies, 1)
	assert.Equal(t, "42", publisher.keys[0], "the conversation id is the ordering key")
	assert.Equal(t, result.MessageID, idempotency.ids["idempotency:transfer:1:device-a:client-1"])
}

func TestIngestMessageIsIdempotent(t *testing.T) {
	svc, _, _, publisher := newMessageFixture(t)
	ctx := context.Background()

	first, err := svc.IngestMessage(ctx, sampleInput())
	require.NoError(t, err)

	second, err := svc.IngestMessage(ctx, sampleInput())
	require.NoError(t, err)

	assert.Equal(t, first.MessageID, second.MessageID)
	assert.Len(t, publisher.bodies, 1, "a duplicate client_msg_id must not be published again")
}

func TestIngestMessagePropagatesPermissionDenial(t *testing.T) {
	svc, checker, _, publisher := newMessageFixture(t)
	checker.decision = PermissionDecision{Allowed: false, Code: CodePermissionDenied, Reason: "sender is muted"}

	_, err := svc.IngestMessage(context.Background(), sampleInput())
	require.Error(t, err)

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, int(CodePermissionDenied), codeErr.Code)
	assert.Equal(t, "sender is muted", codeErr.Message)
	assert.Empty(t, publisher.bodies)
}

func TestIngestMessageWithoutCheckerIsInternal(t *testing.T) {
	idGen, err := snowflake.NewSnowflake(1)
	require.NoError(t, err)

	svc := NewMessageService(nil, &fakeIdempotency{ids: map[string]int64{}}, &fakePublisher{}, nil, idGen)

	_, err = svc.IngestMessage(context.Background(), sampleInput())
	require.Error(t, err)

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeInternal, codeErr.Code)
}

func TestIngestMessageFiltersMentions(t *testing.T) {
	svc, checker, _, publisher := newMessageFixture(t)
	checker.decision = PermissionDecision{Allowed: true, FilteredMentions: []int64{2}}

	in := sampleInput()
	in.Mentions = []string{"2", "3"}

	_, err := svc.IngestMessage(context.Background(), in)
	require.NoError(t, err)

	require.Len(t, publisher.bodies, 1)
	assert.Contains(t, string(publisher.bodies[0]), `"mentions":["2"]`)
	assert.NotContains(t, string(publisher.bodies[0]), `"3"`)
}

func TestIngestMessagePublishFailureIsInternal(t *testing.T) {
	svc, _, idempotency, publisher := newMessageFixture(t)
	publisher.err = errors.New("broker down")

	_, err := svc.IngestMessage(context.Background(), sampleInput())
	require.Error(t, err)

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeInternal, codeErr.Code)
	assert.Empty(t, idempotency.ids, "a failed publish must not leave an idempotency key behind")
}

func TestIngestMessageIdempotencyWriteFailureStillAccepts(t *testing.T) {
	svc, _, idempotency, publisher := newMessageFixture(t)
	idempotency.setErr = errors.New("redis down")

	result, err := svc.IngestMessage(context.Background(), sampleInput())
	require.NoError(t, err)
	assert.NotZero(t, result.MessageID)
	assert.Len(t, publisher.bodies, 1)
}

func TestIngestMessageAttachmentWithoutRPCIsInternal(t *testing.T) {
	svc, _, _, _ := newMessageFixture(t)

	in := sampleInput()
	in.MessageType = attachment.KindImage
	in.Content = `{"schema":"aim.attachment.v1","file_id":"file-1","kind":"image",` +
		`"original":{"name":"a.png","mime":"image/png","size":10},"parse_status":"pending"}`

	_, err := svc.IngestMessage(context.Background(), in)
	require.Error(t, err)

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeInternal, codeErr.Code)
}

func TestIngestMessageRejectsInvalidInput(t *testing.T) {
	svc, _, _, _ := newMessageFixture(t)

	tests := map[string]func(*IngestMessageInput){
		"missing sender":         func(in *IngestMessageInput) { in.SenderID = 0 },
		"missing conversation":   func(in *IngestMessageInput) { in.ConversationID = 0 },
		"missing message type":   func(in *IngestMessageInput) { in.MessageType = "" },
		"message type too long":  func(in *IngestMessageInput) { in.MessageType = "0123456789012345678901234567890123" },
		"missing client msg id":  func(in *IngestMessageInput) { in.ClientMsgID = "" },
		"invalid mention id":     func(in *IngestMessageInput) { in.Mentions = []string{"abc"} },
		"non-positive mention":   func(in *IngestMessageInput) { in.Mentions = []string{"0"} },
		"too many mentions":      func(in *IngestMessageInput) { in.Mentions = make([]string, maxMentions+1) },
		"attachment without rpc": func(in *IngestMessageInput) { in.MessageType = "image" },
	}

	for name, mutate := range tests {
		in := sampleInput()
		mutate(&in)
		_, err := svc.IngestMessage(context.Background(), in)
		require.Error(t, err, name)
	}
}
