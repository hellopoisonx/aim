package rocketmq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	rmq "github.com/apache/rocketmq-clients/golang/v5"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

const envConsoleAppender = "mq.consoleAppender.enabled"

var configureLoggerOnce sync.Once

// configureLogger routes the SDK's zap logger to stdout so container and dev
// logs stay on one stream. Without it the SDK writes rotating files under the
// process working directory.
func configureLogger() {
	configureLoggerOnce.Do(func() {
		_ = os.Setenv(envConsoleAppender, "true")
		rmq.ResetLogger()
	})
}

// messageSender is the SDK producer surface AIM depends on, narrowed so tests
// can substitute a fake.
type messageSender interface {
	Start() error
	GracefulStop() error
	Send(ctx context.Context, msg *rmq.Message) ([]*rmq.SendReceipt, error)
	SendWithTransaction(ctx context.Context, msg *rmq.Message, transaction rmq.Transaction) ([]*rmq.SendReceipt, error)
	BeginTransaction() rmq.Transaction
}

// Message is one event to publish.
type Message struct {
	// Topic is the destination topic.
	Topic string
	// Key lands in the message keys, keeping the consumer's key argument
	// meaningful (user id, conversation id, file id...).
	Key string
	// ShardingKey is the FIFO message group. Messages sharing a sharding key
	// are delivered in order; leave empty for NORMAL topics.
	ShardingKey string
	// Body is the JSON payload.
	Body []byte
}

// Producer wraps the SDK producer with AIM's message shape.
//
// A nil *Producer is safe to use: Start, GracefulStop and Publish become no-ops
// so services can wire an optional producer unconditionally.
type Producer struct {
	producer messageSender
	group    string
	// journals is the event_journal handle used by transaction check-back.
	journals atomic.Pointer[gorm.DB]
	// journalTx runs the local transaction of a transactional publish.
	journalTx journalTx
}

// NewProducer builds a producer for the given topics.
//
// Journal-backed transaction check-back is enabled separately by
// [Producer.WithJournalChecker]; the checker closure is installed up front so
// enabling it after construction works.
func NewProducer(cfg EndpointConf, group string, topics ...string) (*Producer, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("rocketmq: endpoint is required")
	}
	if group == "" {
		return nil, errors.New("rocketmq: producer group is required")
	}
	configureLogger()

	p := &Producer{group: group, journalTx: gormJournalTx}
	checker := &rmq.TransactionChecker{Check: p.checkTransaction}
	producer, err := rmq.NewProducer(&rmq.Config{
		Endpoint:      cfg.Endpoint,
		ConsumerGroup: group,
		Credentials:   cfg.sessionCredentials(),
	}, rmq.WithTopics(topics...), rmq.WithTransactionChecker(checker))
	if err != nil {
		return nil, fmt.Errorf("rocketmq: create producer %s: %w", group, err)
	}
	p.producer = producer
	return p, nil
}

// WithJournalChecker enables transaction check-back against db's event_journal
// table. Call it before Start.
func (p *Producer) WithJournalChecker(db *gorm.DB) *Producer {
	if p == nil {
		return p
	}
	p.journals.Store(db)
	return p
}

// Start connects the producer to the broker.
func (p *Producer) Start() error {
	if p == nil {
		return nil
	}
	if err := p.producer.Start(); err != nil {
		return fmt.Errorf("rocketmq: start producer %s: %w", p.group, err)
	}
	return nil
}

// GracefulStop flushes in-flight sends and disconnects.
func (p *Producer) GracefulStop() error {
	if p == nil {
		return nil
	}
	return p.producer.GracefulStop()
}

// Publish sends one message and waits for the broker acknowledgement.
func (p *Producer) Publish(ctx context.Context, m Message) error {
	if p == nil {
		return nil
	}
	if m.Topic == "" {
		return errors.New("rocketmq: publish requires a topic")
	}
	if _, err := p.producer.Send(ctx, newSDKMessage(m.Topic, m.Body, m.Key, m.ShardingKey)); err != nil {
		return fmt.Errorf("rocketmq: publish to %s: %w", m.Topic, err)
	}
	return nil
}

// newSDKMessage maps AIM's message shape onto the SDK's.
func newSDKMessage(topic string, body []byte, key, shardingKey string) *rmq.Message {
	msg := &rmq.Message{Topic: topic, Body: body}
	if key != "" {
		msg.SetKeys(key)
	}
	if shardingKey != "" {
		msg.SetMessageGroup(shardingKey)
	}
	return msg
}

// checkTransaction adapts the broker callback to resolve.
func (p *Producer) checkTransaction(view *rmq.MessageView) rmq.TransactionResolution {
	return p.resolve(view.GetTopic(), view.GetKeys())
}

// resolve answers the broker's check-back for a half message: an event_journal
// row means the local transaction committed, so the message may be delivered.
func (p *Producer) resolve(topic string, keys []string) rmq.TransactionResolution {
	db := p.journals.Load()
	if db == nil {
		logx.WithContext(context.Background()).
			Errorf("rocketmq: transaction check-back for %s without a journal database", topic)
		return rmq.UNKNOWN
	}
	if len(keys) == 0 {
		logx.WithContext(context.Background()).
			Errorf("rocketmq: transaction check-back for %s has no event_uid key", topic)
		return rmq.UNKNOWN
	}

	exists, err := JournalExists(context.Background(), db, keys[0])
	if err != nil {
		// Transient database errors must not decide the outcome: let the broker
		// ask again.
		logx.WithContext(context.Background()).
			Errorf("rocketmq: transaction check-back query %s failed: %v", keys[0], err)
		return rmq.UNKNOWN
	}
	if exists {
		return rmq.COMMIT
	}
	return rmq.ROLLBACK
}
