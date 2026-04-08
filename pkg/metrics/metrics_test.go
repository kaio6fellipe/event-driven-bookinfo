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

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
