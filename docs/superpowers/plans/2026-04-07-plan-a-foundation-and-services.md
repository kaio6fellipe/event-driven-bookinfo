# Event-Driven Bookinfo -- Plan A: Foundation & Services

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared Go packages and all 5 Bookinfo services with in-memory storage, producing a fully working event-driven e-commerce system.

**Architecture:** Hexagonal architecture monorepo with shared `pkg/` packages (config, logging, metrics, profiling, telemetry, health, server) and 5 services (ratings, details, reviews, notification, productpage). Each backend service follows ports & adapters with in-memory storage. productpage is a BFF using Go html/template + HTMX.

**Tech Stack:** Go 1.25, net/http, log/slog, OTel (tracing + metrics -> Prometheus), Pyroscope SDK, HTMX

**Design Spec:** `docs/superpowers/specs/2026-04-07-bookinfo-monorepo-design.md`

---

## Task 1: Module Setup & Config Package

### Task 1.1 -- Update go.mod

- [ ] Replace the contents of `go.mod` with the new module name and dependencies:

```go
// file: go.mod
module github.com/kaio6fellipe/event-driven-bookinfo

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/grafana/pyroscope-go v1.2.4
	go.opentelemetry.io/contrib/bridges/otelslog v0.11.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0
	go.opentelemetry.io/contrib/instrumentation/runtime v0.67.0
	go.opentelemetry.io/otel v1.42.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.42.0
	go.opentelemetry.io/otel/exporters/prometheus v0.54.0
	go.opentelemetry.io/otel/metric v1.42.0
	go.opentelemetry.io/otel/sdk v1.42.0
	go.opentelemetry.io/otel/sdk/metric v1.42.0
	go.opentelemetry.io/otel/trace v1.42.0
)
```

- [ ] Run `go mod tidy` to resolve all indirect dependencies.

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go mod tidy
```

- [ ] Verify the module compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./...
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add go.mod go.sum && git commit -m "chore: rename module to github.com/kaio6fellipe/event-driven-bookinfo and add deps"
```

### Task 1.2 -- Create pkg/config/config.go

- [ ] Write the test first at `pkg/config/config_test.go`:

```go
// file: pkg/config/config_test.go
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
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/config/...
# Expected: compilation error / test failure (config package does not exist yet)
```

- [ ] Write the implementation at `pkg/config/config.go`:

```go
// file: pkg/config/config.go
package config

import (
	"fmt"
	"os"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	ServiceName            string
	HTTPPort               string
	AdminPort              string
	LogLevel               string
	StorageBackend         string
	DatabaseURL            string
	OTLPEndpoint           string
	PyroscopeServerAddress string
}

// Load reads configuration from environment variables and returns a Config.
// SERVICE_NAME is required. All other fields have sensible defaults.
// When STORAGE_BACKEND is "postgres", DATABASE_URL is required.
func Load() (*Config, error) {
	cfg := &Config{
		ServiceName:            os.Getenv("SERVICE_NAME"),
		HTTPPort:               envOrDefault("HTTP_PORT", "8080"),
		AdminPort:              envOrDefault("ADMIN_PORT", "9090"),
		LogLevel:               envOrDefault("LOG_LEVEL", "info"),
		StorageBackend:         envOrDefault("STORAGE_BACKEND", "memory"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		OTLPEndpoint:           os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
	}

	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("SERVICE_NAME environment variable is required")
	}

	if cfg.StorageBackend == "postgres" && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required when STORAGE_BACKEND is postgres")
	}

	return cfg, nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/config/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/config/ && git commit -m "feat: add pkg/config with env-based config loading and validation"
```

---

## Task 2: Health Package

### Task 2.1 -- Create pkg/health/health.go

- [ ] Write the test first at `pkg/health/health_test.go`:

```go
// file: pkg/health/health_test.go
package health_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/health"
)

func TestLivenessHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	health.LivenessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestReadinessHandler_NoChecks(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestReadinessHandler_ChecksPassing(t *testing.T) {
	passingCheck := func() error { return nil }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler(passingCheck).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestReadinessHandler_CheckFailing(t *testing.T) {
	failingCheck := func() error { return errors.New("db connection failed") }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler(failingCheck).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "not ready" {
		t.Errorf("status = %q, want %q", body["status"], "not ready")
	}
}

func TestReadinessHandler_MultipleChecks_OneFailure(t *testing.T) {
	passingCheck := func() error { return nil }
	failingCheck := func() error { return errors.New("cache unreachable") }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.ReadinessHandler(passingCheck, failingCheck).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/health/...
# Expected: compilation error (health package does not exist yet)
```

- [ ] Write the implementation at `pkg/health/health.go`:

```go
// file: pkg/health/health.go
package health

import (
	"encoding/json"
	"net/http"
)

// LivenessHandler returns an http.Handler that responds with 200 and {"status":"ok"}.
// Used for Kubernetes liveness probes at /healthz.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

// ReadinessHandler returns an http.Handler that runs all check functions.
// If all checks pass (or no checks are provided), it responds with 200 and {"status":"ready"}.
// If any check fails, it responds with 503 and {"status":"not ready"}.
// Used for Kubernetes readiness probes at /readyz.
func ReadinessHandler(checks ...func() error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/health/...
# Expected: all 5 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/health/ && git commit -m "feat: add pkg/health with liveness and readiness handlers"
```

---

## Task 3: Telemetry Package (Tracing)

### Task 3.1 -- Create pkg/telemetry/telemetry.go

- [ ] Write the test first at `pkg/telemetry/telemetry_test.go`:

```go
// file: pkg/telemetry/telemetry_test.go
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

	// No-op shutdown should not error
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/telemetry/...
# Expected: compilation error (telemetry package does not exist yet)
```

- [ ] Write the implementation at `pkg/telemetry/telemetry.go`:

```go
// file: pkg/telemetry/telemetry.go
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup initializes the OpenTelemetry tracing pipeline for the given service name.
// When OTEL_EXPORTER_OTLP_ENDPOINT is not set, it returns a no-op shutdown function.
// The returned function should be called on application shutdown to flush pending spans.
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		// No-op: tracing not configured
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	r, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
		resource.WithAttributes(
			resource.Default().Attributes()...,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/telemetry/...
# Expected: 1 test passes
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/telemetry/ && git commit -m "feat: add pkg/telemetry with OTel tracing setup and no-op fallback"
```

---

## Task 4: Logging Package

### Task 4.1 -- Create pkg/logging/logging.go and pkg/logging/middleware.go

- [ ] Write the test first at `pkg/logging/logging_test.go`:

```go
// file: pkg/logging/logging_test.go
package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
)

func TestNew_IncludesServiceField(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	logger.Info("hello")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	if entry["service"] != "test-service" {
		t.Errorf("service = %v, want %q", entry["service"], "test-service")
	}
}

func TestNew_LogLevels(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantLevel slog.Level
	}{
		{name: "debug", level: "debug", wantLevel: slog.LevelDebug},
		{name: "info", level: "info", wantLevel: slog.LevelInfo},
		{name: "warn", level: "warn", wantLevel: slog.LevelWarn},
		{name: "error", level: "error", wantLevel: slog.LevelError},
		{name: "unknown defaults to info", level: "unknown", wantLevel: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewWithWriter(tt.level, "test-service", &buf)

			// Log at debug level -- should only appear if level is debug
			logger.Debug("debug message")

			if tt.wantLevel == slog.LevelDebug {
				if buf.Len() == 0 {
					t.Error("expected debug message to appear")
				}
			} else {
				if buf.Len() != 0 {
					t.Error("expected debug message to be suppressed")
				}
			}
		})
	}
}

func TestFromContext_ReturnsDefaultWhenMissing(t *testing.T) {
	logger := logging.FromContext(context.Background())
	if logger == nil {
		t.Fatal("expected non-nil logger from empty context")
	}
}

func TestWithContext_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	ctx := logging.WithContext(context.Background(), logger)
	got := logging.FromContext(ctx)

	if got != logger {
		t.Error("expected same logger from context round-trip")
	}
}

func TestMiddleware_AddsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	var capturedCtx context.Context
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}

	// Check that log output contains request_id
	output := buf.String()
	lines := splitJSONLines(output)

	foundRequestID := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if _, ok := entry["request_id"]; ok {
			foundRequestID = true
			break
		}
	}

	if !foundRequestID {
		t.Errorf("expected request_id in log output, got:\n%s", output)
	}
}

func TestMiddleware_UsesExistingRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	found := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["request_id"] == "existing-id-123" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected request_id=existing-id-123 in log output, got:\n%s", output)
	}
}

func TestMiddleware_LogsRequestCompletion(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	handler := logging.Middleware(logger)(inner)

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	foundCompletion := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] == "request completed" {
			foundCompletion = true
			if entry["method"] != "POST" {
				t.Errorf("method = %v, want POST", entry["method"])
			}
			if entry["path"] != "/api/test" {
				t.Errorf("path = %v, want /api/test", entry["path"])
			}
			// Status is stored as float64 in JSON
			if status, ok := entry["status"].(float64); !ok || int(status) != 201 {
				t.Errorf("status = %v, want 201", entry["status"])
			}
			if _, ok := entry["duration_ms"]; !ok {
				t.Error("expected duration_ms in completion log")
			}
			break
		}
	}

	if !foundCompletion {
		t.Errorf("expected 'request completed' log, got:\n%s", output)
	}
}

func splitJSONLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		current += string(c)
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/logging/...
# Expected: compilation error (logging package does not exist yet)
```

- [ ] Write the implementation at `pkg/logging/logging.go`:

```go
// file: pkg/logging/logging.go
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey struct{}

// New creates a JSON slog.Logger with the given level and service name.
// Output is written to os.Stdout.
func New(level string, serviceName string) *slog.Logger {
	return NewWithWriter(level, serviceName, os.Stdout)
}

// NewWithWriter creates a JSON slog.Logger that writes to the given writer.
// This is useful for testing where output needs to be captured.
func NewWithWriter(level string, serviceName string, w io.Writer) *slog.Logger {
	lvl := parseLevel(level)

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: lvl,
	})

	return slog.New(handler).With(
		slog.String("service", serviceName),
	)
}

// FromContext retrieves the request-scoped logger from the context.
// If no logger is found, a default logger is returned.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithContext stores a logger in the context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] Write the middleware at `pkg/logging/middleware.go`:

```go
// file: pkg/logging/middleware.go
package logging

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Middleware returns HTTP middleware that creates a request-scoped logger
// with request_id, method, path, and remote_addr fields.
// It logs request completion with status code and duration.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			reqLogger := logger.With(
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			)

			ctx := WithContext(r.Context(), reqLogger)
			r = r.WithContext(ctx)

			reqLogger.Debug("request started")

			wrapped := newResponseWriter(w)
			next.ServeHTTP(wrapped, r)

			reqLogger.Info("request completed",
				slog.Int("status", wrapped.statusCode),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000.0),
			)
		})
	}
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/logging/...
# Expected: all 7 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/logging/ && git commit -m "feat: add pkg/logging with slog JSON logger, context helpers, and HTTP middleware"
```

---

## Task 5: Metrics Package

### Task 5.1 -- Create pkg/metrics/metrics.go, middleware.go, runtime.go

- [ ] Write the test first at `pkg/metrics/metrics_test.go`:

```go
// file: pkg/metrics/metrics_test.go
package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
)

func TestSetup_ReturnsHandler(t *testing.T) {
	metricsHandler, err := metrics.Setup("test-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metricsHandler == nil {
		t.Fatal("expected non-nil metrics handler")
	}

	// Serve /metrics and check it returns prometheus text format
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	metricsHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	// Prometheus exposition format should contain at least a comment or metric line
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") && !strings.Contains(body, "target_info") {
		t.Errorf("expected prometheus format, got:\n%s", body)
	}
}

