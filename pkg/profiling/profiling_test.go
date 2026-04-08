// file: pkg/profiling/profiling_test.go
package profiling_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
)

func TestStart_NoOpWhenUnset(t *testing.T) {
	cfg := &config.Config{
		ServiceName:            "test-service",
		PyroscopeServerAddress: "",
	}

	stop, err := profiling.Start(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stop == nil {
		t.Fatal("stop func should not be nil")
	}

	// No-op stop should not panic
	stop()
}
