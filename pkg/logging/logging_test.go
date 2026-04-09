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

	oteltrace "go.opentelemetry.io/otel/trace"

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

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestMiddleware_InjectsTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	traceID, _ := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanCtx)
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	foundTrace := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["trace_id"] == "4bf92f3577b34da6a3ce929d0e0e4736" &&
			entry["span_id"] == "00f067aa0ba902b7" {
			foundTrace = true
			break
		}
	}

	if !foundTrace {
		t.Errorf("expected trace_id and span_id in log output, got:\n%s", output)
	}
}

func TestMiddleware_NoTraceContext_OmitsFields(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if _, ok := entry["trace_id"]; ok {
			t.Errorf("did not expect trace_id when no span context, got:\n%s", output)
		}
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
