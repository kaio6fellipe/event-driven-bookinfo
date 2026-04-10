// Package main is the entry point for the productpage service.
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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/productpage/internal/pending"
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
	defer func() { _ = shutdown(ctx) }()

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

	// Pending review store (Redis or no-op)
	var pendingStore pending.Store
	if cfg.RedisURL != "" {
		rs, err := pending.NewRedisStore(cfg.RedisURL)
		if err != nil {
			logger.Error("failed to create redis pending store", "error", err)
			os.Exit(1)
		}
		if err := rs.Ping(ctx); err != nil {
			logger.Error("failed to connect to redis", "error", err)
			os.Exit(1)
		}
		defer func() { _ = rs.Close() }()
		logger.Info("pending review store enabled", "redis_url", cfg.RedisURL)
		pendingStore = rs
	} else {
		logger.Info("pending review store disabled (REDIS_URL not set)")
		pendingStore = pending.NoopStore{}
	}

	templateDir := envOrDefault("TEMPLATE_DIR", "services/productpage/templates")
	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pendingStore, templateDir)

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