func TestMiddleware_RecordsMetrics(t *testing.T) {
	metricsHandler, err := metrics.Setup("test-middleware")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := metrics.Middleware("test-middleware")(inner)

	// Send a request through the middleware
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Now check /metrics has our custom metrics
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	metricsHandler.ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, "http_server_request_duration_seconds") {
		t.Errorf("expected http_server_request_duration_seconds metric, got:\n%s", body)
	}
	if !strings.Contains(body, "http_server_requests_total") {
		t.Errorf("expected http_server_requests_total metric, got:\n%s", body)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/metrics/...
# Expected: compilation error (metrics package does not exist yet)
```

- [ ] Write the implementation at `pkg/metrics/metrics.go`:

```go
// file: pkg/metrics/metrics.go
package metrics

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Setup creates an OTel meter provider with a Prometheus exporter and registers
// it as the global meter provider. It returns an http.Handler that serves the
// /metrics endpoint in Prometheus exposition format.
func Setup(serviceName string) (http.Handler, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	registry := promclient.NewRegistry()

	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(provider)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	return handler, nil
}
```

- [ ] Write the middleware at `pkg/metrics/middleware.go`:

```go
// file: pkg/metrics/middleware.go
package metrics

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// metricsResponseWriter wraps http.ResponseWriter to capture the status code.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Middleware returns HTTP middleware that records request metrics using OTel:
// - http_server_request_duration_seconds (histogram)
// - http_server_requests_total (counter)
// - http_server_active_requests (up-down counter / gauge)
func Middleware(serviceName string) func(http.Handler) http.Handler {
	meter := otel.Meter(serviceName)

	requestDuration, _ := meter.Float64Histogram(
		"http_server_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)

	requestsTotal, _ := meter.Int64Counter(
		"http_server_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)

	activeRequests, _ := meter.Int64UpDownCounter(
		"http_server_active_requests",
		metric.WithDescription("Number of active HTTP requests"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			methodAttr := attribute.String("method", r.Method)
			pathAttr := attribute.String("route", r.URL.Path)

			activeRequests.Add(r.Context(), 1, metric.WithAttributes(methodAttr))

			wrapped := newMetricsResponseWriter(w)
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start).Seconds()
			statusAttr := attribute.String("status", fmt.Sprintf("%d", wrapped.statusCode))

			requestDuration.Record(r.Context(), duration,
				metric.WithAttributes(methodAttr, pathAttr, statusAttr),
			)
			requestsTotal.Add(r.Context(), 1,
				metric.WithAttributes(methodAttr, pathAttr, statusAttr),
			)
			activeRequests.Add(r.Context(), -1, metric.WithAttributes(methodAttr))
		})
	}
}
```

- [ ] Write the runtime metrics at `pkg/metrics/runtime.go`:

```go
// file: pkg/metrics/runtime.go
package metrics

import (
	"log/slog"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

// RegisterRuntimeMetrics registers Go runtime metrics (goroutines, GC, memory)
// via the OTel runtime instrumentation package.
func RegisterRuntimeMetrics() {
	if err := runtime.Start(); err != nil {
		slog.Error("failed to start runtime metrics", "error", err)
	}
}
```

- [ ] Run `go mod tidy` to pull in the prometheus client dependency:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go mod tidy
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/metrics/...
# Expected: all tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/metrics/ go.mod go.sum && git commit -m "feat: add pkg/metrics with OTel Prometheus exporter, HTTP middleware, and runtime metrics"
```

---

## Task 6: Profiling Package

### Task 6.1 -- Create pkg/profiling/profiling.go

- [ ] Write the test first at `pkg/profiling/profiling_test.go`:

```go
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
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/profiling/...
# Expected: compilation error (profiling package does not exist yet)
```

- [ ] Write the implementation at `pkg/profiling/profiling.go`:

```go
// file: pkg/profiling/profiling.go
package profiling

import (
	"fmt"
	"runtime"

	"github.com/grafana/pyroscope-go"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
)

// Start initializes the Pyroscope profiling SDK if PyroscopeServerAddress is set.
// Returns a stop function to shut down the profiler. When the address is empty,
// returns a no-op stop function.
func Start(cfg *config.Config) (func(), error) {
	if cfg.PyroscopeServerAddress == "" {
		return func() {}, nil
	}

	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: cfg.ServiceName,
		ServerAddress:   cfg.PyroscopeServerAddress,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("starting Pyroscope profiler: %w", err)
	}

	return func() {
		profiler.Stop()
	}, nil
}
```

- [ ] Run `go mod tidy`:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go mod tidy
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./pkg/profiling/...
# Expected: 1 test passes
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/profiling/ go.mod go.sum && git commit -m "feat: add pkg/profiling with Pyroscope SDK wrapper and no-op fallback"
```

---

## Task 7: Server Package (Dual-Port)

### Task 7.1 -- Create pkg/server/server.go

- [ ] Write the test first at `pkg/server/server_test.go`:

```go
// file: pkg/server/server_test.go
package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
)

func getFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return fmt.Sprintf("%d", port)
}

