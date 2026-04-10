package database_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
)

func TestNewPoolConfig_HasTracer(t *testing.T) {
	// Use a dummy URL — we only need to verify config parsing, not connectivity.
	databaseURL := "postgres://user:pass@localhost:5432/testdb"

	cfg, err := database.NewPoolConfig(databaseURL)
	if err != nil {
		t.Fatalf("NewPoolConfig returned error: %v", err)
	}
	if cfg.ConnConfig.Tracer == nil {
		t.Fatal("expected ConnConfig.Tracer to be set, got nil")
	}
}

func TestNewPoolConfig_InvalidURL(t *testing.T) {
	_, err := database.NewPoolConfig("not-a-valid-url://")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
