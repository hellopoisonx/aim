// Package events defines the broker payloads exchanged between AIM services.
//
// One struct per topic keeps producers and consumers on the same contract:
// a service that publishes untyped maps or redeclares a private copy of the
// payload drifts silently the first time a field is added.
package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicMessageTransfer carries a message from core to delivery, archiving and
// WebSocket fan-out consumers.
const TopicMessageTransfer = "aim-message-transfer"

// TransferEvent is published by core TransferLogic after a message is accepted.
type TransferEvent struct {
	tracing.TraceContextFields

	MessageID      int64    `json:"message_id"`
	SenderID       int64    `json:"sender_id"`
	DeviceID       string   `json:"device_id"`
	ConversationID int64    `json:"conversation_id"`
	MessageType    string   `json:"message_type"`
	Content        string   `json:"content"`
	ClientMsgID    string   `json:"client_msg_id"`
	Mentions       []string `json:"mentions"`
	Timestamp      int64    `json:"timestamp"`
}
