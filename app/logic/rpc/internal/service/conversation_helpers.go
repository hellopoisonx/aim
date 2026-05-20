package service

import (
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var conversationIDCounter atomic.Int64

func init() {
	conversationIDCounter.Store(time.Now().UnixMilli())
}

// GenerateConversationID generates a unique conversation ID.
// Uses a combination of current timestamp and an atomic counter.
// TODO: Replace with snowflake ID generator for production use.
func GenerateConversationID() int64 {
	return conversationIDCounter.Add(1)
}

// generateConversationID is the unexported version for use within the service package.
func generateConversationID() int64 {
	return GenerateConversationID()
}

// PGTimestamptzFromUnix converts a Unix millisecond timestamp to pgtype.Timestamptz.
// If unixMs is 0, returns an invalid (null) Timestamptz.
func PGTimestamptzFromUnix(unixMs int64) pgtype.Timestamptz {
	if unixMs == 0 {
		return pgtype.Timestamptz{Valid: false}
	}

	return pgtype.Timestamptz{
		Time:  time.UnixMilli(unixMs),
		Valid: true,
	}
}

// pgTimestamptzFromUnix is the unexported version for use within the service package.
func pgTimestamptzFromUnix(unixMs int64) pgtype.Timestamptz {
	return PGTimestamptzFromUnix(unixMs)
}

// UnixFromPGTimestamptz converts a pgtype.Timestamptz to Unix milliseconds.
// If the timestamp is invalid (null), returns 0.
func UnixFromPGTimestamptz(ts pgtype.Timestamptz) int64 {
	if !ts.Valid {
		return 0
	}

	return ts.Time.UnixMilli()
}

// unixFromPGTimestamptz is the unexported version for use within the service package.
func unixFromPGTimestamptz(ts pgtype.Timestamptz) int64 {
	return UnixFromPGTimestamptz(ts)
}