// Package main is the entry point for the ratings service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/natspub"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	messagingadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/migrations"
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
	ratingsSubmitted, _ := meter.Int64Counter(
		"ratings_submitted_total",
		metric.WithDescription("Total number of ratings submitted"),
	)
	_ = ratingsSubmitted // Will be incremented via middleware or service decorator in a future iteration

	// Wire hex arch — select adapter based on storage backend
	var repo port.RatingRepository
	var pool *pgxpool.Pool
	var readinessChecks []func() error

	switch cfg.StorageBackend {
	case "postgres":
		var err error
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

		repo = postgres.NewRatingRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewRatingRepository()
		logger.Info("using memory storage backend")
	}

	var idemStore idempotency.Store
	if pool != nil {
		idemStore = idempotency.NewPostgresStore(pool)
	} else {
		idemStore = idempotency.NewMemoryStore()
	}

	publisher, closePublisher := buildPublisher(ctx, cfg, logger)
	defer closePublisher()

	svc := service.NewRatingService(repo, idemStore, publisher)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// buildPublisher selects a publisher based on EVENT_BACKEND. It returns
// the publisher and a cleanup function to release underlying resources.
func buildPublisher(ctx context.Context, cfg *config.Config, logger *slog.Logger) (port.EventPublisher, func()) {
	backend := os.Getenv("EVENT_BACKEND")
	switch backend {
	case "kafka", "":
		if cfg.KafkaBrokers == "" {
			logger.Info("kafka publisher disabled — using no-op")
			return messagingadapter.NewNoopPublisher(), func() {}
		}
		d := events.Find(messagingadapter.Exposed, "rating-submitted")
		kPub, err := kafkapub.NewProducer(ctx, cfg.KafkaBrokers, d.Topic)
		if err != nil {
			logger.Error("failed to create Kafka producer", "error", err)
			os.Exit(1)
		}
		kProd := messagingadapter.NewProducer(kPub)
		logger.Info("kafka publisher enabled", "topic", d.Topic)
		return kProd, kProd.Close
	case "jetstream":
		// No no-op fallback for jetstream: it is k8s-only; missing NATS_URL is a config error, not a degraded mode.
		natsURL := os.Getenv("NATS_URL")
		if natsURL == "" {
			logger.Error("NATS_URL must be set when EVENT_BACKEND=jetstream")
			os.Exit(1)
		}
		token := os.Getenv("NATS_TOKEN")
		d := events.Find(messagingadapter.Exposed, "rating-submitted")
		np, err := natspub.NewProducer(ctx, natsURL, token, d.Topic, d.Topic)
		if err != nil {
			logger.Error("failed to create NATS producer", "error", err)
			os.Exit(1)
		}
		nProd := messagingadapter.NewProducer(np)
		logger.Info("jetstream publisher enabled", "topic", d.Topic)
		return nProd, nProd.Close
	default:
		logger.Error("unknown EVENT_BACKEND", "value", backend)
		os.Exit(1)
	}
	return nil, func() {} // unreachable
}
