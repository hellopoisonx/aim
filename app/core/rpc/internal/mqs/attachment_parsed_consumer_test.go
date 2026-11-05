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
