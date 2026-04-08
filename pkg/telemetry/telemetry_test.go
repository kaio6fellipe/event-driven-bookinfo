package telemetry_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestSetup_NoOpWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := telemetry.Setup(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func should not be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}
