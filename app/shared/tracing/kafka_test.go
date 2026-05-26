package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

func TestDetachSpanContextPreservesContextValuesAndBaggage(t *testing.T) {
	t.Parallel()

	type contextKey string

	key := contextKey("key")

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})

	member, err := baggage.NewMember("tenant", "aim")
	require.NoError(t, err)
	bag, err := baggage.New(member)
	require.NoError(t, err)

	ctx := context.WithValue(context.Background(), key, "value")
	ctx = baggage.ContextWithBaggage(ctx, bag)
	ctx = trace.ContextWithSpanContext(ctx, spanContext)

	detached := DetachSpanContext(ctx)

	require.Equal(t, "value", detached.Value(key))
	require.Equal(t, "aim", baggage.FromContext(detached).Member("tenant").Value())
	require.False(t, trace.SpanContextFromContext(detached).IsValid())
}
