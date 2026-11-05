package push

import (
	"testing"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/stretchr/testify/require"
)

func TestPositiveUniqueIDsDropsDuplicatesAndNonPositive(t *testing.T) {
	require.Equal(t, []int64{3, 1}, PositiveUniqueIDs([]int64{3, 0, -1, 1, 3}))
	require.Empty(t, PositiveUniqueIDs(nil))
}

func TestToMemberNodesSkipsSenderAndOriginNode(t *testing.T) {
	svcCtx := &svc.ServiceContext{PresenceStore: fakePresenceDirectory{nodes: []string{"node-a", "node-b"}}}

	calls := make(map[int64][]string)
	err := ToMemberNodes(t.Context(), svcCtx, []int64{42, 43}, 42, "node-b", func(memberID int64, nodeID string) error {
		calls[memberID] = append(calls[memberID], nodeID)
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, calls[42], "sender is skipped")
	require.Equal(t, []string{"node-a"}, calls[43], "origin node is skipped")
}
