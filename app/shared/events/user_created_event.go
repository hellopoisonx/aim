package events

import "github.com/hellopoisonx/aim/app/shared/tracing"

// UserCreatedEvent is published by auth service when a new user registers.
// It is consumed by logic service to create the corresponding user_info row.
type UserCreatedEvent struct {
	tracing.TraceContextFields

	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	CreatedAt int64  `json:"created_at"`
}
