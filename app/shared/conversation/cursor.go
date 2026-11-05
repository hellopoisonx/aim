package conversation

import "time"

// MessageCursor is a position in a conversation's stored message history.
// The zero cursor addresses the newest page.
type MessageCursor struct {
	CreatedAt time.Time
	ID        int64
}

// IsZero reports whether the cursor addresses the newest page.
func (c MessageCursor) IsZero() bool {
	return c.CreatedAt.IsZero() && c.ID == 0
}
