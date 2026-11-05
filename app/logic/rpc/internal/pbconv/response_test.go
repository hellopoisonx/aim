package pbconv

import (
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"

	"github.com/stretchr/testify/require"
)

// logic.proto documents UserInfoResponse.created_at/updated_at as unix timestamps in seconds.
func TestUserTimestampsAreUnixSeconds(t *testing.T) {
	at := time.Unix(1_700_000_000, 0)
	got := User(orm.UserInfo{ID: 1, CreatedAt: at, UpdatedAt: at})

	require.Equal(t, int64(1_700_000_000), got.GetCreatedAt())
	require.Equal(t, int64(1_700_000_000), got.GetUpdatedAt())
}
