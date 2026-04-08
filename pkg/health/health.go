// Package health provides HTTP handlers for liveness and readiness probes.
package health

import (
	"encoding/json"
	"net/http"
)

// LivenessHandler returns an HTTP handler that always responds 200 OK with {"status":"ok"}.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

// ReadinessHandler returns an HTTP handler that runs all checks and responds
// 200 OK when ready or 503 Service Unavailable when any check fails.
func ReadinessHandler(checks ...func() error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, check := range checks {
			if err := check(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
}
