// Package kafka centralises Kafka producer construction.
//
// Services publish through go-queue's kq pusher; the only place that needs the
// low-level segmentio client is the balancer choice below. Keeping it here
// stops five service packages from importing the driver directly.
package kafka

import (
	"github.com/segmentio/kafka-go"
	"github.com/zeromicro/go-queue/kq"
)

// NewPusher builds a key-partitioned kq pusher.
//
// The Murmur2 balancer is pinned because AIM orders events per key (conversation
// or user id): a stable hash keeps every event with the same key on the same
// partition, which Kafka preserves delivery order within.
func NewPusher(brokers []string, topic string) *kq.Pusher {
	return kq.NewPusher(brokers, topic, kq.WithBalancer(&kafka.Murmur2Balancer{}))
}
