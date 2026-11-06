package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicGatewayPush carries chat message pushes from core to every gateway node.
// It is a FIFO topic keyed by conversation id, so each conversation stays in
// order.
const TopicGatewayPush = "aim-gateway-push"

// SenderInfo is the sender snapshot carried with a push so gateways never have
// to look the sender up per recipient.
type SenderInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PushEvent is one chat message fan-out published by core.
//
// Exactly one of TargetUserID and MemberIDs is set:
//   - direct conversations write fan out: one event per recipient, so every
//     recipient gets its own copy (TargetUserID set);
//   - group conversations read fan out: a single conversation event carries the
//     member snapshot and each gateway node fans it out to its local
//     connections (MemberIDs set).
type PushEvent struct {
	tracing.TraceContextFields

	MessageID        int64       `json:"message_id"`
	ConversationID   int64       `json:"conversation_id"`
	ConversationType string      `json:"conversation_type"`
	MessageType      string      `json:"message_type"`
	Content          string      `json:"content"`
	SenderID         int64       `json:"sender_id"`
	SentAt           int64       `json:"sent_at"`
	ClientMsgID      string      `json:"client_msg_id"`
	Mentions         []string    `json:"mentions"`
	SenderInfo       *SenderInfo `json:"sender_info,omitempty"`
	SourceDeviceID   string      `json:"source_device_id"`

	TargetUserID int64   `json:"target_user_id,omitempty"`
	MemberIDs    []int64 `json:"member_ids,omitempty"`
}
