package telemetry_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestInjectTraceContextNATS_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	msg := &nats.Msg{Header: nats.Header{}}
	telemetry.InjectTraceContextNATS(ctx, msg)
	_ = msg.Header.Get("traceparent")
}

func TestStartNATSProducerSpan_ReturnsValidSpan(t *testing.T) {
	ctx, span := telemetry.StartNATSProducerSpan(context.Background(), "subject.test", "idem-1")
	defer span.End()
	if ctx == nil {
		t.Fatal("ctx should not be nil")
	}
	if span == nil {
		t.Fatal("span should not be nil")
	}
}
