// Package metrics provides OTel-based Prometheus metrics setup and HTTP middleware.
package metrics //nolint:revive // package name is intentional

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
