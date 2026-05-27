package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	parserpkg "github.com/hellopoisonx/aim/app/data_parsing/internal/parser"
	sharedattachment "github.com/hellopoisonx/aim/app/shared/attachment"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/stretchr/testify/require"
)

type recordingParser struct {
	called bool
}

func (p *recordingParser) Parse(ctx context.Context, kind, mime string, data []byte) (*parserpkg.Result, error) {
	p.called = true
	return nil, errors.New("unexpected parse call")
}

func TestConsumeSkipsGenericFileWithoutParsing(t *testing.T) {
	p := &recordingParser{}
	w := &Worker{parser: p}
	event := events.AttachmentUploadedEvent{FileID: "file-id", Kind: sharedattachment.KindFile, Mime: "application/pdf"}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	err = w.Consume(context.Background(), event.FileID, string(payload))
	require.NoError(t, err)
	require.False(t, p.called)
}
