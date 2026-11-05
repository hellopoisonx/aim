package pbconv

import (
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"

	"github.com/stretchr/testify/require"
)

// logic.proto documents UserInfoResponse.created_at/updated_at as unix milliseconds.
func TestUserTimestampsAreUnixMilliseconds(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_123)
	got := User(orm.UserInfo{ID: 1, CreatedAt: at, UpdatedAt: at})

	require.Equal(t, int64(1_700_000_000_123), got.GetCreatedAt())
	require.Equal(t, int64(1_700_000_000_123), got.GetUpdatedAt())
}
