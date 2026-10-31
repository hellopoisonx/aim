// Package rocketmq centralises AIM's RocketMQ (gRPC proxy protocol) wiring:
// producer construction, the simple-consumer loop used by every consumer
// service, and the transactional-message plumbing that replaced the removed
// outbox poller.
//
// # Message reliability model
//
// Normal topics are published synchronously and consumed at-least-once: a
// handler error leaves the message unacknowledged, so the broker redelivers it
// after the invisible duration. Handlers must therefore be idempotent.
//
// Events that must be published exactly when their local database transaction
// commits (user.created, conversation events, attachment.uploaded) use
// transactional messages: the half message is sent first, the local transaction
// writes business rows plus an event_journal row, and the commit/rollback
// resolution follows. A lost resolution is resolved by the broker's check-back,
// which consults event_journal (see [Producer.WithJournalChecker]).
package rocketmq

//lint:file-ignore SA5008 go-zero conf uses json tag options for defaults.

import (
	"github.com/apache/rocketmq-clients/golang/v5/credentials"
)

// EndpointConf is the connection block shared by producers and consumers.
type EndpointConf struct {
	// Endpoint is the proxy gRPC address, e.g. rocketmq-proxy:8081.
	Endpoint string `json:",optional"`
	// AccessKey and SecretKey are optional ACL credentials.
	AccessKey string `json:",optional"`
	SecretKey string `json:",optional"`
}

// sessionCredentials builds SDK credentials. The SDK treats a nil
// Credentials pointer as "sign nothing", which is what deployments without
// ACL need; an empty bucket would otherwise produce an invalid signature.
func (c EndpointConf) sessionCredentials() *credentials.SessionCredentials {
	if c.AccessKey == "" && c.SecretKey == "" {
		return nil
	}
	return &credentials.SessionCredentials{AccessKey: c.AccessKey, AccessSecret: c.SecretKey}
}

// ProducerConf configures one producer.
type ProducerConf struct {
	EndpointConf
	// Group is the producer group reported to the broker.
	Group string `json:",optional"`
	// Topic is the topic this producer sends to.
	Topic string `json:",optional"`
}

// IsConfigured reports whether the producer has enough configuration to start.
func (c ProducerConf) IsConfigured() bool {
	return c.Endpoint != "" && c.Topic != ""
}

// ConsumerConf configures one consumer service.
type ConsumerConf struct {
	EndpointConf
	// Name labels this consumer in logs.
	Name string `json:",optional"`
	// Group is the consumer group; it defines the offset namespace.
	Group string `json:",optional"`
	// Topic is the subscribed topic.
	Topic string `json:",optional"`
	// Concurrency is the number of receive loops. FIFO topics MUST stay at 1:
	// a queue is only ordered while one goroutine acks it sequentially.
	Concurrency int `json:",default=1"`
}

// IsConfigured reports whether the consumer has enough configuration to start.
func (c ConsumerConf) IsConfigured() bool {
	return c.Endpoint != "" && c.Topic != ""
}
