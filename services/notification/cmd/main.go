// Package main is the entry point for the notification service.
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
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/inbound/http"
	logdispatcher "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/migrations"
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

	// Business metrics
	meter := otel.Meter(cfg.ServiceName)
	notificationsDispatched, _ := meter.Int64Counter(
		"notifications_dispatched_total",
		metric.WithDescription("Total number of notifications dispatched by channel"),
	)
	notificationsFailed, _ := meter.Int64Counter(
		"notifications_failed_total",
		metric.WithDescription("Total number of failed notification dispatches by channel"),
	)
	notificationsByStatus, _ := meter.Int64UpDownCounter(
		"notifications_by_status",
		metric.WithDescription("Current count of notifications by status"),
	)
	_ = notificationsDispatched
	_ = notificationsFailed
	_ = notificationsByStatus

	// Wire hex arch — select adapter based on storage backend
	var repo port.NotificationRepository
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

		repo = postgres.NewNotificationRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewNotificationRepository()
		logger.Info("using memory storage backend")
	}

	dispatcher := logdispatcher.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
