package rocketmq

import (
	"context"
	"errors"
	"testing"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeTransaction struct {
	commits     int
	rollbacks   int
	commitErr   error
	rollbackErr error
}

func (t *fakeTransaction) Commit() error {
	t.commits++
	return t.commitErr
}

func (t *fakeTransaction) RollBack() error {
	t.rollbacks++
	return t.rollbackErr
}

type fakeSender struct {
	started   int
	stopped   int
	sent      []*rmq.Message
	sendErr   error
	sentTx    []*rmq.Message
	sendTxErr error
	tx        *fakeTransaction
}

func (f *fakeSender) Start() error { f.started++; return nil }

func (f *fakeSender) GracefulStop() error { f.stopped++; return nil }

func (f *fakeSender) Send(_ context.Context, msg *rmq.Message) ([]*rmq.SendReceipt, error) {
	f.sent = append(f.sent, msg)
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return []*rmq.SendReceipt{{MessageID: "m"}}, nil
}

func (f *fakeSender) SendWithTransaction(_ context.Context, msg *rmq.Message, _ rmq.Transaction) ([]*rmq.SendReceipt, error) {
	f.sentTx = append(f.sentTx, msg)
	if f.sendTxErr != nil {
		return nil, f.sendTxErr
	}
	return []*rmq.SendReceipt{{MessageID: "m"}}, nil
}

func (f *fakeSender) BeginTransaction() rmq.Transaction {
	f.tx = &fakeTransaction{}
	return f.tx
}

func newTestProducer(sender *fakeSender) *Producer {
	return &Producer{group: "test-group", producer: sender, journalTx: gormJournalTx}
}

// journalStub replaces the database transaction so tests can drive the
// commit/rollback decision without Postgres.
func journalStub(calls *[]EventJournal, run func(localTx func(context.Context, *gorm.DB) error) error) journalTx {
	return func(ctx context.Context, _ *gorm.DB, journal EventJournal, localTx func(context.Context, *gorm.DB) error) error {
		*calls = append(*calls, journal)
		if run != nil {
			return run(localTx)
		}
		return localTx(ctx, nil)
	}
}

func TestPublishMapsMessageFields(t *testing.T) {
	sender := &fakeSender{}
	producer := newTestProducer(sender)

	err := producer.Publish(context.Background(), Message{
		Topic:       "aim-read-receipt-events",
		Key:         "42",
		ShardingKey: "42",
		Body:        []byte(`{"k":"v"}`),
	})
	require.NoError(t, err)
	require.Len(t, sender.sent, 1)

	sent := sender.sent[0]
	assert.Equal(t, "aim-read-receipt-events", sent.Topic)
	assert.JSONEq(t, `{"k":"v"}`, string(sent.Body))
	assert.Equal(t, []string{"42"}, sent.GetKeys())
	require.NotNil(t, sent.GetMessageGroup())
	assert.Equal(t, "42", *sent.GetMessageGroup())
}

func TestPublishWithoutShardingKeyKeepsMessageGroupEmpty(t *testing.T) {
	sender := &fakeSender{}
	producer := newTestProducer(sender)

	require.NoError(t, producer.Publish(context.Background(), Message{Topic: "aim-presence-events", Body: []byte("{}")}))
	require.Len(t, sender.sent, 1)
	assert.Nil(t, sender.sent[0].GetMessageGroup())
	assert.Empty(t, sender.sent[0].GetKeys())
}

func TestPublishRequiresTopicAndPropagatesErrors(t *testing.T) {
	sender := &fakeSender{}
	producer := newTestProducer(sender)

	require.Error(t, producer.Publish(context.Background(), Message{}))

	sender.sendErr = errors.New("broker down")
	err := producer.Publish(context.Background(), Message{Topic: "aim-typing-events"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broker down")
}

func TestNilProducerIsNoop(t *testing.T) {
	var producer *Producer

	assert.NoError(t, producer.Publish(context.Background(), Message{Topic: "aim-typing-events"}))
	assert.NoError(t, producer.Start())
	assert.NoError(t, producer.GracefulStop())
	assert.Nil(t, producer.WithJournalChecker(nil))
}

func TestSendTransactionalCommitsAfterLocalTransaction(t *testing.T) {
	sender := &fakeSender{}
	producer := newTestProducer(sender)

	var journals []EventJournal
	producer.journalTx = journalStub(&journals, nil)

	ran := false
	err := producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{
		Topic:    "aim-user-events",
		EventUID: "user.created:7",
		Key:      "7",
		Body:     []byte(`{"user_id":7}`),
	}, func(context.Context, *gorm.DB) error {
		ran = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, ran)

	require.Len(t, sender.sentTx, 1)
	assert.Equal(t, []string{"user.created:7", "7"}, sender.sentTx[0].GetKeys())
	assert.Equal(t, []byte(`{"user_id":7}`), sender.sentTx[0].Body)

	require.Len(t, journals, 1)
	assert.Equal(t, EventJournal{EventUID: "user.created:7", Topic: "aim-user-events", MsgKey: "7"}, journals[0])

	require.NotNil(t, sender.tx)
	assert.Equal(t, 1, sender.tx.commits)
	assert.Zero(t, sender.tx.rollbacks)
}

func TestSendTransactionalRollsBackWhenLocalTransactionFails(t *testing.T) {
	sender := &fakeSender{}
	producer := newTestProducer(sender)

	localErr := errors.New("insert credential failed")
	var journals []EventJournal
	producer.journalTx = journalStub(&journals, nil)

	err := producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{
		Topic:    "aim-user-events",
		EventUID: "user.created:8",
	}, func(context.Context, *gorm.DB) error {
		return localErr
	})
	require.ErrorIs(t, err, localErr)
	require.Len(t, journals, 1)

	require.NotNil(t, sender.tx)
	assert.Zero(t, sender.tx.commits)
	assert.Equal(t, 1, sender.tx.rollbacks)
}

func TestSendTransactionalSkipsLocalTransactionWhenHalfMessageFails(t *testing.T) {
	sender := &fakeSender{sendTxErr: errors.New("proxy unreachable")}
	producer := newTestProducer(sender)

	var journals []EventJournal
	producer.journalTx = journalStub(&journals, nil)

	ran := false
	err := producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{
		Topic:    "aim-user-events",
		EventUID: "user.created:9",
	}, func(context.Context, *gorm.DB) error {
		ran = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, ran, "no business row may be written when the half message cannot be sent")
	assert.Empty(t, journals)
	assert.Zero(t, sender.tx.rollbacks, "an unsent half message needs no rollback")
}

func TestSendTransactionalRejectsIncompleteMessages(t *testing.T) {
	producer := newTestProducer(&fakeSender{})

	noop := func(context.Context, *gorm.DB) error { return nil }

	require.Error(t, producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{Topic: "aim-user-events"}, noop))
	require.Error(t, producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{EventUID: "x"}, noop))
	require.Error(t, producer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{Topic: "t", EventUID: "x"}, nil))
	require.Error(t, producer.SendTransactional(context.Background(), nil, TransactionalMessage{Topic: "t", EventUID: "x"}, noop))

	var nilProducer *Producer
	require.Error(t, nilProducer.SendTransactional(context.Background(), &gorm.DB{}, TransactionalMessage{Topic: "t", EventUID: "x"}, noop))
}

func TestResolveWithoutJournalDatabaseIsUnknown(t *testing.T) {
	producer := newTestProducer(&fakeSender{})
	assert.Equal(t, rmq.UNKNOWN, producer.resolve("aim-user-events", []string{"user.created:1"}))
}

func TestResolveWithoutKeysIsUnknown(t *testing.T) {
	producer := newTestProducer(&fakeSender{})
	producer.journals.Store(&gorm.DB{})

	assert.Equal(t, rmq.UNKNOWN, producer.resolve("aim-user-events", nil))
}
