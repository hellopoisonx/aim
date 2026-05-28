package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/hellopoisonx/aim/app/shared/tracing"

// TraceContextFields carries W3C trace context through transports that do not expose message headers.
type TraceContextFields struct {
	TraceParent string `json:"traceparent,omitempty"`
	TraceState  string `json:"tracestate,omitempty"`
}

func InjectTraceContext(ctx context.Context) TraceContextFields {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)

	return TraceContextFields{
		TraceParent: carrier.Get("traceparent"),
		TraceState:  carrier.Get("tracestate"),
	}
}

func ExtractTraceContext(ctx context.Context, fields TraceContextFields) context.Context {
	carrier := propagation.MapCarrier{}
	if fields.TraceParent != "" {
		carrier.Set("traceparent", fields.TraceParent)
	}

	if fields.TraceState != "" {
		carrier.Set("tracestate", fields.TraceState)
	}

	return propagation.TraceContext{}.Extract(ctx, carrier)
}

// DetachSpanContext returns a copy of ctx that preserves cancellation, deadlines,
// baggage and other values, but clears the active span context.
//
// Use this for long-lived transports (for example, a WebSocket session after the
// HTTP upgrade is complete) before creating short-lived per-message spans. This
// avoids exporting spans whose parent is the still-open upgrade/request span;
// some tracing backends delay or warn about children whose parent has not been
// exported yet.
func DetachSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, trace.SpanContext{})
}

func StartKafkaConsumerSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attrs...))
}

func StartKafkaProducerSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attrs...))
}

func RecordSpanError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
