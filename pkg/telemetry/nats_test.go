package telemetry_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestInjectTraceContextNATS_WithActiveSpan_AddsTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	msg := &nats.Msg{Subject: "s", Header: nats.Header{}}
	telemetry.InjectTraceContextNATS(ctx, msg)

	got := msg.Header.Get("traceparent")
	if got == "" {
		t.Fatal("expected traceparent header, got none")
	}

	want := span.SpanContext().TraceID().String()
	if len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent trace_id = %q, want trace_id %q embedded", got, want)
	}
}

func TestInjectTraceContextNATS_NoActiveSpan_NoTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	msg := &nats.Msg{Subject: "s", Header: nats.Header{}}
	telemetry.InjectTraceContextNATS(context.Background(), msg)

	if got := msg.Header.Get("traceparent"); got != "" {
		t.Fatalf("expected no traceparent header, got %q", got)
	}
}

func TestInjectTraceContextNATS_NilHeader_Initialises(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "p")
	defer span.End()

	msg := &nats.Msg{Subject: "s"}
	telemetry.InjectTraceContextNATS(ctx, msg)

	if msg.Header == nil {
		t.Fatal("Header should be initialised when nil")
	}
	if msg.Header.Get("traceparent") == "" {
		t.Fatal("traceparent should be set after init + inject")
	}
}

func TestInjectTraceContextNATS_PreservesExistingHeaders(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "p")
	defer span.End()

	msg := &nats.Msg{
		Subject: "s",
		Header:  nats.Header{"Ce-Type": []string{"com.example.x"}},
	}
	telemetry.InjectTraceContextNATS(ctx, msg)

	if got := msg.Header.Get("Ce-Type"); got != "com.example.x" {
		t.Errorf("Ce-Type header was lost or mutated: got %q", got)
	}
	if msg.Header.Get("traceparent") == "" {
		t.Error("traceparent was not added")
	}
}

func TestStartNATSProducerSpan_AttachesSpanToContext(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	defer parent.End()

	ctx, span := telemetry.StartNATSProducerSpan(parentCtx, "raw_books_details", "idem-1")
	defer span.End()

	got := trace.SpanFromContext(ctx)
	if got.SpanContext().SpanID() == parent.SpanContext().SpanID() {
		t.Fatalf("ctx still references parent span %q; expected new producer span",
			parent.SpanContext().SpanID().String())
	}
	if !got.SpanContext().IsValid() {
		t.Fatal("returned ctx has no valid span context")
	}
	if got.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("trace_id = %s, want inherited %s",
			got.SpanContext().TraceID(), parent.SpanContext().TraceID())
	}
}

func TestStartNATSProducerSpan_NoParent_StillReturnsValidSpan(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	ctx, span := telemetry.StartNATSProducerSpan(context.Background(), "raw_books_details", "idem-1")
	defer span.End()

	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Fatal("expected valid span context even without a parent span")
	}
}