func TestServer_BothPortsStart(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-server",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"pong": "true"})
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, nil)
	}()

	// Wait for servers to start
	waitForServer(t, httpPort)
	waitForServer(t, adminPort)

	// Test API port
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/v1/ping", httpPort))
	if err != nil {
		t.Fatalf("failed to reach API port: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("API status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["pong"] != "true" {
		t.Errorf("pong = %q, want %q", body["pong"], "true")
	}

	// Test admin healthz
	resp2, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", adminPort))
	if err != nil {
		t.Fatalf("failed to reach admin port: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("admin /healthz status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	var healthBody map[string]string
	json.NewDecoder(resp2.Body).Decode(&healthBody)
	if healthBody["status"] != "ok" {
		t.Errorf("healthz status = %q, want %q", healthBody["status"], "ok")
	}

	// Test admin readyz
	resp3, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/readyz", adminPort))
	if err != nil {
		t.Fatalf("failed to reach admin readyz: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Errorf("admin /readyz status = %d, want %d", resp3.StatusCode, http.StatusOK)
	}

	// Shut down
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-shutdown",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/slow", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, nil)
	}()

	waitForServer(t, httpPort)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestServer_AdminMetricsEndpoint(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-metrics-endpoint",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(mux *http.ServeMux) {}

	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("# HELP test_metric A test metric\n# TYPE test_metric counter\ntest_metric 42\n"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, metricsHandler)
	}()

	waitForServer(t, adminPort)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/metrics", adminPort))
	if err != nil {
		t.Fatalf("failed to reach /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	if body == "" {
		t.Error("expected non-empty /metrics response")
	}

	cancel()
	<-errCh
}

func waitForServer(t *testing.T, port string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server on port %s did not start in time", port)
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/server/...
# Expected: compilation error (server package does not exist yet)
```

- [ ] Write the implementation at `pkg/server/server.go`:

```go
// file: pkg/server/server.go
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/health"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Run starts two HTTP servers: the API server and the admin server.
// The API server serves business routes registered via registerRoutes.
// The admin server serves /healthz, /readyz, /metrics, and /debug/pprof/*.
// Both servers shut down gracefully on context cancellation or SIGINT/SIGTERM.
// If metricsHandler is nil, /metrics returns 404.
// readinessChecks are passed to the readiness handler.
func Run(
	ctx context.Context,
	cfg *config.Config,
	registerRoutes func(mux *http.ServeMux),
	metricsHandler http.Handler,
	readinessChecks ...func() error,
) error {
	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	// --- API Server ---
	apiMux := http.NewServeMux()
	registerRoutes(apiMux)

	// Wrap with middleware chain: logging -> metrics -> tracing -> handler
	var apiHandler http.Handler = apiMux
	apiHandler = otelhttp.NewHandler(apiHandler, cfg.ServiceName+"-api")
	apiHandler = metrics.Middleware(cfg.ServiceName)(apiHandler)
	apiHandler = logging.Middleware(logger)(apiHandler)

	apiServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           apiHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// --- Admin Server ---
	adminMux := http.NewServeMux()
	adminMux.Handle("GET /healthz", health.LivenessHandler())
	adminMux.Handle("GET /readyz", health.ReadinessHandler(readinessChecks...))

	if metricsHandler != nil {
		adminMux.Handle("GET /metrics", metricsHandler)
	}

	// pprof handlers
	adminMux.HandleFunc("GET /debug/pprof/", pprof.Index)
	adminMux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	adminMux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	adminMux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	adminMux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	adminServer := &http.Server{
		Addr:              ":" + cfg.AdminPort,
		Handler:           adminMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// If the parent context does not already handle signals, layer on signal handling
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start servers
	errCh := make(chan error, 2)

	go func() {
		logger.Info("starting API server", slog.String("addr", apiServer.Addr))
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API server: %w", err)
		}
	}()

	go func() {
		logger.Info("starting admin server", slog.String("addr", adminServer.Addr))
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("admin server: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections...")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("API server shutdown error", "error", err)
	}

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("admin server shutdown error", "error", err)
	}

	logger.Info("servers stopped")
	return nil
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v -timeout 30s ./pkg/server/...
# Expected: all 3 tests pass
```

- [ ] Run all package tests to ensure nothing is broken:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./pkg/...
# Expected: all tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add pkg/server/ && git commit -m "feat: add pkg/server with dual-port HTTP server, admin routes, and graceful shutdown"
```

---

## Task 8: Ratings Service (Domain + Ports)

### Task 8.1 -- Create domain and port definitions

- [ ] Write the test first at `services/ratings/internal/core/domain/rating_test.go`:

```go
// file: services/ratings/internal/core/domain/rating_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

func TestNewRating_Valid(t *testing.T) {
	tests := []struct {
		name     string
		productID string
		reviewer string
		stars    int
	}{
		{name: "min stars", productID: "product-1", reviewer: "reviewer-1", stars: 1},
		{name: "max stars", productID: "product-2", reviewer: "reviewer-2", stars: 5},
		{name: "mid stars", productID: "product-3", reviewer: "reviewer-3", stars: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := domain.NewRating(tt.productID, tt.reviewer, tt.stars)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.ID == "" {
				t.Error("expected non-empty ID")
			}
			if r.ProductID != tt.productID {
				t.Errorf("ProductID = %q, want %q", r.ProductID, tt.productID)
			}
			if r.Reviewer != tt.reviewer {
				t.Errorf("Reviewer = %q, want %q", r.Reviewer, tt.reviewer)
			}
			if r.Stars != tt.stars {
				t.Errorf("Stars = %d, want %d", r.Stars, tt.stars)
			}
		})
	}
}

func TestNewRating_InvalidStars(t *testing.T) {
	tests := []struct {
		name  string
		stars int
	}{
		{name: "zero stars", stars: 0},
		{name: "negative stars", stars: -1},
		{name: "six stars", stars: 6},
		{name: "hundred stars", stars: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewRating("product-1", "reviewer-1", tt.stars)
			if err == nil {
				t.Fatal("expected error for invalid stars")
			}
		})
	}
}

func TestNewRating_EmptyProductID(t *testing.T) {
	_, err := domain.NewRating("", "reviewer-1", 5)
	if err == nil {
		t.Fatal("expected error for empty product ID")
	}
}

func TestNewRating_EmptyReviewer(t *testing.T) {
	_, err := domain.NewRating("product-1", "", 5)
	if err == nil {
		t.Fatal("expected error for empty reviewer")
	}
}

func TestProductRatings_Average(t *testing.T) {
	pr := &domain.ProductRatings{
		ProductID: "product-1",
		Ratings: []domain.Rating{
			{ID: "1", ProductID: "product-1", Reviewer: "a", Stars: 4},
			{ID: "2", ProductID: "product-1", Reviewer: "b", Stars: 2},
			{ID: "3", ProductID: "product-1", Reviewer: "c", Stars: 3},
		},
	}

	avg := pr.Average()
	if avg != 3.0 {
		t.Errorf("Average() = %f, want 3.0", avg)
	}
}

func TestProductRatings_Average_Empty(t *testing.T) {
	pr := &domain.ProductRatings{
		ProductID: "product-1",
		Ratings:   []domain.Rating{},
	}

	avg := pr.Average()
	if avg != 0.0 {
		t.Errorf("Average() = %f, want 0.0", avg)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/ratings/internal/core/domain/...
# Expected: compilation error (domain package does not exist yet)
```

- [ ] Write the domain at `services/ratings/internal/core/domain/rating.go`:

```go
// file: services/ratings/internal/core/domain/rating.go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// Rating represents a single star rating given by a reviewer for a product.
type Rating struct {
	ID        string
	ProductID string
	Reviewer  string
	Stars     int
}

// ProductRatings aggregates all ratings for a single product.
type ProductRatings struct {
	ProductID string
	Ratings   []Rating
}

// NewRating creates a new Rating with validation.
// Stars must be between 1 and 5 inclusive.
// ProductID and Reviewer must be non-empty.
func NewRating(productID, reviewer string, stars int) (*Rating, error) {
	if productID == "" {
		return nil, fmt.Errorf("product ID is required")
	}
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required")
	}
	if stars < 1 || stars > 5 {
		return nil, fmt.Errorf("stars must be between 1 and 5, got %d", stars)
	}

	return &Rating{
		ID:        uuid.New().String(),
		ProductID: productID,
		Reviewer:  reviewer,
		Stars:     stars,
	}, nil
}

// Average returns the average star rating. Returns 0 if there are no ratings.
func (pr *ProductRatings) Average() float64 {
	if len(pr.Ratings) == 0 {
		return 0
	}

	total := 0
	for _, r := range pr.Ratings {
		total += r.Stars
	}

	return float64(total) / float64(len(pr.Ratings))
}
```

- [ ] Write the inbound port at `services/ratings/internal/core/port/inbound.go`:

```go
// file: services/ratings/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingService defines the inbound operations for the ratings domain.
type RatingService interface {
	// GetProductRatings returns all ratings for a given product ID.
	GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error)

	// SubmitRating creates and stores a new rating.
	SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*domain.Rating, error)
}
```

- [ ] Write the outbound port at `services/ratings/internal/core/port/outbound.go`:

```go
// file: services/ratings/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository defines the outbound persistence operations for ratings.
type RatingRepository interface {
	// FindByProductID returns all ratings for a given product ID.
	FindByProductID(ctx context.Context, productID string) ([]domain.Rating, error)

	// Save persists a rating.
	Save(ctx context.Context, rating *domain.Rating) error
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/ratings/internal/core/domain/...
# Expected: all 6 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/ratings/internal/core/ && git commit -m "feat(ratings): add domain model, validation, and port interfaces"
```

---

## Task 9: Ratings Service (Service Layer)

### Task 9.1 -- Create service implementation with tests

- [ ] Write the test first at `services/ratings/internal/core/service/rating_service_test.go`:

```go
// file: services/ratings/internal/core/service/rating_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func TestSubmitRating_Success(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	rating, err := svc.SubmitRating(context.Background(), "product-1", "alice", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rating.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rating.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", rating.ProductID, "product-1")
	}
	if rating.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", rating.Reviewer, "alice")
	}
	if rating.Stars != 5 {
		t.Errorf("Stars = %d, want %d", rating.Stars, 5)
	}
}

func TestSubmitRating_ValidationError(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	tests := []struct {
		name      string
		productID string
		reviewer  string
		stars     int
	}{
		{name: "empty product ID", productID: "", reviewer: "alice", stars: 5},
		{name: "empty reviewer", productID: "product-1", reviewer: "", stars: 5},
		{name: "stars too low", productID: "product-1", reviewer: "alice", stars: 0},
		{name: "stars too high", productID: "product-1", reviewer: "alice", stars: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitRating(context.Background(), tt.productID, tt.reviewer, tt.stars)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGetProductRatings_Empty(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	pr, err := svc.GetProductRatings(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.ProductID != "nonexistent" {
		t.Errorf("ProductID = %q, want %q", pr.ProductID, "nonexistent")
	}
	if len(pr.Ratings) != 0 {
		t.Errorf("expected 0 ratings, got %d", len(pr.Ratings))
	}
	if pr.Average() != 0.0 {
		t.Errorf("Average() = %f, want 0.0", pr.Average())
	}
}

func TestGetProductRatings_WithRatings(t *testing.T) {
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)

	_, err := svc.SubmitRating(context.Background(), "product-1", "alice", 4)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 1: %v", err)
	}

	_, err = svc.SubmitRating(context.Background(), "product-1", "bob", 2)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 2: %v", err)
	}

	_, err = svc.SubmitRating(context.Background(), "product-2", "charlie", 5)
	if err != nil {
		t.Fatalf("unexpected error submitting rating 3: %v", err)
	}

	pr, err := svc.GetProductRatings(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pr.Ratings) != 2 {
		t.Fatalf("expected 2 ratings, got %d", len(pr.Ratings))
	}

	avg := pr.Average()
	if avg != 3.0 {
		t.Errorf("Average() = %f, want 3.0", avg)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/ratings/internal/core/service/...
# Expected: compilation error (service and memory packages do not exist yet)
```

- [ ] Write the in-memory repository at `services/ratings/internal/adapter/outbound/memory/rating_repository.go`:

```go
// file: services/ratings/internal/adapter/outbound/memory/rating_repository.go
package memory

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository is an in-memory implementation of port.RatingRepository.
type RatingRepository struct {
	mu      sync.RWMutex
	ratings []domain.Rating
}

// NewRatingRepository creates a new in-memory rating repository.
func NewRatingRepository() *RatingRepository {
	return &RatingRepository{
		ratings: make([]domain.Rating, 0),
	}
}

// FindByProductID returns all ratings for the given product ID.
func (r *RatingRepository) FindByProductID(_ context.Context, productID string) ([]domain.Rating, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Rating
	for _, rating := range r.ratings {
		if rating.ProductID == productID {
			result = append(result, rating)
		}
	}

	return result, nil
}

// Save persists a rating in memory.
func (r *RatingRepository) Save(_ context.Context, rating *domain.Rating) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ratings = append(r.ratings, *rating)
	return nil
}
```

- [ ] Write the service at `services/ratings/internal/core/service/rating_service.go`:

```go
// file: services/ratings/internal/core/service/rating_service.go
package service

import (
	"context"
	"fmt"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
)

// RatingService implements the port.RatingService interface.
type RatingService struct {
	repo port.RatingRepository
}

// NewRatingService creates a new RatingService with the given repository.
func NewRatingService(repo port.RatingRepository) *RatingService {
	return &RatingService{repo: repo}
}

// GetProductRatings returns all ratings aggregated for a product.
func (s *RatingService) GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error) {
	ratings, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding ratings for product %s: %w", productID, err)
	}

	return &domain.ProductRatings{
		ProductID: productID,
		Ratings:   ratings,
	}, nil
}

// SubmitRating creates and persists a new rating.
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*domain.Rating, error) {
	rating, err := domain.NewRating(productID, reviewer, stars)
	if err != nil {
		return nil, fmt.Errorf("creating rating: %w", err)
	}

	if err := s.repo.Save(ctx, rating); err != nil {
		return nil, fmt.Errorf("saving rating: %w", err)
	}

	return rating, nil
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/ratings/internal/core/service/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/ratings/internal/core/service/ services/ratings/internal/adapter/outbound/memory/ && git commit -m "feat(ratings): add service layer and in-memory repository adapter"
```

---

## Task 10: Ratings Service (HTTP Adapter + Entry Point)

### Task 10.1 -- Create DTO, HTTP handler, and entry point

- [ ] Write the DTO at `services/ratings/internal/adapter/inbound/http/dto.go`:

```go
// file: services/ratings/internal/adapter/inbound/http/dto.go
package http

// SubmitRatingRequest is the JSON body for POST /v1/ratings.
type SubmitRatingRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// RatingResponse represents a single rating in API responses.
type RatingResponse struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// ProductRatingsResponse represents the aggregated ratings for a product.
type ProductRatingsResponse struct {
	ProductID string           `json:"product_id"`
	Average   float64          `json:"average"`
	Count     int              `json:"count"`
	Ratings   []RatingResponse `json:"ratings"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] Write the handler test first at `services/ratings/internal/adapter/inbound/http/handler_test.go`:

```go
// file: services/ratings/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestGetProductRatings_Empty(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/ratings/product-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductRatingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
	if body.Count != 0 {
		t.Errorf("Count = %d, want 0", body.Count)
	}
	if body.Average != 0 {
		t.Errorf("Average = %f, want 0", body.Average)
	}
}

func TestSubmitRating_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitRatingRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Stars:     5,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.RatingResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
	if body.Stars != 5 {
		t.Errorf("Stars = %d, want 5", body.Stars)
	}
}

func TestSubmitRating_InvalidStars(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitRatingRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Stars:     6,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitRating_EmptyBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitAndGet_RoundTrip(t *testing.T) {
	mux := setupHandler(t)

	// Submit two ratings for the same product
	for _, reviewer := range []string{"alice", "bob"} {
		reqBody := handler.SubmitRatingRequest{
			ProductID: "product-1",
			Reviewer:  reviewer,
			Stars:     4,
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("submit status = %d, want %d", rec.Code, http.StatusCreated)
		}
	}

	// Get ratings
	req := httptest.NewRequest(http.MethodGet, "/v1/ratings/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductRatingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.Count != 2 {
		t.Errorf("Count = %d, want 2", body.Count)
	}
	if body.Average != 4.0 {
		t.Errorf("Average = %f, want 4.0", body.Average)
	}
	if len(body.Ratings) != 2 {
		t.Errorf("len(Ratings) = %d, want 2", len(body.Ratings))
	}
}

func TestSubmitRating_InvalidJSON(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/ratings/internal/adapter/inbound/http/...
# Expected: compilation error (handler package does not exist yet)
```

- [ ] Write the handler at `services/ratings/internal/adapter/inbound/http/handler.go`:

```go
// file: services/ratings/internal/adapter/inbound/http/handler.go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
)

// Handler holds the HTTP handlers for the ratings service.
type Handler struct {
	svc port.RatingService
}

// NewHandler creates a new HTTP handler with the given rating service.
func NewHandler(svc port.RatingService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the ratings routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/ratings/{id}", h.getProductRatings)
	mux.HandleFunc("POST /v1/ratings", h.submitRating)
}

func (h *Handler) getProductRatings(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	pr, err := h.svc.GetProductRatings(r.Context(), productID)
	if err != nil {
		logger.Error("failed to get product ratings", "error", err, "product_id", productID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	ratings := make([]RatingResponse, 0, len(pr.Ratings))
	for _, rating := range pr.Ratings {
		ratings = append(ratings, RatingResponse{
			ID:        rating.ID,
			ProductID: rating.ProductID,
			Reviewer:  rating.Reviewer,
			Stars:     rating.Stars,
		})
	}

	writeJSON(w, http.StatusOK, ProductRatingsResponse{
		ProductID: pr.ProductID,
		Average:   pr.Average(),
		Count:     len(pr.Ratings),
		Ratings:   ratings,
	})
}

func (h *Handler) submitRating(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	rating, err := h.svc.SubmitRating(r.Context(), req.ProductID, req.Reviewer, req.Stars)
	if err != nil {
		logger.Warn("failed to submit rating", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("rating submitted", "rating_id", rating.ID, "product_id", rating.ProductID)

	writeJSON(w, http.StatusCreated, RatingResponse{
		ID:        rating.ID,
		ProductID: rating.ProductID,
		Reviewer:  rating.Reviewer,
		Stars:     rating.Stars,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/ratings/internal/adapter/inbound/http/...
# Expected: all 6 tests pass
```

- [ ] Write the entry point at `services/ratings/cmd/main.go`:

```go
// file: services/ratings/cmd/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	ratingsSubmitted, _ := meter.Int64Counter(
		"ratings_submitted_total",
		metric.WithDescription("Total number of ratings submitted"),
	)
	_ = ratingsSubmitted // Will be incremented via middleware or service decorator in a future iteration

	// Wire hex arch
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] Verify it compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/ratings/cmd/
```

- [ ] Run all ratings tests:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/ratings/...
# Expected: all tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/ratings/internal/adapter/inbound/ services/ratings/cmd/ && git commit -m "feat(ratings): add HTTP handler, DTOs, and composition root entry point"
```

---

## Task 11: Details Service (Full Hex Arch)

### Task 11.1 -- Create domain and ports

- [ ] Write the domain test at `services/details/internal/core/domain/detail_test.go`:

```go
// file: services/details/internal/core/domain/detail_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

func TestNewDetail_Valid(t *testing.T) {
	d, err := domain.NewDetail(
		"The Art of Go",
		"Jane Doe",
		2024,
		"paperback",
		350,
		"Go Press",
		"English",
		"1234567890",
		"1234567890123",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
	if d.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", d.Author, "Jane Doe")
	}
	if d.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", d.Title, "The Art of Go")
	}
	if d.Year != 2024 {
		t.Errorf("Year = %d, want %d", d.Year, 2024)
	}
	if d.Type != "paperback" {
		t.Errorf("Type = %q, want %q", d.Type, "paperback")
	}
	if d.Pages != 350 {
		t.Errorf("Pages = %d, want %d", d.Pages, 350)
	}
	if d.Publisher != "Go Press" {
		t.Errorf("Publisher = %q, want %q", d.Publisher, "Go Press")
	}
	if d.Language != "English" {
		t.Errorf("Language = %q, want %q", d.Language, "English")
	}
	if d.ISBN10 != "1234567890" {
		t.Errorf("ISBN10 = %q, want %q", d.ISBN10, "1234567890")
	}
	if d.ISBN13 != "1234567890123" {
		t.Errorf("ISBN13 = %q, want %q", d.ISBN13, "1234567890123")
	}
}

func TestNewDetail_EmptyTitle(t *testing.T) {
	_, err := domain.NewDetail("", "Jane Doe", 2024, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestNewDetail_EmptyAuthor(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "", 2024, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for empty author")
	}
}

func TestNewDetail_InvalidYear(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "Jane Doe", 0, "paperback", 350, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for invalid year")
	}
}

func TestNewDetail_InvalidPages(t *testing.T) {
	_, err := domain.NewDetail("The Art of Go", "Jane Doe", 2024, "paperback", 0, "Go Press", "English", "1234567890", "1234567890123")
	if err == nil {
		t.Fatal("expected error for invalid pages")
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/details/internal/core/domain/...
# Expected: compilation error
```

- [ ] Write the domain at `services/details/internal/core/domain/detail.go`:

```go
// file: services/details/internal/core/domain/detail.go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// Detail represents book metadata.
type Detail struct {
	ID        string
	Title     string
	Author    string
	Year      int
	Type      string
	Pages     int
	Publisher string
	Language  string
	ISBN10    string
	ISBN13    string
}

// NewDetail creates a new Detail with validation.
func NewDetail(title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*Detail, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if author == "" {
		return nil, fmt.Errorf("author is required")
	}
	if year <= 0 {
		return nil, fmt.Errorf("year must be positive, got %d", year)
	}
	if pages <= 0 {
		return nil, fmt.Errorf("pages must be positive, got %d", pages)
	}

	return &Detail{
		ID:        uuid.New().String(),
		Title:     title,
		Author:    author,
		Year:      year,
		Type:      bookType,
		Pages:     pages,
		Publisher: publisher,
		Language:  language,
		ISBN10:    isbn10,
		ISBN13:    isbn13,
	}, nil
}
```

- [ ] Write the inbound port at `services/details/internal/core/port/inbound.go`:

```go
// file: services/details/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailService defines the inbound operations for the details domain.
type DetailService interface {
	// GetDetail returns book details by ID.
	GetDetail(ctx context.Context, id string) (*domain.Detail, error)

	// AddDetail creates and stores a new book detail.
	AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*domain.Detail, error)
}
```

- [ ] Write the outbound port at `services/details/internal/core/port/outbound.go`:

```go
// file: services/details/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository defines the outbound persistence operations for details.
type DetailRepository interface {
	// FindByID returns a detail by its ID.
	FindByID(ctx context.Context, id string) (*domain.Detail, error)

	// Save persists a detail.
	Save(ctx context.Context, detail *domain.Detail) error
}
```

- [ ] Run the domain test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/details/internal/core/domain/...
# Expected: all 5 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/details/internal/core/ && git commit -m "feat(details): add domain model, validation, and port interfaces"
```

### Task 11.2 -- Create service layer and in-memory adapter

- [ ] Write the service test at `services/details/internal/core/service/detail_service_test.go`:

```go
// file: services/details/internal/core/service/detail_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

func TestAddDetail_Success(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)

	detail, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.ID == "" {
		t.Error("expected non-empty ID")
	}
	if detail.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", detail.Title, "The Art of Go")
	}
}

func TestAddDetail_ValidationError(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)

	_, err := svc.AddDetail(context.Background(),
		"", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123",
	)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
}

func TestGetDetail_Found(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)

	created, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123",
	)
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	found, err := svc.GetDetail(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error getting: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID = %q, want %q", found.ID, created.ID)
	}
	if found.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", found.Title, "The Art of Go")
	}
}

func TestGetDetail_NotFound(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)

	_, err := svc.GetDetail(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent detail")
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/details/internal/core/service/...
# Expected: compilation error
```

- [ ] Write the in-memory repository at `services/details/internal/adapter/outbound/memory/detail_repository.go`:

```go
// file: services/details/internal/adapter/outbound/memory/detail_repository.go
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository is an in-memory implementation of port.DetailRepository.
type DetailRepository struct {
	mu      sync.RWMutex
	details map[string]domain.Detail
}

// NewDetailRepository creates a new in-memory detail repository.
func NewDetailRepository() *DetailRepository {
	return &DetailRepository{
		details: make(map[string]domain.Detail),
	}
}

// FindByID returns a detail by its ID.
func (r *DetailRepository) FindByID(_ context.Context, id string) (*domain.Detail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	detail, ok := r.details[id]
	if !ok {
		return nil, fmt.Errorf("detail not found: %s", id)
	}

	return &detail, nil
}

// Save persists a detail in memory.
func (r *DetailRepository) Save(_ context.Context, detail *domain.Detail) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.details[detail.ID] = *detail
	return nil
}
```

- [ ] Write the service at `services/details/internal/core/service/detail_service.go`:

```go
// file: services/details/internal/core/service/detail_service.go
package service

import (
	"context"
	"fmt"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
)

// DetailService implements the port.DetailService interface.
type DetailService struct {
	repo port.DetailRepository
}

// NewDetailService creates a new DetailService with the given repository.
func NewDetailService(repo port.DetailRepository) *DetailService {
	return &DetailService{repo: repo}
}

// GetDetail returns a book detail by ID.
func (s *DetailService) GetDetail(ctx context.Context, id string) (*domain.Detail, error) {
	detail, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding detail %s: %w", id, err)
	}
	return detail, nil
}

// AddDetail creates and persists a new book detail.
func (s *DetailService) AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) (*domain.Detail, error) {
	detail, err := domain.NewDetail(title, author, year, bookType, pages, publisher, language, isbn10, isbn13)
	if err != nil {
		return nil, fmt.Errorf("creating detail: %w", err)
	}

	if err := s.repo.Save(ctx, detail); err != nil {
		return nil, fmt.Errorf("saving detail: %w", err)
	}

	return detail, nil
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/details/internal/core/service/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/details/internal/core/service/ services/details/internal/adapter/outbound/memory/ && git commit -m "feat(details): add service layer and in-memory repository adapter"
```

### Task 11.3 -- Create HTTP adapter and entry point

- [ ] Write the DTO at `services/details/internal/adapter/inbound/http/dto.go`:

```go
// file: services/details/internal/adapter/inbound/http/dto.go
package http

// AddDetailRequest is the JSON body for POST /v1/details.
type AddDetailRequest struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// DetailResponse represents book details in API responses.
type DetailResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] Write the handler test at `services/details/internal/adapter/inbound/http/handler_test.go`:

```go
// file: services/details/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestAddDetail_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.AddDetailRequest{
		Title:     "The Art of Go",
		Author:    "Jane Doe",
		Year:      2024,
		Type:      "paperback",
		Pages:     350,
		Publisher: "Go Press",
		Language:  "English",
		ISBN10:    "1234567890",
		ISBN13:    "1234567890123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.DetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.Title != "The Art of Go" {
		t.Errorf("Title = %q, want %q", body.Title, "The Art of Go")
	}
	if body.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", body.Author, "Jane Doe")
	}
}

