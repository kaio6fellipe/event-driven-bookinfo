// Package main is the entry point for the reviews service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	ratingshttp "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
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

	// Wire hex arch — select adapter based on storage backend
	var repo port.ReviewRepository
	var readinessChecks []func() error

	switch cfg.StorageBackend {
	case "postgres":
		pool, err := database.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			logger.Error("failed to create database pool", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		if cfg.RunMigrations {
			if err := database.RunMigrations(cfg.DatabaseURL, migrations.FS); err != nil {
				logger.Error("failed to run migrations", "error", err)
				os.Exit(1)
			}
			logger.Info("database migrations applied")
		}

		repo = postgres.NewReviewRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewReviewRepository()
		logger.Info("using memory storage backend")
	}

	ratingsURL := envOrDefault("RATINGS_SERVICE_URL", "http://localhost:8080")
	ratingsClient := ratingshttp.NewRatingsClient(ratingsURL)
	svc := service.NewReviewService(repo, ratingsClient)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
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
