// Package server provides a dual HTTP server (API + admin) with graceful shutdown.
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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/health"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
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

	// Wrap with middleware chain: tracing -> logging -> metrics -> handler
	var apiHandler http.Handler = apiMux
	apiHandler = metrics.Middleware(cfg.ServiceName)(apiHandler)
	apiHandler = logging.Middleware(logger)(apiHandler)
	apiHandler = otelhttp.NewHandler(apiHandler, cfg.ServiceName+"-api")

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