func TestAddDetail_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAddDetail_MissingTitle(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.AddDetailRequest{
		Author: "Jane Doe",
		Year:   2024,
		Pages:  350,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetDetail_Found(t *testing.T) {
	mux := setupHandler(t)

	// Create a detail first
	reqBody := handler.AddDetailRequest{
		Title:     "Concurrency in Go",
		Author:    "John Smith",
		Year:      2023,
		Type:      "hardcover",
		Pages:     400,
		Publisher: "Tech Books",
		Language:  "English",
		ISBN10:    "0987654321",
		ISBN13:    "0987654321098",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/details", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.DetailResponse
	json.NewDecoder(createRec.Body).Decode(&created)

	// Get the detail
	getReq := httptest.NewRequest(http.MethodGet, "/v1/details/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.DetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID != created.ID {
		t.Errorf("ID = %q, want %q", body.ID, created.ID)
	}
	if body.Title != "Concurrency in Go" {
		t.Errorf("Title = %q, want %q", body.Title, "Concurrency in Go")
	}
}

func TestGetDetail_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/details/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/details/internal/adapter/inbound/http/...
# Expected: compilation error
```

- [ ] Write the handler at `services/details/internal/adapter/inbound/http/handler.go`:

```go
// file: services/details/internal/adapter/inbound/http/handler.go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
)

// Handler holds the HTTP handlers for the details service.
type Handler struct {
	svc port.DetailService
}

// NewHandler creates a new HTTP handler with the given detail service.
func NewHandler(svc port.DetailService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the details routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/details/{id}", h.getDetail)
	mux.HandleFunc("POST /v1/details", h.addDetail)
}

func (h *Handler) getDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.svc.GetDetail(r.Context(), id)
	if err != nil {
		logger.Warn("detail not found", "id", id, "error", err)
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "detail not found"})
		return
	}

	writeJSON(w, http.StatusOK, DetailResponse{
		ID:        detail.ID,
		Title:     detail.Title,
		Author:    detail.Author,
		Year:      detail.Year,
		Type:      detail.Type,
		Pages:     detail.Pages,
		Publisher: detail.Publisher,
		Language:  detail.Language,
		ISBN10:    detail.ISBN10,
		ISBN13:    detail.ISBN13,
	})
}

func (h *Handler) addDetail(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req AddDetailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	detail, err := h.svc.AddDetail(r.Context(),
		req.Title, req.Author, req.Year, req.Type,
		req.Pages, req.Publisher, req.Language, req.ISBN10, req.ISBN13,
	)
	if err != nil {
		logger.Warn("failed to add detail", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("detail added", "detail_id", detail.ID, "title", detail.Title)

	writeJSON(w, http.StatusCreated, DetailResponse{
		ID:        detail.ID,
		Title:     detail.Title,
		Author:    detail.Author,
		Year:      detail.Year,
		Type:      detail.Type,
		Pages:     detail.Pages,
		Publisher: detail.Publisher,
		Language:  detail.Language,
		ISBN10:    detail.ISBN10,
		ISBN13:    detail.ISBN13,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] Write the entry point at `services/details/cmd/main.go`:

```go
// file: services/details/cmd/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	booksAdded, _ := meter.Int64Counter(
		"books_added_total",
		metric.WithDescription("Total number of books added"),
	)
	_ = booksAdded

	// Wire hex arch
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] Run all details tests and verify they pass:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/details/...
# Expected: all tests pass
```

- [ ] Verify it compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/details/cmd/
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/details/internal/adapter/inbound/ services/details/cmd/ && git commit -m "feat(details): add HTTP handler, DTOs, and composition root entry point"
```

---

## Task 12: Reviews Service (Full Hex Arch + Ratings Client)

### Task 12.1 -- Create domain and ports

- [ ] Write the domain test at `services/reviews/internal/core/domain/review_test.go`:

```go
// file: services/reviews/internal/core/domain/review_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func TestNewReview_Valid(t *testing.T) {
	r, err := domain.NewReview("product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.ID == "" {
		t.Error("expected non-empty ID")
	}
	if r.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", r.ProductID, "product-1")
	}
	if r.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", r.Reviewer, "alice")
	}
	if r.Text != "Great book!" {
		t.Errorf("Text = %q, want %q", r.Text, "Great book!")
	}
	if r.Rating != nil {
		t.Error("expected nil Rating on new review")
	}
}

func TestNewReview_EmptyProductID(t *testing.T) {
	_, err := domain.NewReview("", "alice", "Great book!")
	if err == nil {
		t.Fatal("expected error for empty product ID")
	}
}

func TestNewReview_EmptyReviewer(t *testing.T) {
	_, err := domain.NewReview("product-1", "", "Great book!")
	if err == nil {
		t.Fatal("expected error for empty reviewer")
	}
}

func TestNewReview_EmptyText(t *testing.T) {
	_, err := domain.NewReview("product-1", "alice", "")
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/core/domain/...
# Expected: compilation error
```

- [ ] Write the domain at `services/reviews/internal/core/domain/review.go`:

```go
// file: services/reviews/internal/core/domain/review.go
package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// ReviewRating holds the rating data attached to a review.
type ReviewRating struct {
	Stars   int
	Average float64
	Count   int
}

// Review represents a user review for a product.
type Review struct {
	ID        string
	ProductID string
	Reviewer  string
	Text      string
	Rating    *ReviewRating
}

// NewReview creates a new Review with validation.
func NewReview(productID, reviewer, text string) (*Review, error) {
	if productID == "" {
		return nil, fmt.Errorf("product ID is required")
	}
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required")
	}
	if text == "" {
		return nil, fmt.Errorf("review text is required")
	}

	return &Review{
		ID:        uuid.New().String(),
		ProductID: productID,
		Reviewer:  reviewer,
		Text:      text,
	}, nil
}
```

- [ ] Write the inbound port at `services/reviews/internal/core/port/inbound.go`:

```go
// file: services/reviews/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewService defines the inbound operations for the reviews domain.
type ReviewService interface {
	// GetProductReviews returns all reviews for a product, enriched with ratings data.
	GetProductReviews(ctx context.Context, productID string) ([]domain.Review, error)

	// SubmitReview creates and stores a new review.
	SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error)
}
```

- [ ] Write the outbound port at `services/reviews/internal/core/port/outbound.go`:

```go
// file: services/reviews/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository defines the outbound persistence operations for reviews.
type ReviewRepository interface {
	// FindByProductID returns all reviews for a given product ID.
	FindByProductID(ctx context.Context, productID string) ([]domain.Review, error)

	// Save persists a review.
	Save(ctx context.Context, review *domain.Review) error
}

