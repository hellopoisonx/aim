package mqs

import (
	"testing"

	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/stretchr/testify/require"
)

// The consumer echoes metadata into message content, so unknown producer fields
// must survive the decode instead of being dropped by MediaMetadata.
func TestDecodeAttachmentParsedEventKeepsMetadataVerbatim(t *testing.T) {
	payload := `{"file_id":"f1","kind":"image","parse_status":"ready","width":10,` +
		`"metadata":{"mime":"image/png","future_field":"kept"},"parsed_at":7}`

	event, metadata, err := decodeAttachmentParsedEvent([]byte(payload))
	require.NoError(t, err)

	require.IsType(t, events.AttachmentParsedEvent{}, event)
	require.Equal(t, "f1", event.FileID)
	require.Equal(t, 10, event.Width)
	require.Equal(t, int64(7), event.ParsedAt)
	require.JSONEq(t, `{"mime":"image/png","future_field":"kept"}`, string(metadata))
}

func TestDecodeAttachmentParsedEventWithoutMetadata(t *testing.T) {
	event, metadata, err := decodeAttachmentParsedEvent([]byte(`{"file_id":"f1","parse_status":"failed"}`))
	require.NoError(t, err)
	require.Equal(t, "f1", event.FileID)
	require.Nil(t, metadata)
}

func TestPositiveUniqueIDsDropsDuplicatesAndNonPositive(t *testing.T) {
	require.Equal(t, []int64{3, 1}, positiveUniqueIDs([]int64{3, 0, -1, 1, 3}))
	require.Empty(t, positiveUniqueIDs(nil))
}

func TestPushToMemberNodesSkipsSenderAndOriginNode(t *testing.T) {
	svcCtx := newTestServiceContext(t, newFakeGatewayClient())
	svcCtx.PresenceStore = fakePresenceDirectory{nodes: []string{"node-a", "node-b"}}

	calls := make(map[int64][]string)
	err := pushToMemberNodes(t.Context(), svcCtx, []int64{42, 43}, 42, "node-b", func(memberID int64, nodeID string) error {
		calls[memberID] = append(calls[memberID], nodeID)
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, calls[42], "sender is skipped")
	require.Equal(t, []string{"node-a"}, calls[43], "origin node is skipped")
}
