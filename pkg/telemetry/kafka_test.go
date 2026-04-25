package telemetry_test

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestInjectTraceContext_WithActiveSpan_AddsTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	rec := &kgo.Record{Topic: "t"}
	telemetry.InjectTraceContext(ctx, rec)

	headers := map[string]string{}
	for _, h := range rec.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}

	want := span.SpanContext().TraceID().String()
	got := headers["traceparent"]
	if len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent trace_id = %q, want trace_id %q embedded", got, want)
	}
}

func TestInjectTraceContext_NoActiveSpan_NoTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	rec := &kgo.Record{Topic: "t"}
	telemetry.InjectTraceContext(context.Background(), rec)

	for _, h := range rec.Headers {
		if h.Key == "traceparent" {
			t.Fatalf("expected no traceparent header, got %q", string(h.Value))
		}
	}
}

func TestInjectTraceContext_PreservesExistingHeaders(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "p")
	defer span.End()

	rec := &kgo.Record{
		Topic: "t",
		Headers: []kgo.RecordHeader{
			{Key: "ce_type", Value: []byte("com.example.x")},
		},
	}
	telemetry.InjectTraceContext(ctx, rec)

	var hasCE, hasTP bool
	for _, h := range rec.Headers {
		if h.Key == "ce_type" {
			hasCE = true
		}
		if h.Key == "traceparent" {
			hasTP = true
		}
	}
	if !hasCE {
		t.Error("ce_type header was lost")
	}
	if !hasTP {
		t.Error("traceparent was not added")
	}
}

func TestStartProducerSpan_AttachesSpanToContext(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	defer parent.End()

	ctx, span := telemetry.StartProducerSpan(parentCtx, "my_topic", "my_key")
	defer span.End()

	// The returned ctx must carry the new span (not the parent).
	got := trace.SpanFromContext(ctx)
	if got.SpanContext().SpanID() == parent.SpanContext().SpanID() {
		t.Fatalf("ctx still references parent span %q; expected new producer span",
			parent.SpanContext().SpanID().String())
	}
	if !got.SpanContext().IsValid() {
		t.Fatal("returned ctx has no valid span context")
	}
	// Trace ID is inherited from parent.
	if got.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("trace_id = %s, want inherited %s",
			got.SpanContext().TraceID(), parent.SpanContext().TraceID())
	}
}

func TestStartProducerSpan_NoParent_StillReturnsValidSpan(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	ctx, span := telemetry.StartProducerSpan(context.Background(), "my_topic", "my_key")
	defer span.End()

	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Fatal("expected valid span context even without a parent span")
	}
}
