package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// TopicFriendApplication carries friend-application state changes from the
// gateway to core, which fans them out to every gateway node the target user is
// connected to.
const TopicFriendApplication = "aim.friend_application.events"

// FriendApplicationEvent announces a new or updated friendship to the target
// user's connected devices.
type FriendApplicationEvent struct {
	tracing.TraceContextFields

	// TargetUserID receives the notification.
	TargetUserID int64 `json:"target_user_id"`
	// UserID is the initiator of the application.
	UserID int64 `json:"user_id"`
	// FriendID is the application's other party (the recipient's user id).
	FriendID int64 `json:"friend_id"`
	// Status is the friendship status after this change (pending/accepted/...).
	Status string `json:"status"`
	// CreatedAt and UpdatedAt are millisecond timestamps of the friendship row.
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}
