package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicTyping carries typing notices from the gateway to core, which fans them
// out to conversation members.
const TopicTyping = "aim-typing-events"

// TypingEvent is the broker message for a typing notification.
type TypingEvent struct {
	tracing.TraceContextFields

	FromUserID     int64  `json:"from_user_id"`
	ConversationID int64  `json:"conversation_id"`
	Timestamp      int64  `json:"timestamp"`
	GatewayNodeID  string `json:"gateway_node_id"`
}
