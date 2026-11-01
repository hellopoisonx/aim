package rocketmq

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertJournalRequiresTransactionAndEventUID(t *testing.T) {
	require.Error(t, InsertJournal(context.Background(), nil, EventJournal{EventUID: "user.created:1"}))
	require.Error(t, InsertJournal(context.Background(), nil, EventJournal{}))
}

func TestJournalTableName(t *testing.T) {
	assert.Equal(t, "event_journal", EventJournal{}.TableName())
}
