package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicPresence carries presence changes from the gateway to core, which fans
// them out to the user's friends.
const TopicPresence = "aim-presence-events"

// PresenceEvent reports a user's connectivity change on one gateway node.
type PresenceEvent struct {
	tracing.TraceContextFields

	UserID        int64  `json:"user_id"`
	DeviceID      string `json:"device_id"`
	Status        string `json:"status"`
	UpdatedAt     int64  `json:"updated_at"`
	GatewayNodeID string `json:"gateway_node_id"`
}
