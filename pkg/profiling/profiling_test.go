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

func TestStart_BuildsTagsFromEnv(t *testing.T) {
	t.Setenv("POD_NAME", "ratings-abc123")
	t.Setenv("POD_NAMESPACE", "bookinfo")

	tags := profiling.BuildTags()

	if tags["pod"] != "ratings-abc123" {
		t.Errorf("expected pod tag 'ratings-abc123', got %q", tags["pod"])
	}
	if tags["namespace"] != "bookinfo" {
		t.Errorf("expected namespace tag 'bookinfo', got %q", tags["namespace"])
	}
}

func TestStart_BuildsTagsOmitsEmptyEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")

	tags := profiling.BuildTags()

	if _, ok := tags["pod"]; ok {
		t.Error("expected pod tag to be absent when POD_NAME is empty")
	}
	if _, ok := tags["namespace"]; ok {
		t.Error("expected namespace tag to be absent when POD_NAMESPACE is empty")
	}
}
