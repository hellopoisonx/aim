package logic

import (
	"context"
	"time"
)

// idempotencyStore checks and stores idempotency keys.
// The store maps a key (e.g. "idempotency:transfer:{sender}:{device}:{client_msg_id}")
// to a server-generated message ID. If the key exists, the request is a duplicate
// and the stored message ID should be returned instead of processing again.
type idempotencyStore interface {
	// Check returns (exists, messageID, error). If the key exists, messageID
	// is the server-assigned message ID from the original request.
	Check(ctx context.Context, key string) (exists bool, messageID int64, err error)
	// Set stores the message ID under the given key with the specified TTL.
	Set(ctx context.Context, key string, messageID int64, ttl time.Duration) error
}

// messagePublisher publishes transfer events to a message queue (Kafka).
// The key is used for partition routing; the value is the serialized event payload.
type messagePublisher interface {
	// Publish sends the event to the queue. The key is used for partition routing.
	Publish(ctx context.Context, key string, value []byte) error
}