// RatingsClient defines the outbound operations for fetching ratings.
type RatingsClient interface {
	// GetProductRatings returns the aggregated rating info for a product.
	GetProductRatings(ctx context.Context, productID string) (*domain.ReviewRating, error)
}
```

- [ ] Run domain tests and verify they pass:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/reviews/internal/core/domain/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/reviews/internal/core/ && git commit -m "feat(reviews): add domain model, validation, and port interfaces"
```

### Task 12.2 -- Create service layer, in-memory adapter, and ratings client

- [ ] Write the service test at `services/reviews/internal/core/service/review_service_test.go`:

```go
// file: services/reviews/internal/core/service/review_service_test.go
package service_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

// stubRatingsClient returns fixed rating data for testing.
type stubRatingsClient struct {
	rating *domain.ReviewRating
	err    error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.ReviewRating, error) {
	return s.rating, s.err
}

func TestSubmitReview_Success(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	review, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Great book!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if review.ID == "" {
		t.Error("expected non-empty ID")
	}
	if review.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", review.ProductID, "product-1")
	}
	if review.Reviewer != "alice" {
		t.Errorf("Reviewer = %q, want %q", review.Reviewer, "alice")
	}
}

func TestSubmitReview_ValidationError(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{}
	svc := service.NewReviewService(repo, client)

	tests := []struct {
		name      string
		productID string
		reviewer  string
		text      string
	}{
		{name: "empty product ID", productID: "", reviewer: "alice", text: "Great!"},
		{name: "empty reviewer", productID: "product-1", reviewer: "", text: "Great!"},
		{name: "empty text", productID: "product-1", reviewer: "alice", text: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SubmitReview(context.Background(), tt.productID, tt.reviewer, tt.text)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGetProductReviews_Empty(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		rating: &domain.ReviewRating{Stars: 0, Average: 0, Count: 0},
	}
	svc := service.NewReviewService(repo, client)

	reviews, err := svc.GetProductReviews(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		rating: &domain.ReviewRating{Stars: 0, Average: 4.5, Count: 10},
	}
	svc := service.NewReviewService(repo, client)

	_, err := svc.SubmitReview(context.Background(), "product-1", "alice", "Excellent!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.SubmitReview(context.Background(), "product-1", "bob", "Good read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviews, err := svc.GetProductReviews(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reviews) != 2 {
		t.Fatalf("expected 2 reviews, got %d", len(reviews))
	}

	// Each review should have rating data
	for _, review := range reviews {
		if review.Rating == nil {
			t.Error("expected non-nil Rating on review")
			continue
		}
		if review.Rating.Average != 4.5 {
			t.Errorf("Rating.Average = %f, want 4.5", review.Rating.Average)
		}
		if review.Rating.Count != 10 {
			t.Errorf("Rating.Count = %d, want 10", review.Rating.Count)
		}
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/core/service/...
# Expected: compilation error
```

- [ ] Write the in-memory repository at `services/reviews/internal/adapter/outbound/memory/review_repository.go`:

```go
// file: services/reviews/internal/adapter/outbound/memory/review_repository.go
package memory

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository is an in-memory implementation of port.ReviewRepository.
type ReviewRepository struct {
	mu      sync.RWMutex
	reviews []domain.Review
}

// NewReviewRepository creates a new in-memory review repository.
func NewReviewRepository() *ReviewRepository {
	return &ReviewRepository{
		reviews: make([]domain.Review, 0),
	}
}

// FindByProductID returns all reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(_ context.Context, productID string) ([]domain.Review, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Review
	for _, review := range r.reviews {
		result = append(result, domain.Review{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		})
	}

	filtered := make([]domain.Review, 0)
	for _, review := range result {
		if review.ProductID == productID {
			filtered = append(filtered, review)
		}
	}

	return filtered, nil
}

// Save persists a review in memory.
func (r *ReviewRepository) Save(_ context.Context, review *domain.Review) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reviews = append(r.reviews, *review)
	return nil
}
```

- [ ] Write the service at `services/reviews/internal/core/service/review_service.go`:

```go
// file: services/reviews/internal/core/service/review_service.go
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
	}
}

// GetProductReviews returns all reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string) ([]domain.Review, error) {
	reviews, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	// Fetch ratings from the ratings service
	rating, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, nil
	}

	// Enrich each review with the product-level rating
	for i := range reviews {
		reviews[i].Rating = rating
	}

	return reviews, nil
}

// SubmitReview creates and persists a new review.
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text string) (*domain.Review, error) {
	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	if err := s.repo.Save(ctx, review); err != nil {
		return nil, fmt.Errorf("saving review: %w", err)
	}

	return review, nil
}
```

- [ ] Write the HTTP ratings client adapter at `services/reviews/internal/adapter/outbound/http/ratings_client.go`:

```go
// file: services/reviews/internal/adapter/outbound/http/ratings_client.go
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ratingsResponse mirrors the ratings service ProductRatingsResponse.
type ratingsResponse struct {
	ProductID string `json:"product_id"`
	Average   float64 `json:"average"`
	Count     int     `json:"count"`
}

// RatingsClient is an HTTP client that fetches ratings from the ratings service.
type RatingsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRatingsClient creates a new RatingsClient pointing to the given base URL.
func NewRatingsClient(baseURL string) *RatingsClient {
	return &RatingsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetProductRatings fetches the aggregated ratings for a product from the ratings service.
func (c *RatingsClient) GetProductRatings(ctx context.Context, productID string) (*domain.ReviewRating, error) {
	url := fmt.Sprintf("%s/v1/ratings/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching ratings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ratings service returned status %d", resp.StatusCode)
	}

	var body ratingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding ratings response: %w", err)
	}

	return &domain.ReviewRating{
		Average: body.Average,
		Count:   body.Count,
	}, nil
}
```

- [ ] Run the test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/reviews/internal/core/service/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/reviews/internal/core/service/ services/reviews/internal/adapter/outbound/ && git commit -m "feat(reviews): add service layer, in-memory repo, and HTTP ratings client"
```

### Task 12.3 -- Create HTTP adapter and entry point

- [ ] Write the DTO at `services/reviews/internal/adapter/inbound/http/dto.go`:

```go
// file: services/reviews/internal/adapter/inbound/http/dto.go
package http

// SubmitReviewRequest is the JSON body for POST /v1/reviews.
type SubmitReviewRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Text      string `json:"text"`
}

// ReviewRatingResponse represents rating data embedded in a review response.
type ReviewRatingResponse struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review in API responses.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}

// ProductReviewsResponse wraps multiple reviews for a product.
type ProductReviewsResponse struct {
	ProductID string           `json:"product_id"`
	Reviews   []ReviewResponse `json:"reviews"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] Write the handler test at `services/reviews/internal/adapter/inbound/http/handler_test.go`:

```go
// file: services/reviews/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

type stubRatingsClient struct {
	rating *domain.ReviewRating
	err    error
}

func (s *stubRatingsClient) GetProductRatings(_ context.Context, _ string) (*domain.ReviewRating, error) {
	return s.rating, s.err
}

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewReviewRepository()
	client := &stubRatingsClient{
		rating: &domain.ReviewRating{Average: 4.0, Count: 5},
	}
	svc := service.NewReviewService(repo, client)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestSubmitReview_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Great book!",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.ReviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}
}

