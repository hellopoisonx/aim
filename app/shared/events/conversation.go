package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// ConversationEvent is the group-mutation system message logic publishes in the
// same transaction as the mutation; core fans it out to TargetUserIDs.
type ConversationEvent struct {
	tracing.TraceContextFields

	MessageID      int64   `json:"message_id"`
	ConversationID int64   `json:"conversation_id"`
	SenderID       int64   `json:"sender_id"`
	MessageType    string  `json:"message_type"`
	Content        string  `json:"content"`
	TargetUserIDs  []int64 `json:"target_user_ids"`
	Timestamp      int64   `json:"timestamp"`
}
