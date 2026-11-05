package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicReadReceipt carries read-cursor advances from the gateway to core. The
// topic is FIFO, so conversation_id doubles as the message group.
const TopicReadReceipt = "aim-read-receipt-events"

// ReadReceiptEvent is published when a member advances their read cursor.
type ReadReceiptEvent struct {
	tracing.TraceContextFields

	FromUserID        int64  `json:"from_user_id"`
	ConversationID    int64  `json:"conversation_id"`
	LastReadMessageID int64  `json:"last_read_message_id"`
	UpdatedAt         int64  `json:"updated_at"`
	GatewayNodeID     string `json:"gateway_node_id"`
}