func TestSubmitReview_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitReview_EmptyText(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetProductReviews_WithRatings(t *testing.T) {
	mux := setupHandler(t)

	// Submit a review
	reqBody := handler.SubmitReviewRequest{
		ProductID: "product-1",
		Reviewer:  "alice",
		Text:      "Loved it!",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/reviews", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	// Get reviews
	getReq := httptest.NewRequest(http.MethodGet, "/v1/reviews/product-1", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.ProductReviewsResponse
	if err := json.NewDecoder(getRec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ProductID != "product-1" {
		t.Errorf("ProductID = %q, want %q", body.ProductID, "product-1")
	}

	if len(body.Reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(body.Reviews))
	}

	review := body.Reviews[0]
	if review.Rating == nil {
		t.Fatal("expected non-nil rating")
	}
	if review.Rating.Average != 4.0 {
		t.Errorf("Rating.Average = %f, want 4.0", review.Rating.Average)
	}
	if review.Rating.Count != 5 {
		t.Errorf("Rating.Count = %d, want 5", review.Rating.Count)
	}
}

func TestGetProductReviews_Empty(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/reviews/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.ProductReviewsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if len(body.Reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(body.Reviews))
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/reviews/internal/adapter/inbound/http/...
# Expected: compilation error
```

- [ ] Write the handler at `services/reviews/internal/adapter/inbound/http/handler.go`:

```go
// file: services/reviews/internal/adapter/inbound/http/handler.go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// Handler holds the HTTP handlers for the reviews service.
type Handler struct {
	svc port.ReviewService
}

// NewHandler creates a new HTTP handler with the given review service.
func NewHandler(svc port.ReviewService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the reviews routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/reviews/{id}", h.getProductReviews)
	mux.HandleFunc("POST /v1/reviews", h.submitReview)
}

func (h *Handler) getProductReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	reviews, err := h.svc.GetProductReviews(r.Context(), productID)
	if err != nil {
		logger.Error("failed to get product reviews", "error", err, "product_id", productID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	reviewResponses := make([]ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		resp := ReviewResponse{
			ID:        review.ID,
			ProductID: review.ProductID,
			Reviewer:  review.Reviewer,
			Text:      review.Text,
		}
		if review.Rating != nil {
			resp.Rating = &ReviewRatingResponse{
				Average: review.Rating.Average,
				Count:   review.Rating.Count,
			}
		}
		reviewResponses = append(reviewResponses, resp)
	}

	writeJSON(w, http.StatusOK, ProductReviewsResponse{
		ProductID: productID,
		Reviews:   reviewResponses,
	})
}

func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	review, err := h.svc.SubmitReview(r.Context(), req.ProductID, req.Reviewer, req.Text)
	if err != nil {
		logger.Warn("failed to submit review", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("review submitted", "review_id", review.ID, "product_id", review.ProductID)

	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] Write the entry point at `services/reviews/cmd/main.go`:

```go
// file: services/reviews/cmd/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	ratingshttp "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	reviewsSubmitted, _ := meter.Int64Counter(
		"reviews_submitted_total",
		metric.WithDescription("Total number of reviews submitted"),
	)
	_ = reviewsSubmitted

	// Wire hex arch
	repo := memory.NewReviewRepository()
	ratingsURL := envOrDefault("RATINGS_SERVICE_URL", "http://localhost:8080")
	ratingsClient := ratingshttp.NewRatingsClient(ratingsURL)
	svc := service.NewReviewService(repo, ratingsClient)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
```

- [ ] Run all reviews tests and verify they pass:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/reviews/...
# Expected: all tests pass
```

- [ ] Verify it compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/reviews/cmd/
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/reviews/internal/adapter/inbound/ services/reviews/cmd/ && git commit -m "feat(reviews): add HTTP handler, DTOs, ratings client, and composition root"
```

---

## Task 13: Notification Service (Full Hex Arch + Log Dispatcher)

### Task 13.1 -- Create domain and ports

- [ ] Write the domain test at `services/notification/internal/core/domain/notification_test.go`:

```go
// file: services/notification/internal/core/domain/notification_test.go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

func TestNewNotification_Valid(t *testing.T) {
	tests := []struct {
		name      string
		recipient string
		channel   domain.Channel
		subject   string
		body      string
	}{
		{name: "email", recipient: "alice@example.com", channel: domain.ChannelEmail, subject: "New Review", body: "A review was posted"},
		{name: "sms", recipient: "+1234567890", channel: domain.ChannelSMS, subject: "New Rating", body: "A rating was posted"},
		{name: "push", recipient: "user-123", channel: domain.ChannelPush, subject: "New Book", body: "A book was added"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := domain.NewNotification(tt.recipient, tt.channel, tt.subject, tt.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if n.ID == "" {
				t.Error("expected non-empty ID")
			}
			if n.Recipient != tt.recipient {
				t.Errorf("Recipient = %q, want %q", n.Recipient, tt.recipient)
			}
			if n.Channel != tt.channel {
				t.Errorf("Channel = %q, want %q", n.Channel, tt.channel)
			}
			if n.Subject != tt.subject {
				t.Errorf("Subject = %q, want %q", n.Subject, tt.subject)
			}
			if n.Body != tt.body {
				t.Errorf("Body = %q, want %q", n.Body, tt.body)
			}
			if n.Status != domain.StatusQueued {
				t.Errorf("Status = %q, want %q", n.Status, domain.StatusQueued)
			}
		})
	}
}

func TestNewNotification_EmptyRecipient(t *testing.T) {
	_, err := domain.NewNotification("", domain.ChannelEmail, "Subject", "Body")
	if err == nil {
		t.Fatal("expected error for empty recipient")
	}
}

func TestNewNotification_EmptySubject(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.ChannelEmail, "", "Body")
	if err == nil {
		t.Fatal("expected error for empty subject")
	}
}

func TestNewNotification_EmptyBody(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "")
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestNewNotification_InvalidChannel(t *testing.T) {
	_, err := domain.NewNotification("alice@example.com", domain.Channel("telegram"), "Subject", "Body")
	if err == nil {
		t.Fatal("expected error for invalid channel")
	}
}

func TestNotification_MarkSent(t *testing.T) {
	n, _ := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "Body")
	n.MarkSent()

	if n.Status != domain.StatusSent {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusSent)
	}
	if n.SentAt.IsZero() {
		t.Error("expected non-zero SentAt")
	}
}

func TestNotification_MarkFailed(t *testing.T) {
	n, _ := domain.NewNotification("alice@example.com", domain.ChannelEmail, "Subject", "Body")
	n.MarkFailed()

	if n.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusFailed)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/notification/internal/core/domain/...
# Expected: compilation error
```

- [ ] Write the domain at `services/notification/internal/core/domain/notification.go`:

```go
// file: services/notification/internal/core/domain/notification.go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Channel represents the notification delivery channel.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

// NotificationStatus represents the delivery status.
type NotificationStatus string

const (
	StatusQueued NotificationStatus = "queued"
	StatusSent   NotificationStatus = "sent"
	StatusFailed NotificationStatus = "failed"
)

// Notification represents a notification to be dispatched.
type Notification struct {
	ID        string
	Recipient string
	Channel   Channel
	Subject   string
	Body      string
	Status    NotificationStatus
	SentAt    time.Time
}

// NewNotification creates a new Notification with validation.
func NewNotification(recipient string, channel Channel, subject, body string) (*Notification, error) {
	if recipient == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}
	if !isValidChannel(channel) {
		return nil, fmt.Errorf("invalid channel: %s", channel)
	}

	return &Notification{
		ID:        uuid.New().String(),
		Recipient: recipient,
		Channel:   channel,
		Subject:   subject,
		Body:      body,
		Status:    StatusQueued,
	}, nil
}

// MarkSent updates the notification status to sent with the current time.
func (n *Notification) MarkSent() {
	n.Status = StatusSent
	n.SentAt = time.Now()
}

// MarkFailed updates the notification status to failed.
func (n *Notification) MarkFailed() {
	n.Status = StatusFailed
}

func isValidChannel(c Channel) bool {
	switch c {
	case ChannelEmail, ChannelSMS, ChannelPush:
		return true
	default:
		return false
	}
}
```

- [ ] Write the inbound port at `services/notification/internal/core/port/inbound.go`:

```go
// file: services/notification/internal/core/port/inbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationService defines the inbound operations for the notification domain.
type NotificationService interface {
	// Dispatch creates and dispatches a notification.
	Dispatch(ctx context.Context, recipient string, channel domain.Channel, subject, body string) (*domain.Notification, error)

	// GetByID returns a notification by its ID.
	GetByID(ctx context.Context, id string) (*domain.Notification, error)

	// GetByRecipient returns all notifications for a given recipient.
	GetByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error)
}
```

- [ ] Write the outbound port at `services/notification/internal/core/port/outbound.go`:

```go
// file: services/notification/internal/core/port/outbound.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository defines the outbound persistence operations for notifications.
type NotificationRepository interface {
	// Save persists a notification.
	Save(ctx context.Context, notification *domain.Notification) error

	// FindByID returns a notification by its ID.
	FindByID(ctx context.Context, id string) (*domain.Notification, error)

	// FindByRecipient returns all notifications for a given recipient.
	FindByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error)
}

// NotificationDispatcher defines the outbound operations for actually sending notifications.
type NotificationDispatcher interface {
	// Send dispatches the notification via the appropriate channel.
	Send(ctx context.Context, notification *domain.Notification) error
}
```

- [ ] Run the domain test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/notification/internal/core/domain/...
# Expected: all 7 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/notification/internal/core/ && git commit -m "feat(notification): add domain model, validation, and port interfaces"
```

### Task 13.2 -- Create service, adapters, and entry point

- [ ] Write the service test at `services/notification/internal/core/service/notification_service_test.go`:

```go
// file: services/notification/internal/core/service/notification_service_test.go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)

func TestDispatch_Success(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)

	n, err := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "New Review", "A review was posted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.Status != domain.StatusSent {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusSent)
	}
}

func TestDispatch_ValidationError(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)

	_, err := svc.Dispatch(context.Background(), "", domain.ChannelEmail, "Subject", "Body")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDispatch_DispatcherFailure(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := &failingDispatcher{}
	svc := service.NewNotificationService(repo, dispatcher)

	n, err := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject", "Body")
	// Dispatch should still succeed but the notification should be marked as failed
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", n.Status, domain.StatusFailed)
	}
}

type failingDispatcher struct{}

func (d *failingDispatcher) Send(_ context.Context, _ *domain.Notification) error {
	return errors.New("send failed")
}

func TestGetByID_Found(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)

	created, _ := svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject", "Body")

	found, err := svc.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID = %q, want %q", found.ID, created.ID)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)

	_, err := svc.GetByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent notification")
	}
}

func TestGetByRecipient(t *testing.T) {
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)

	_, _ = svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelEmail, "Subject 1", "Body 1")
	_, _ = svc.Dispatch(context.Background(), "alice@example.com", domain.ChannelSMS, "Subject 2", "Body 2")
	_, _ = svc.Dispatch(context.Background(), "bob@example.com", domain.ChannelEmail, "Subject 3", "Body 3")

	notifications, err := svc.GetByRecipient(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(notifications))
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/notification/internal/core/service/...
# Expected: compilation error
```

- [ ] Write the in-memory repository at `services/notification/internal/adapter/outbound/memory/notification_repository.go`:

```go
// file: services/notification/internal/adapter/outbound/memory/notification_repository.go
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository is an in-memory implementation of port.NotificationRepository.
type NotificationRepository struct {
	mu            sync.RWMutex
	notifications map[string]domain.Notification
}

// NewNotificationRepository creates a new in-memory notification repository.
func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{
		notifications: make(map[string]domain.Notification),
	}
}

// Save persists a notification in memory.
func (r *NotificationRepository) Save(_ context.Context, notification *domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.notifications[notification.ID] = *notification
	return nil
}

// FindByID returns a notification by its ID.
func (r *NotificationRepository) FindByID(_ context.Context, id string) (*domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notifications[id]
	if !ok {
		return nil, fmt.Errorf("notification not found: %s", id)
	}

	return &n, nil
}

// FindByRecipient returns all notifications for a given recipient.
func (r *NotificationRepository) FindByRecipient(_ context.Context, recipient string) ([]domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Notification
	for _, n := range r.notifications {
		if n.Recipient == recipient {
			result = append(result, n)
		}
	}

	return result, nil
}
```

- [ ] Write the log dispatcher at `services/notification/internal/adapter/outbound/log/dispatcher.go`:

```go
// file: services/notification/internal/adapter/outbound/log/dispatcher.go
package log

import (
	"context"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// Dispatcher is a notification dispatcher that logs instead of actually sending.
type Dispatcher struct{}

// NewDispatcher creates a new log-based notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// Send logs the notification details instead of actually dispatching it.
func (d *Dispatcher) Send(ctx context.Context, notification *domain.Notification) error {
	logger := logging.FromContext(ctx)
	logger.Info("dispatching notification (log mode)",
		slog.String("notification_id", notification.ID),
		slog.String("recipient", notification.Recipient),
		slog.String("channel", string(notification.Channel)),
		slog.String("subject", notification.Subject),
		slog.String("body", notification.Body),
	)
	return nil
}
```

- [ ] Write the service at `services/notification/internal/core/service/notification_service.go`:

```go
// file: services/notification/internal/core/service/notification_service.go
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
)

// NotificationService implements the port.NotificationService interface.
type NotificationService struct {
	repo       port.NotificationRepository
	dispatcher port.NotificationDispatcher
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(repo port.NotificationRepository, dispatcher port.NotificationDispatcher) *NotificationService {
	return &NotificationService{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

// Dispatch creates, dispatches, and persists a notification.
// If the dispatcher fails, the notification is marked as failed but still persisted.
func (s *NotificationService) Dispatch(ctx context.Context, recipient string, channel domain.Channel, subject, body string) (*domain.Notification, error) {
	notification, err := domain.NewNotification(recipient, channel, subject, body)
	if err != nil {
		return nil, fmt.Errorf("creating notification: %w", err)
	}

	if err := s.dispatcher.Send(ctx, notification); err != nil {
		logger := logging.FromContext(ctx)
		logger.Error("failed to dispatch notification",
			slog.String("notification_id", notification.ID),
			slog.String("error", err.Error()),
		)
		notification.MarkFailed()
	} else {
		notification.MarkSent()
	}

	if err := s.repo.Save(ctx, notification); err != nil {
		return nil, fmt.Errorf("saving notification: %w", err)
	}

	return notification, nil
}

// GetByID returns a notification by its ID.
func (s *NotificationService) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding notification %s: %w", id, err)
	}
	return n, nil
}

// GetByRecipient returns all notifications for a given recipient.
func (s *NotificationService) GetByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error) {
	notifications, err := s.repo.FindByRecipient(ctx, recipient)
	if err != nil {
		return nil, fmt.Errorf("finding notifications for %s: %w", recipient, err)
	}
	return notifications, nil
}
```

- [ ] Run the service test and verify it passes:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/notification/internal/core/service/...
# Expected: all 6 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/notification/internal/core/service/ services/notification/internal/adapter/outbound/ && git commit -m "feat(notification): add service layer, in-memory repo, and log dispatcher"
```

### Task 13.3 -- Create HTTP adapter and entry point

- [ ] Write the DTO at `services/notification/internal/adapter/inbound/http/dto.go`:

```go
// file: services/notification/internal/adapter/inbound/http/dto.go
package http

import "time"

// DispatchNotificationRequest is the JSON body for POST /v1/notifications.
type DispatchNotificationRequest struct {
	Recipient string `json:"recipient"`
	Channel   string `json:"channel"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// NotificationResponse represents a notification in API responses.
