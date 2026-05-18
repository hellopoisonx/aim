package moderation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopChecker_AllowsAll(t *testing.T) {
	c := NoopChecker{}
	d, err := c.Check(context.Background(), "any content")
	require.NoError(t, err)
	require.True(t, d.Allowed)
}

func TestNoopChecker_ImplementsInterface(t *testing.T) {
	var c Checker = NoopChecker{}
	require.NotNil(t, c)
}