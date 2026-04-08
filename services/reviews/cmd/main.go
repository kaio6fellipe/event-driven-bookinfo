// Package main is the entry point for the reviews service.
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