type NotificationResponse struct {
	ID        string    `json:"id"`
	Recipient string    `json:"recipient"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

// NotificationsListResponse wraps multiple notifications.
type NotificationsListResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
```

- [ ] Write the handler test at `services/notification/internal/adapter/inbound/http/handler_test.go`:

```go
// file: services/notification/internal/adapter/inbound/http/handler_test.go
package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)

func setupHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	repo := memory.NewNotificationRepository()
	dispatcher := log.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)
	mux := http.NewServeMux()
	h := handler.NewHandler(svc)
	h.RegisterRoutes(mux)
	return mux
}

func TestDispatchNotification_Success(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "email",
		Subject:   "New Review",
		Body:      "A review was posted",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body handler.NotificationResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body.ID == "" {
		t.Error("expected non-empty ID")
	}
	if body.Status != "sent" {
		t.Errorf("Status = %q, want %q", body.Status, "sent")
	}
}

func TestDispatchNotification_InvalidBody(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDispatchNotification_InvalidChannel(t *testing.T) {
	mux := setupHandler(t)

	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "telegram",
		Subject:   "Subject",
		Body:      "Body",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetNotificationByID_Found(t *testing.T) {
	mux := setupHandler(t)

	// Create a notification
	reqBody := handler.DispatchNotificationRequest{
		Recipient: "alice@example.com",
		Channel:   "email",
		Subject:   "Subject",
		Body:      "Body",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created handler.NotificationResponse
	json.NewDecoder(createRec.Body).Decode(&created)

	// Get by ID
	getReq := httptest.NewRequest(http.MethodGet, "/v1/notifications/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var body handler.NotificationResponse
	json.NewDecoder(getRec.Body).Decode(&body)

	if body.ID != created.ID {
		t.Errorf("ID = %q, want %q", body.ID, created.ID)
	}
}

func TestGetNotificationByID_NotFound(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetNotificationsByRecipient(t *testing.T) {
	mux := setupHandler(t)

	// Create two notifications for alice
	for i := 0; i < 2; i++ {
		reqBody := handler.DispatchNotificationRequest{
			Recipient: "alice@example.com",
			Channel:   "email",
			Subject:   "Subject",
			Body:      "Body",
		}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/notifications", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	// Query by recipient
	req := httptest.NewRequest(http.MethodGet, "/v1/notifications?recipient=alice@example.com", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body handler.NotificationsListResponse
	json.NewDecoder(rec.Body).Decode(&body)

	if len(body.Notifications) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(body.Notifications))
	}
}

func TestGetNotifications_MissingRecipient(t *testing.T) {
	mux := setupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/notification/internal/adapter/inbound/http/...
# Expected: compilation error
```

- [ ] Write the handler at `services/notification/internal/adapter/inbound/http/handler.go`:

```go
// file: services/notification/internal/adapter/inbound/http/handler.go
package http

import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
)

// Handler holds the HTTP handlers for the notification service.
type Handler struct {
	svc port.NotificationService
}

// NewHandler creates a new HTTP handler with the given notification service.
func NewHandler(svc port.NotificationService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the notification routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/notifications", h.dispatch)
	mux.HandleFunc("GET /v1/notifications/{id}", h.getByID)
	mux.HandleFunc("GET /v1/notifications", h.listByRecipient)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req DispatchNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	notification, err := h.svc.Dispatch(r.Context(), req.Recipient, domain.Channel(req.Channel), req.Subject, req.Body)
	if err != nil {
		logger.Warn("failed to dispatch notification", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("notification dispatched",
		"notification_id", notification.ID,
		"channel", string(notification.Channel),
		"status", string(notification.Status),
	)

	writeJSON(w, http.StatusCreated, NotificationResponse{
		ID:        notification.ID,
		Recipient: notification.Recipient,
		Channel:   string(notification.Channel),
		Subject:   notification.Subject,
		Body:      notification.Body,
		Status:    string(notification.Status),
		SentAt:    notification.SentAt,
	})
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	notification, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		logger.Warn("notification not found", "id", id, "error", err)
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "notification not found"})
		return
	}

	writeJSON(w, http.StatusOK, NotificationResponse{
		ID:        notification.ID,
		Recipient: notification.Recipient,
		Channel:   string(notification.Channel),
		Subject:   notification.Subject,
		Body:      notification.Body,
		Status:    string(notification.Status),
		SentAt:    notification.SentAt,
	})
}

func (h *Handler) listByRecipient(w http.ResponseWriter, r *http.Request) {
	recipient := r.URL.Query().Get("recipient")
	if recipient == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "recipient query parameter is required"})
		return
	}

	logger := logging.FromContext(r.Context())

	notifications, err := h.svc.GetByRecipient(r.Context(), recipient)
	if err != nil {
		logger.Error("failed to get notifications", "error", err, "recipient", recipient)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	responses := make([]NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		responses = append(responses, NotificationResponse{
			ID:        n.ID,
			Recipient: n.Recipient,
			Channel:   string(n.Channel),
			Subject:   n.Subject,
			Body:      n.Body,
			Status:    string(n.Status),
			SentAt:    n.SentAt,
		})
	}

	writeJSON(w, http.StatusOK, NotificationsListResponse{
		Notifications: responses,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] Write the entry point at `services/notification/cmd/main.go`:

```go
// file: services/notification/cmd/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/inbound/http"
	logdispatcher "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metrics
	meter := otel.Meter(cfg.ServiceName)
	notificationsDispatched, _ := meter.Int64Counter(
		"notifications_dispatched_total",
		metric.WithDescription("Total number of notifications dispatched by channel"),
	)
	notificationsFailed, _ := meter.Int64Counter(
		"notifications_failed_total",
		metric.WithDescription("Total number of failed notification dispatches by channel"),
	)
	notificationsByStatus, _ := meter.Int64UpDownCounter(
		"notifications_by_status",
		metric.WithDescription("Current count of notifications by status"),
	)
	_ = notificationsDispatched
	_ = notificationsFailed
	_ = notificationsByStatus

	// Wire hex arch
	repo := memory.NewNotificationRepository()
	dispatcher := logdispatcher.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] Run all notification tests and verify they pass:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/notification/...
# Expected: all tests pass
```

- [ ] Verify it compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/notification/cmd/
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/notification/internal/adapter/inbound/ services/notification/cmd/ && git commit -m "feat(notification): add HTTP handler, DTOs, and composition root entry point"
```

---

## Task 14: Productpage Service (BFF + HTMX)

### Task 14.1 -- Create HTTP clients for backend services

- [ ] Write the details client at `services/productpage/internal/client/details.go`:

```go
// file: services/productpage/internal/client/details.go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DetailResponse represents the details service API response.
type DetailResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// DetailsClient fetches book details from the details service.
type DetailsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewDetailsClient creates a new DetailsClient pointing to the given base URL.
func NewDetailsClient(baseURL string) *DetailsClient {
	return &DetailsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetDetail fetches a book detail by ID.
func (c *DetailsClient) GetDetail(ctx context.Context, id string) (*DetailResponse, error) {
	url := fmt.Sprintf("%s/v1/details/%s", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("detail not found: %s", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("details service returned status %d", resp.StatusCode)
	}

	var body DetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding detail response: %w", err)
	}

	return &body, nil
}
```

- [ ] Write the reviews client at `services/productpage/internal/client/reviews.go`:

```go
// file: services/productpage/internal/client/reviews.go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ReviewRatingResponse represents rating data from the reviews service.
type ReviewRatingResponse struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

// ReviewResponse represents a single review from the reviews service.
type ReviewResponse struct {
	ID        string                `json:"id"`
	ProductID string                `json:"product_id"`
	Reviewer  string                `json:"reviewer"`
	Text      string                `json:"text"`
	Rating    *ReviewRatingResponse `json:"rating,omitempty"`
}

// ProductReviewsResponse represents the reviews service aggregated response.
type ProductReviewsResponse struct {
	ProductID string           `json:"product_id"`
	Reviews   []ReviewResponse `json:"reviews"`
}

// ReviewsClient fetches reviews from the reviews service.
type ReviewsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewReviewsClient creates a new ReviewsClient pointing to the given base URL.
func NewReviewsClient(baseURL string) *ReviewsClient {
	return &ReviewsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetProductReviews fetches all reviews for a product.
func (c *ReviewsClient) GetProductReviews(ctx context.Context, productID string) (*ProductReviewsResponse, error) {
	url := fmt.Sprintf("%s/v1/reviews/%s", c.baseURL, productID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reviews service returned status %d", resp.StatusCode)
	}

	var body ProductReviewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding reviews response: %w", err)
	}

	return &body, nil
}
```

- [ ] Write the ratings client at `services/productpage/internal/client/ratings.go`:

```go
// file: services/productpage/internal/client/ratings.go
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SubmitRatingRequest represents the request body for submitting a rating.
type SubmitRatingRequest struct {
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// RatingResponse represents a single rating from the ratings service.
type RatingResponse struct {
	ID        string `json:"id"`
	ProductID string `json:"product_id"`
	Reviewer  string `json:"reviewer"`
	Stars     int    `json:"stars"`
}

// RatingsClient submits ratings to the ratings service.
type RatingsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRatingsClient creates a new RatingsClient pointing to the given base URL.
func NewRatingsClient(baseURL string) *RatingsClient {
	return &RatingsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SubmitRating submits a new rating to the ratings service.
func (c *RatingsClient) SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*RatingResponse, error) {
	reqBody := SubmitRatingRequest{
		ProductID: productID,
		Reviewer:  reviewer,
		Stars:     stars,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/ratings", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submitting rating: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ratings service returned status %d", resp.StatusCode)
	}

	var body RatingResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding rating response: %w", err)
	}

	return &body, nil
}
```

- [ ] Write the view model at `services/productpage/internal/model/product.go`:

```go
// file: services/productpage/internal/model/product.go
package model

// ProductDetail is the aggregated view model for a product detail page.
type ProductDetail struct {
	ID        string
	Title     string
	Author    string
	Year      int
	Type      string
	Pages     int
	Publisher string
	Language  string
	ISBN10    string
	ISBN13    string
}

// ProductReview is the view model for a single review.
type ProductReview struct {
	ID       string
	Reviewer string
	Text     string
	Average  float64
	Count    int
}

// ProductPage is the full page view model combining detail and reviews.
type ProductPage struct {
	Detail  *ProductDetail
	Reviews []ProductReview
}

// Product is a summary view model for listing.
type Product struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/productpage/internal/client/ services/productpage/internal/model/ && git commit -m "feat(productpage): add HTTP clients for backend services and view models"
```

### Task 14.2 -- Create HTML templates

- [ ] Write the layout template at `services/productpage/templates/layout.html`:

```html
<!-- file: services/productpage/templates/layout.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Bookinfo - Product Page</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: system-ui, -apple-system, sans-serif; line-height: 1.6; color: #333; max-width: 960px; margin: 0 auto; padding: 20px; }
        h1 { margin-bottom: 20px; color: #1a1a1a; }
        h2 { margin: 20px 0 10px; color: #2a2a2a; }
        .product-detail { background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .product-detail dt { font-weight: bold; color: #555; }
        .product-detail dd { margin-bottom: 10px; margin-left: 0; }
        .review { border: 1px solid #e0e0e0; padding: 15px; border-radius: 8px; margin-bottom: 10px; }
        .review .reviewer { font-weight: bold; color: #1a73e8; }
        .review .text { margin-top: 8px; }
        .rating { color: #f4b400; font-size: 1.2em; }
        .rating-form { background: #f0f4ff; padding: 20px; border-radius: 8px; margin-top: 20px; }
        .rating-form label { display: block; margin-bottom: 5px; font-weight: bold; }
        .rating-form input, .rating-form select { padding: 8px; margin-bottom: 10px; border: 1px solid #ccc; border-radius: 4px; width: 100%; }
        .rating-form button { background: #1a73e8; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        .rating-form button:hover { background: #1557b0; }
        .loading { color: #888; font-style: italic; }
        .error { color: #d93025; }
        .success { color: #188038; padding: 10px; background: #e6f4ea; border-radius: 4px; }
        a { color: #1a73e8; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .product-list { list-style: none; }
        .product-list li { padding: 10px; border-bottom: 1px solid #e0e0e0; }
    </style>
</head>
<body>
    {{block "content" .}}{{end}}
</body>
</html>
```

- [ ] Write the product page template at `services/productpage/templates/productpage.html`:

```html
<!-- file: services/productpage/templates/productpage.html -->
{{define "content"}}
<h1>Bookinfo</h1>

<p>Select a product to view its details, or <a href="/">view all products</a>.</p>

{{if .Detail}}
<section>
    <h2>Book Details</h2>
    <div id="details-section"
         hx-get="/partials/details/{{.Detail.ID}}"
         hx-trigger="load"
         hx-swap="innerHTML">
        <p class="loading">Loading details...</p>
    </div>
</section>

<section>
    <h2>Reviews</h2>
    <div id="reviews-section"
         hx-get="/partials/reviews/{{.Detail.ID}}"
         hx-trigger="load"
         hx-swap="innerHTML">
        <p class="loading">Loading reviews...</p>
    </div>
</section>

<section>
    <h2>Submit a Rating</h2>
    <div id="rating-form-section">
        <div class="rating-form">
            <form hx-post="/partials/rating"
                  hx-target="#rating-result"
                  hx-swap="innerHTML">
                <input type="hidden" name="product_id" value="{{.Detail.ID}}">
                <label for="reviewer">Your Name</label>
                <input type="text" id="reviewer" name="reviewer" required placeholder="Enter your name">
                <label for="stars">Rating</label>
                <select id="stars" name="stars" required>
                    <option value="5">5 Stars</option>
                    <option value="4">4 Stars</option>
                    <option value="3">3 Stars</option>
                    <option value="2">2 Stars</option>
                    <option value="1">1 Star</option>
                </select>
                <button type="submit">Submit Rating</button>
            </form>
            <div id="rating-result"></div>
        </div>
    </div>
</section>
{{else}}
<p>No product selected. Please provide a product ID in the URL.</p>
{{end}}
{{end}}
```

- [ ] Write the details partial at `services/productpage/templates/partials/details.html`:

```html
<!-- file: services/productpage/templates/partials/details.html -->
<div class="product-detail">
    <dl>
        <dt>Title</dt>
        <dd>{{.Title}}</dd>
        <dt>Author</dt>
        <dd>{{.Author}}</dd>
        <dt>Year</dt>
        <dd>{{.Year}}</dd>
        <dt>Type</dt>
        <dd>{{.Type}}</dd>
        <dt>Pages</dt>
        <dd>{{.Pages}}</dd>
        <dt>Publisher</dt>
        <dd>{{.Publisher}}</dd>
        <dt>Language</dt>
        <dd>{{.Language}}</dd>
        <dt>ISBN-10</dt>
        <dd>{{.ISBN10}}</dd>
        <dt>ISBN-13</dt>
        <dd>{{.ISBN13}}</dd>
    </dl>
</div>
```

