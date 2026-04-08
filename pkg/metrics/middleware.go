// Package metrics provides OTel-based Prometheus metrics setup and HTTP middleware.
package metrics //nolint:revive // package name is intentional; does not shadow stdlib

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
