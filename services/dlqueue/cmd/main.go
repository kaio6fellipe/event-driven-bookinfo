// Package main is the entry point for the dlqueue service.
package main

import (
	"context"
	"log/slog"
	stdhttp "net/http"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/inbound/http"
	replayhttp "github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/service"
	dlqmetrics "github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/migrations"
)

const defaultMaxRetries = 3

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

	dlqMetrics, err := dlqmetrics.New(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to create dlq metrics", "error", err)
		os.Exit(1)
	}

	maxRetries := defaultMaxRetries
	if v := os.Getenv("MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxRetries = n
		}
	}

	var repo port.DLQRepository
	var pool *pgxpool.Pool
	var readinessChecks []func() error

	switch cfg.StorageBackend {
	case "postgres":
		pool, err = database.NewPool(ctx, cfg.DatabaseURL)
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

		repo = postgres.NewDLQRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewDLQRepository()
		logger.Info("using memory storage backend")
	}

	replayClient := replayhttp.NewReplayClient(nil)
	svc := service.NewDLQService(repo, replayClient, maxRetries, dlqMetrics)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *stdhttp.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