- [ ] Write the reviews partial at `services/productpage/templates/partials/reviews.html`:

```html
<!-- file: services/productpage/templates/partials/reviews.html -->
{{if .}}
{{range .}}
<div class="review">
    <span class="reviewer">{{.Reviewer}}</span>
    {{if gt .Count 0}}
    <span class="rating">
        (avg: {{printf "%.1f" .Average}} / {{.Count}} ratings)
    </span>
    {{end}}
    <p class="text">{{.Text}}</p>
</div>
{{end}}
{{else}}
<p>No reviews yet.</p>
{{end}}
```

- [ ] Write the rating form response partial at `services/productpage/templates/partials/rating-form.html`:

```html
<!-- file: services/productpage/templates/partials/rating-form.html -->
{{if .Success}}
<p class="success">Rating submitted successfully! ({{.Stars}} stars)</p>
{{else}}
<p class="error">Failed to submit rating: {{.Error}}</p>
{{end}}
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/productpage/templates/ && git commit -m "feat(productpage): add HTML templates with HTMX partials"
```

### Task 14.3 -- Create handlers

- [ ] Write the handler test first at `services/productpage/internal/handler/handler_test.go`:

```go
// file: services/productpage/internal/handler/handler_test.go
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/client"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/handler"
)

func setupMockServers(t *testing.T) (detailsURL, reviewsURL, ratingsURL string) {
	t.Helper()

	detailsMux := http.NewServeMux()
	detailsMux.HandleFunc("GET /v1/details/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":        r.PathValue("id"),
			"title":     "Test Book",
			"author":    "Test Author",
			"year":      2024,
			"type":      "paperback",
			"pages":     300,
			"publisher": "Test Press",
			"language":  "English",
			"isbn_10":   "1234567890",
			"isbn_13":   "1234567890123",
		})
	})
	detailsServer := httptest.NewServer(detailsMux)
	t.Cleanup(detailsServer.Close)

	reviewsMux := http.NewServeMux()
	reviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews": []map[string]any{
				{
					"id":         "review-1",
					"product_id": r.PathValue("id"),
					"reviewer":   "alice",
					"text":       "Great book!",
					"rating":     map[string]any{"average": 4.5, "count": 10},
				},
			},
		})
	})
	reviewsServer := httptest.NewServer(reviewsMux)
	t.Cleanup(reviewsServer.Close)

	ratingsMux := http.NewServeMux()
	ratingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":         "rating-1",
			"product_id": "product-1",
			"reviewer":   "bob",
			"stars":      5,
		})
	})
	ratingsServer := httptest.NewServer(ratingsMux)
	t.Cleanup(ratingsServer.Close)

	return detailsServer.URL, reviewsServer.URL, ratingsServer.URL
}

func TestAPIGetProducts(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, "services/productpage/templates")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/products/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	detail, ok := body["detail"].(map[string]any)
	if !ok {
		t.Fatal("expected detail in response")
	}
	if detail["title"] != "Test Book" {
		t.Errorf("title = %v, want Test Book", detail["title"])
	}
}

func TestPartialDetails(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, "services/productpage/templates")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/details/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Test Book") {
		t.Errorf("expected 'Test Book' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "Test Author") {
		t.Errorf("expected 'Test Author' in response, got:\n%s", body)
	}
}

func TestPartialReviews(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, "services/productpage/templates")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/partials/reviews/product-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "alice") {
		t.Errorf("expected 'alice' in response, got:\n%s", body)
	}
	if !strings.Contains(body, "Great book!") {
		t.Errorf("expected 'Great book!' in response, got:\n%s", body)
	}
}

func TestPartialRatingSubmit(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, "services/productpage/templates")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "successfully") {
		t.Errorf("expected success message, got:\n%s", body)
	}
}
```

- [ ] Run the test and verify it fails:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test ./services/productpage/internal/handler/...
# Expected: compilation error
```

- [ ] Write the handler at `services/productpage/internal/handler/handler.go`:

```go
// file: services/productpage/internal/handler/handler.go
package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/client"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/model"
)

// Handler holds the HTTP handlers for the productpage BFF.
type Handler struct {
	detailsClient *client.DetailsClient
	reviewsClient *client.ReviewsClient
	ratingsClient *client.RatingsClient
	templates     *template.Template
	templateDir   string
}

// NewHandler creates a new productpage handler.
func NewHandler(
	detailsClient *client.DetailsClient,
	reviewsClient *client.ReviewsClient,
	ratingsClient *client.RatingsClient,
	templateDir string,
) *Handler {
	tmpl := template.Must(template.ParseGlob(filepath.Join(templateDir, "*.html")))
	template.Must(tmpl.ParseGlob(filepath.Join(templateDir, "partials", "*.html")))

	return &Handler{
		detailsClient: detailsClient,
		reviewsClient: reviewsClient,
		ratingsClient: ratingsClient,
		templates:     tmpl,
		templateDir:   templateDir,
	}
}

// RegisterRoutes registers all productpage routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// HTML page
	mux.HandleFunc("GET /", h.productPage)

	// JSON API
	mux.HandleFunc("GET /v1/products/{id}", h.apiGetProduct)

	// HTMX partials
	mux.HandleFunc("GET /partials/details/{id}", h.partialDetails)
	mux.HandleFunc("GET /partials/reviews/{id}", h.partialReviews)
	mux.HandleFunc("POST /partials/rating", h.partialRatingSubmit)
}

func (h *Handler) productPage(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("id")

	data := struct {
		Detail *model.ProductDetail
	}{}

	if productID != "" {
		detail, err := h.detailsClient.GetDetail(r.Context(), productID)
		if err == nil {
			data.Detail = &model.ProductDetail{
				ID:        detail.ID,
				Title:     detail.Title,
				Author:    detail.Author,
				Year:      detail.Year,
				Type:      detail.Type,
				Pages:     detail.Pages,
				Publisher: detail.Publisher,
				Language:  detail.Language,
				ISBN10:    detail.ISBN10,
				ISBN13:    detail.ISBN13,
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "layout.html", data)
}

func (h *Handler) apiGetProduct(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.detailsClient.GetDetail(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch detail", "product_id", productID, "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
		return
	}

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch reviews", "product_id", productID, "error", err)
		reviews = &client.ProductReviewsResponse{ProductID: productID, Reviews: []client.ReviewResponse{}}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"detail":  detail,
		"reviews": reviews.Reviews,
	})
}

func (h *Handler) partialDetails(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	detail, err := h.detailsClient.GetDetail(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch detail for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<p class="error">Failed to load details.</p>`))
		return
	}

	data := model.ProductDetail{
		ID:        detail.ID,
		Title:     detail.Title,
		Author:    detail.Author,
		Year:      detail.Year,
		Type:      detail.Type,
		Pages:     detail.Pages,
		Publisher: detail.Publisher,
		Language:  detail.Language,
		ISBN10:    detail.ISBN10,
		ISBN13:    detail.ISBN13,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "details.html", data)
}

func (h *Handler) partialReviews(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	logger := logging.FromContext(r.Context())

	reviews, err := h.reviewsClient.GetProductReviews(r.Context(), productID)
	if err != nil {
		logger.Warn("failed to fetch reviews for partial", "product_id", productID, "error", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<p class="error">Failed to load reviews.</p>`))
		return
	}

	var viewModels []model.ProductReview
	for _, review := range reviews.Reviews {
		vm := model.ProductReview{
			ID:       review.ID,
			Reviewer: review.Reviewer,
			Text:     review.Text,
		}
		if review.Rating != nil {
			vm.Average = review.Rating.Average
			vm.Count = review.Rating.Count
		}
		viewModels = append(viewModels, vm)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "reviews.html", viewModels)
}

func (h *Handler) partialRatingSubmit(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid form data",
		})
		return
	}

	productID := r.FormValue("product_id")
	reviewer := r.FormValue("reviewer")
	starsStr := r.FormValue("stars")

	stars, err := strconv.Atoi(starsStr)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid stars value",
		})
		return
	}

	_, err = h.ratingsClient.SubmitRating(r.Context(), productID, reviewer, stars)
	if err != nil {
		logger.Warn("failed to submit rating", "error", err)
		w.WriteHeader(http.StatusOK)
		h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
		"Success": true,
		"Stars":   stars,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] Run the handler test from the project root (templates are relative to project root):

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -v ./services/productpage/internal/handler/...
# Expected: all 4 tests pass
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/productpage/internal/handler/ && git commit -m "feat(productpage): add page, API, and HTMX partial handlers"
```

### Task 14.4 -- Create entry point

- [ ] Write the entry point at `services/productpage/cmd/main.go`:

```go
// file: services/productpage/cmd/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/client"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/handler"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(ctx)

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// HTTP clients for backend services
	detailsURL := envOrDefault("DETAILS_SERVICE_URL", "http://localhost:8081")
	reviewsURL := envOrDefault("REVIEWS_SERVICE_URL", "http://localhost:8082")
	ratingsURL := envOrDefault("RATINGS_SERVICE_URL", "http://localhost:8083")

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	templateDir := envOrDefault("TEMPLATE_DIR", "services/productpage/templates")
	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, templateDir)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
```

- [ ] Verify it compiles:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./services/productpage/cmd/
```

- [ ] Commit:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add services/productpage/cmd/ && git commit -m "feat(productpage): add composition root entry point"
```

---

## Task 15: Integration Verification

### Task 15.1 -- Full build and test

- [ ] Run `go mod tidy` to clean up dependencies:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go mod tidy
```

- [ ] Verify all packages build:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go build ./...
# Expected: no errors
```

- [ ] Run all tests with race detector:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go test -race -count=1 ./...
# Expected: all tests pass, no race conditions
```

- [ ] Run go vet:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && go vet ./...
# Expected: no issues
```

- [ ] Commit final state if go.mod/go.sum changed:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && git add go.mod go.sum && git commit -m "chore: go mod tidy after all services implemented"
```

### Task 15.2 -- Manual smoke test (optional, requires multiple terminals)

- [ ] Start ratings service:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && SERVICE_NAME=ratings HTTP_PORT=8083 ADMIN_PORT=9093 go run ./services/ratings/cmd/
```

- [ ] Start details service:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && SERVICE_NAME=details HTTP_PORT=8081 ADMIN_PORT=9091 go run ./services/details/cmd/
```

- [ ] Start reviews service:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && SERVICE_NAME=reviews HTTP_PORT=8082 ADMIN_PORT=9092 RATINGS_SERVICE_URL=http://localhost:8083 go run ./services/reviews/cmd/
```

- [ ] Start productpage service:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server && SERVICE_NAME=productpage HTTP_PORT=8080 ADMIN_PORT=9090 DETAILS_SERVICE_URL=http://localhost:8081 REVIEWS_SERVICE_URL=http://localhost:8082 RATINGS_SERVICE_URL=http://localhost:8083 go run ./services/productpage/cmd/
```

- [ ] Verify health of all services:

```bash
curl -s http://localhost:9090/healthz | python3 -m json.tool
# Expected: {"status": "ok"}

curl -s http://localhost:9091/healthz | python3 -m json.tool
# Expected: {"status": "ok"}

curl -s http://localhost:9092/healthz | python3 -m json.tool
# Expected: {"status": "ok"}

curl -s http://localhost:9093/healthz | python3 -m json.tool
# Expected: {"status": "ok"}
```

- [ ] Add a book detail:

```bash
curl -s -X POST http://localhost:8081/v1/details \
  -H "Content-Type: application/json" \
  -d '{"title":"The Art of Go","author":"Jane Doe","year":2024,"type":"paperback","pages":350,"publisher":"Go Press","language":"English","isbn_10":"1234567890","isbn_13":"1234567890123"}' | python3 -m json.tool
# Expected: 201 with detail ID
```

- [ ] Submit a rating (use the detail ID from above as product_id):

```bash
curl -s -X POST http://localhost:8083/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"product_id":"<DETAIL_ID>","reviewer":"alice","stars":5}' | python3 -m json.tool
# Expected: 201 with rating
```

- [ ] Submit a review:

```bash
curl -s -X POST http://localhost:8082/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{"product_id":"<DETAIL_ID>","reviewer":"alice","text":"Excellent book!"}' | python3 -m json.tool
# Expected: 201 with review
```

- [ ] Fetch product via BFF:

```bash
curl -s http://localhost:8080/v1/products/<DETAIL_ID> | python3 -m json.tool
# Expected: combined detail + reviews with ratings
```

- [ ] Open browser to http://localhost:8080/?id=DETAIL_ID to verify HTML + HTMX rendering.
