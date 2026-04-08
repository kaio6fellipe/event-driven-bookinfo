package config_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "test-service")
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("HTTPPort = %q, want %q", cfg.HTTPPort, "8080")
	}
	if cfg.AdminPort != "9090" {
		t.Errorf("AdminPort = %q, want %q", cfg.AdminPort, "9090")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.StorageBackend != "memory" {
		t.Errorf("StorageBackend = %q, want %q", cfg.StorageBackend, "memory")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "")
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("OTLPEndpoint = %q, want %q", cfg.OTLPEndpoint, "")
	}
	if cfg.PyroscopeServerAddress != "" {
		t.Errorf("PyroscopeServerAddress = %q, want %q", cfg.PyroscopeServerAddress, "")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("SERVICE_NAME", "overridden")
	t.Setenv("HTTP_PORT", "3000")
	t.Setenv("ADMIN_PORT", "3001")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("STORAGE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4317")
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://pyro:4040")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ServiceName != "overridden" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "overridden")
	}
	if cfg.HTTPPort != "3000" {
		t.Errorf("HTTPPort = %q, want %q", cfg.HTTPPort, "3000")
	}
	if cfg.AdminPort != "3001" {
		t.Errorf("AdminPort = %q, want %q", cfg.AdminPort, "3001")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.StorageBackend != "postgres" {
		t.Errorf("StorageBackend = %q, want %q", cfg.StorageBackend, "postgres")
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://localhost/test")
	}
	if cfg.OTLPEndpoint != "http://otel:4317" {
		t.Errorf("OTLPEndpoint = %q, want %q", cfg.OTLPEndpoint, "http://otel:4317")
	}
	if cfg.PyroscopeServerAddress != "http://pyro:4040" {
		t.Errorf("PyroscopeServerAddress = %q, want %q", cfg.PyroscopeServerAddress, "http://pyro:4040")
	}
}

func TestLoad_MissingServiceName(t *testing.T) {
	t.Setenv("SERVICE_NAME", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing SERVICE_NAME, got nil")
	}
}

func TestLoad_PostgresWithoutDatabaseURL(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("STORAGE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for postgres without DATABASE_URL, got nil")
	}
}
