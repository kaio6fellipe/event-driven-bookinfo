// Package main is the entry point for the ingestion service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/inbound/http"
	messagingadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/messaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/openlibrary"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/service"
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
	scrapesTotal, _ := meter.Int64Counter(
		"ingestion_scrapes_total",
		metric.WithDescription("Total number of completed scrape cycles"),
	)
	booksPublished, _ := meter.Int64Counter(
		"ingestion_books_published_total",
		metric.WithDescription("Total number of events published to Kafka"),
	)
	errorsTotal, _ := meter.Int64Counter(
		"ingestion_errors_total",
		metric.WithDescription("Total number of publish failures"),
	)
	_, _ = scrapesTotal, booksPublished // Will be used in future metric decorator
	_ = errorsTotal

	// Wire hex arch: fetcher + Kafka producer
	outboundClient := &http.Client{Timeout: 30 * time.Second}
	fetcher := openlibrary.NewClient(outboundClient)

	backend := os.Getenv("EVENT_BACKEND")
	var publisher *messagingadapter.Producer
	switch backend {
	case "kafka", "":
		if cfg.KafkaBrokers == "" {
			logger.Error("KAFKA_BROKERS is required for ingestion service")
			os.Exit(1)
		}
		kPub, err := kafkapub.NewProducer(ctx, cfg.KafkaBrokers, cfg.KafkaTopic)
		if err != nil {
			logger.Error("failed to create Kafka producer", "error", err)
			os.Exit(1)
		}
		publisher = messagingadapter.NewProducer(kPub)
	case "jetstream":
		logger.Error("EVENT_BACKEND=jetstream not yet wired (phase 2)")
		os.Exit(1)
	default:
		logger.Error("unknown EVENT_BACKEND", "value", backend)
		os.Exit(1)
	}
	defer publisher.Close()

	svc := service.NewIngestionService(fetcher, publisher, cfg.SearchQueries, cfg.MaxResultsPerQuery)
	h := handler.NewHandler(svc)

	// Start background poll loop with cancellable context.
	// server.Run blocks until shutdown completes, then cancel stops the poll loop.
	pollCtx, pollCancel := context.WithCancel(context.Background())
	go pollLoop(pollCtx, logger, svc, cfg.PollInterval)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler); err != nil {
		logger.Error("server error", "error", err)
		pollCancel()
		os.Exit(1)
	}
	pollCancel()
}

func pollLoop(ctx context.Context, logger *slog.Logger, svc *service.IngestionService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("ingestion poll loop started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			logger.Info("ingestion poll loop stopped")
			return
		case <-ticker.C:
			logger.Info("poll loop: starting ingestion cycle")
			if _, err := svc.TriggerScrape(ctx, nil); err != nil {
				logger.Error("poll loop: ingestion cycle failed", "error", err)
			}
		}
	}
}
