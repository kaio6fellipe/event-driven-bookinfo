# Ingestion Kafka Producer & Event-Driven Consumption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor ingestion from HTTP Gateway publisher to native Kafka producer, and extend the Helm chart with `events.exposed` / `events.consumed` for Kafka-based event pipelines independent of CQRS.

**Architecture:** Ingestion produces CloudEvents directly to a dedicated Kafka topic via franz-go. The Helm chart gains two new abstractions: `events.exposed` (creates Kafka EventSources) and `events.consumed` (creates Consumer Sensors). These coexist with the existing CQRS webhook pattern without modification.

**Tech Stack:** franz-go (Kafka client), cloudevents/sdk-go (CloudEvents), Argo Events Kafka EventSource, Helm templates

---

## File Map

### New Files
- `services/ingestion/internal/adapter/outbound/kafka/producer.go` — Kafka producer adapter
- `services/ingestion/internal/adapter/outbound/kafka/producer_test.go` — Kafka producer unit tests
- `charts/bookinfo-service/templates/kafka-eventsource.yaml` — Kafka EventSource template
- `charts/bookinfo-service/templates/consumer-sensor.yaml` — Consumer Sensor template
- `charts/bookinfo-service/ci/values-ingestion-kafka.yaml` — CI test: exposed events
- `charts/bookinfo-service/ci/values-details-consumer.yaml` — CI test: consumed events

### Modified Files
- `pkg/config/config.go` — add KafkaBrokers + KafkaTopic fields
- `services/ingestion/cmd/main.go` — wire Kafka producer instead of gateway publisher
- `services/ingestion/internal/core/port/outbound.go` — update EventPublisher doc comment
- `charts/bookinfo-service/values.yaml` — add `events` defaults
- `charts/bookinfo-service/values.schema.json` — add `events` schema
- `charts/bookinfo-service/templates/configmap.yaml` — inject KAFKA_BROKERS
- `charts/bookinfo-service/templates/_helpers.tpl` — add consumer sensor name helper
- `deploy/ingestion/values-local.yaml` — replace GATEWAY_URL with events.exposed
- `deploy/details/values-local.yaml` — add events.consumed
- `go.mod` / `go.sum` — new dependencies

### Removed Files
- `services/ingestion/internal/adapter/outbound/gateway/publisher.go` — replaced by Kafka adapter

---

### Task 1: Add Kafka and CloudEvents Dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add franz-go and cloudevents dependencies**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
go get github.com/twmb/franz-go/pkg/kgo@latest
go get github.com/twmb/franz-go/pkg/kadm@latest
go get github.com/cloudevents/sdk-go/v2@latest
go get github.com/google/uuid@latest
```

Note: `github.com/google/uuid` is already in go.mod but verify it's available.

- [ ] **Step 2: Tidy modules**

```bash
go mod tidy
```

- [ ] **Step 3: Verify dependencies resolve**

```bash
go build ./...
```

Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "build(deps): add franz-go and cloudevents-sdk-go for Kafka producer"
```

---

### Task 2: Add KafkaBrokers and KafkaTopic to Config

**Files:**
- Modify: `pkg/config/config.go:12-30`
- Test: existing tests still pass

- [ ] **Step 1: Add KafkaBrokers and KafkaTopic fields to Config struct**

In `pkg/config/config.go`, add two fields to the `Config` struct after the ingestion-specific fields:

```go
// Kafka producer configuration (used by ingestion service)
KafkaBrokers string
KafkaTopic   string
```

- [ ] **Step 2: Load the new fields in the Load function**

In the `Load()` function, add after `MaxResultsPerQuery`:

```go
KafkaBrokers: os.Getenv("KAFKA_BROKERS"),
KafkaTopic:   envOrDefault("KAFKA_TOPIC", "raw_books_details"),
```

- [ ] **Step 3: Run tests**

```bash
go test ./pkg/config/...
```

Expected: PASS (no existing tests break — new fields are optional).

- [ ] **Step 4: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(pkg/config): add KafkaBrokers and KafkaTopic fields"
```

---

### Task 3: Write Kafka Producer Adapter Tests

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the test file**

```go
package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// fakeClient captures records produced via ProduceSync.
type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	var results kgo.ProduceResults
	for _, r := range rs {
		f.records = append(f.records, r)
		results = append(results, kgo.ProduceResult{Record: r})
	}
	return results
}

func (f *fakeClient) Close() {}

func TestPublishBookAdded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		book    domain.Book
		wantKey string
	}{
		{
			name: "valid book with ISBN-13",
			book: domain.Book{
				Title:       "The Go Programming Language",
				Authors:     []string{"Alan Donovan", "Brian Kernighan"},
				ISBN:        "9780134190440",
				PublishYear: 2015,
				Pages:       380,
				Publisher:   "Addison-Wesley",
				Language:    "en",
			},
			wantKey: "9780134190440",
		},
		{
			name: "valid book with ISBN-10",
			book: domain.Book{
				Title:       "Learning Go",
				Authors:     []string{"Jon Bodner"},
				ISBN:        "1492077216",
				PublishYear: 2021,
				Pages:       375,
				Publisher:   "O'Reilly",
				Language:    "en",
			},
			wantKey: "1492077216",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := &fakeClient{}
			p := kafkaadapter.NewProducerWithClient(fc, "raw_books_details")

			err := p.PublishBookAdded(context.Background(), tt.book)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fc.mu.Lock()
			defer fc.mu.Unlock()

			if len(fc.records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(fc.records))
			}

			r := fc.records[0]

			// Verify topic
			if r.Topic != "raw_books_details" {
				t.Errorf("topic = %q, want %q", r.Topic, "raw_books_details")
			}

			// Verify key is ISBN
			if string(r.Key) != tt.wantKey {
				t.Errorf("key = %q, want %q", string(r.Key), tt.wantKey)
			}

			// Verify CloudEvents headers
			headers := make(map[string]string)
			for _, h := range r.Headers {
				headers[h.Key] = string(h.Value)
			}

			if headers["ce_type"] != "com.bookinfo.ingestion.book-added" {
				t.Errorf("ce_type = %q, want %q", headers["ce_type"], "com.bookinfo.ingestion.book-added")
			}
			if headers["ce_source"] != "ingestion" {
				t.Errorf("ce_source = %q, want %q", headers["ce_source"], "ingestion")
			}
			if headers["ce_specversion"] != "1.0" {
				t.Errorf("ce_specversion = %q, want %q", headers["ce_specversion"], "1.0")
			}
			if headers["ce_subject"] != tt.wantKey {
				t.Errorf("ce_subject = %q, want %q", headers["ce_subject"], tt.wantKey)
			}
			if headers["content-type"] != "application/json" {
				t.Errorf("content-type = %q, want %q", headers["content-type"], "application/json")
			}
			if headers["ce_id"] == "" {
				t.Error("ce_id header is empty, expected a UUID")
			}
			if headers["ce_time"] == "" {
				t.Error("ce_time header is empty, expected RFC3339 timestamp")
			}

			// Verify body is valid JSON with expected fields
			var body map[string]interface{}
			if err := json.Unmarshal(r.Value, &body); err != nil {
				t.Fatalf("failed to unmarshal record value: %v", err)
			}
			if body["title"] != tt.book.Title {
				t.Errorf("body title = %q, want %q", body["title"], tt.book.Title)
			}
			if body["isbn"] != tt.book.ISBN {
				t.Errorf("body isbn = %q, want %q", body["isbn"], tt.book.ISBN)
			}
			if body["idempotency_key"] == nil {
				t.Error("body missing idempotency_key")
			}
		})
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	fc := &errorClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "raw_books_details")

	book := domain.Book{
		Title:       "Test Book",
		Authors:     []string{"Author"},
		ISBN:        "1234567890",
		PublishYear: 2024,
	}

	err := p.PublishBookAdded(context.Background(), book)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// errorClient simulates a produce failure.
type errorClient struct{}

func (e *errorClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	var results kgo.ProduceResults
	for _, r := range rs {
		results = append(results, kgo.ProduceResult{
			Record: r,
			Err:    context.DeadlineExceeded,
		})
	}
	return results
}

func (e *errorClient) Close() {}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./services/ingestion/internal/adapter/outbound/kafka/... -v
```

Expected: FAIL — `kafkaadapter.NewProducerWithClient` does not exist yet.

- [ ] **Step 3: Commit the test file**

```bash
git add services/ingestion/internal/adapter/outbound/kafka/producer_test.go
git commit -m "test(ingestion): add Kafka producer adapter tests"
```

---

### Task 4: Implement Kafka Producer Adapter

**Files:**
- Create: `services/ingestion/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Write the Kafka producer adapter**

```go
// Package kafka implements the EventPublisher port using a native Kafka producer.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

const (
	ceType    = "com.bookinfo.ingestion.book-added"
	ceSource  = "ingestion"
	ceVersion = "1.0"

	defaultPartitions       = 3
	defaultReplicationFactor = 1
)

// bookEvent is the CloudEvents data payload published to Kafka.
type bookEvent struct {
	Title          string   `json:"title"`
	Authors        []string `json:"authors"`
	ISBN           string   `json:"isbn"`
	PublishYear    int      `json:"publish_year"`
	Subjects       []string `json:"subjects,omitempty"`
	Pages          int      `json:"pages,omitempty"`
	Publisher      string   `json:"publisher,omitempty"`
	Language       string   `json:"language,omitempty"`
	ISBN10         string   `json:"isbn_10,omitempty"`
	ISBN13         string   `json:"isbn_13,omitempty"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// KafkaClient abstracts the franz-go client for testing.
type KafkaClient interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer implements port.EventPublisher by producing to Kafka.
type Producer struct {
	client KafkaClient
	topic  string
}

// NewProducer creates a real Kafka producer that connects to the given brokers.
// It auto-creates the topic if it does not exist.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	seeds := strings.Split(brokers, ",")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Kafka client: %w", err)
	}

	if err := ensureTopic(ctx, client, topic); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring topic %q: %w", topic, err)
	}

	return &Producer{client: client, topic: topic}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for testing).
func NewProducerWithClient(client KafkaClient, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishBookAdded sends a book-added CloudEvent to Kafka.
func (p *Producer) PublishBookAdded(ctx context.Context, book domain.Book) error {
	logger := logging.FromContext(ctx)

	isbn10, isbn13 := classifyISBN(book.ISBN)

	evt := bookEvent{
		Title:          book.Title,
		Authors:        book.Authors,
		ISBN:           book.ISBN,
		PublishYear:    book.PublishYear,
		Subjects:       book.Subjects,
		Pages:          book.Pages,
		Publisher:      book.Publisher,
		Language:       book.Language,
		ISBN10:         isbn10,
		ISBN13:         isbn13,
		IdempotencyKey: fmt.Sprintf("ingestion-isbn-%s", book.ISBN),
	}

	value, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshaling book event: %w", err)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(book.ISBN),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceType)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(book.ISBN)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published book-added event to Kafka", "topic", p.topic, "isbn", book.ISBN)
	return nil
}

// Close flushes pending messages and closes the Kafka client.
func (p *Producer) Close() {
	p.client.Close()
}

func ensureTopic(ctx context.Context, client *kgo.Client, topic string) error {
	admin := kadm.NewClient(client)

	resp, err := admin.CreateTopics(ctx, int32(defaultPartitions), int16(defaultReplicationFactor), nil, topic)
	if err != nil {
		return fmt.Errorf("creating topic: %w", err)
	}

	for _, t := range resp.Sorted() {
		if t.Err != nil && t.ErrorMessage != "" && !isTopicExistsError(t.ErrorMessage) {
			return fmt.Errorf("topic %q: %s", t.Topic, t.ErrorMessage)
		}
	}

	return nil
}

func isTopicExistsError(msg string) bool {
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "TOPIC_ALREADY_EXISTS")
}

func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
```

- [ ] **Step 2: Run the tests to verify they pass**

```bash
go test ./services/ingestion/internal/adapter/outbound/kafka/... -v -race
```

Expected: all 3 test cases PASS.

- [ ] **Step 3: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/kafka/producer.go
git commit -m "feat(ingestion): add Kafka producer adapter with CloudEvents and topic auto-creation"
```

---

### Task 5: Update EventPublisher Port Comment and Wire Kafka in main.go

**Files:**
- Modify: `services/ingestion/internal/core/port/outbound.go:15-21`
- Modify: `services/ingestion/cmd/main.go`
- Remove: `services/ingestion/internal/adapter/outbound/gateway/publisher.go`

- [ ] **Step 1: Update the EventPublisher doc comment**

In `services/ingestion/internal/core/port/outbound.go`, replace the comment block:

```go
// EventPublisher sends events to Kafka.
// Returns nil when the event is successfully produced to the topic.
// Returns error on produce failures.
type EventPublisher interface {
	// PublishBookAdded sends a book-added event to Kafka.
	PublishBookAdded(ctx context.Context, book domain.Book) error
}
```

- [ ] **Step 2: Rewire cmd/main.go to use Kafka producer**

Replace the entire `services/ingestion/cmd/main.go` with:

```go
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
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/inbound/http"
	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/kafka"
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

	publisher, err := kafkaadapter.NewProducer(ctx, cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
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
```

Key changes from original:
- Removed `otelhttp` import and OTel-instrumented HTTP transport (no longer needed for gateway calls; openlibrary client keeps its own `http.Client`)
- Replaced `gateway.NewPublisher` with `kafkaadapter.NewProducer`
- Added `defer publisher.Close()` for graceful Kafka shutdown
- Removed `GATEWAY_URL` reference

- [ ] **Step 3: Delete the gateway publisher adapter**

```bash
rm services/ingestion/internal/adapter/outbound/gateway/publisher.go
rmdir services/ingestion/internal/adapter/outbound/gateway
```

- [ ] **Step 4: Verify the project builds**

```bash
go build ./services/ingestion/...
```

Expected: clean build.

- [ ] **Step 5: Run all ingestion tests**

```bash
go test ./services/ingestion/... -v -race
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A services/ingestion/ pkg/config/config.go
git commit -m "feat(ingestion): replace gateway HTTP publisher with Kafka producer

Swap the outbound adapter from HTTP POST to Gateway to native Kafka
producer using franz-go. Topic auto-created on startup. CloudEvents
envelope with structured headers. Gateway adapter removed."
```

---

### Task 6: Add events Defaults to Helm Chart values.yaml

**Files:**
- Modify: `charts/bookinfo-service/values.yaml:144-169`

- [ ] **Step 1: Add events section before postgresql in values.yaml**

Insert the following block before the `# -- PostgreSQL (Bitnami subchart)` line (after `routes: []`):

```yaml
# -- Kafka event pipeline (independent of CQRS)
events:
  kafka:
    broker: ""
  exposed: {}
    # event-name:
    #   topic: kafka_topic_name
    #   eventBusName: kafka
    #   contentType: application/json
  consumed: {}
    # event-name:
    #   eventSourceName: producer-event-name
    #   eventName: event-name
    #   triggers:
    #     - name: trigger-name
    #       url: self
    #       path: /v1/endpoint        # required when url is "self"
    #       method: POST
    #       payload:
    #         - passthrough
    #   retryStrategy: {}
    #   dlq:
    #     enabled: true
    #     url: ""
```

- [ ] **Step 2: Run helm lint to verify syntax**

```bash
helm lint charts/bookinfo-service
```

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -m "feat(chart): add events.kafka/exposed/consumed defaults to values.yaml"
```

---

### Task 7: Add events Schema to values.schema.json

**Files:**
- Modify: `charts/bookinfo-service/values.schema.json`

- [ ] **Step 1: Add events property to the schema**

Add the following inside the top-level `"properties"` object, after the `"postgresql"` block:

```json
    "events": {
      "type": "object",
      "properties": {
        "kafka": {
          "type": "object",
          "properties": {
            "broker": { "type": "string" }
          }
        },
        "exposed": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "properties": {
              "topic": { "type": "string" },
              "eventBusName": { "type": "string" },
              "contentType": { "type": "string" }
            },
            "required": ["topic"]
          }
        },
        "consumed": {
          "type": "object",
          "additionalProperties": {
            "type": "object",
            "properties": {
              "eventSourceName": { "type": "string" },
              "eventName": { "type": "string" },
              "triggers": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "name": { "type": "string" },
                    "url": { "type": "string" },
                    "path": { "type": "string" },
                    "method": { "type": "string" },
                    "headers": { "type": "object" },
                    "payload": { "type": "array" }
                  },
                  "required": ["name", "url", "payload"]
                }
              },
              "retryStrategy": { "type": "object" },
              "dlq": {
                "type": "object",
                "properties": {
                  "enabled": { "type": "boolean" },
                  "url": { "type": "string" },
                  "retryStrategy": { "type": "object" }
                }
              }
            },
            "required": ["eventSourceName", "eventName", "triggers"]
          }
        }
      }
    }
```

- [ ] **Step 2: Run helm lint to verify**

```bash
helm lint charts/bookinfo-service
```

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/values.schema.json
git commit -m "feat(chart): add events schema to values.schema.json"
```

---

### Task 8: Add Consumer Sensor Name Helper to _helpers.tpl

**Files:**
- Modify: `charts/bookinfo-service/templates/_helpers.tpl`

- [ ] **Step 1: Add the consumer sensor name template**

Append to the end of `_helpers.tpl`:

```yaml
{{/*
Consumer sensor name: derived from release name (separate from CQRS sensor).
*/}}
{{- define "bookinfo-service.consumerSensorName" -}}
{{ include "bookinfo-service.fullname" . }}-consumer-sensor
{{- end }}
```

- [ ] **Step 2: Run helm lint**

```bash
helm lint charts/bookinfo-service
```

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/_helpers.tpl
git commit -m "feat(chart): add consumerSensorName helper to _helpers.tpl"
```

---

### Task 9: Inject KAFKA_BROKERS in ConfigMap Template

**Files:**
- Modify: `charts/bookinfo-service/templates/configmap.yaml`

- [ ] **Step 1: Add KAFKA_BROKERS injection**

Add the following block after the observability section (before the closing of the ConfigMap `data:`), right before the final empty line:

```yaml
  {{- with .Values.events.kafka.broker }}
  KAFKA_BROKERS: {{ . | quote }}
  {{- end }}
```

The full `configmap.yaml` should now be:

```yaml
{{/* charts/bookinfo-service/templates/configmap.yaml */}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
data:
  SERVICE_NAME: {{ include "bookinfo-service.serviceName" . | quote }}
  HTTP_PORT: {{ .Values.ports.http | quote }}
  ADMIN_PORT: {{ .Values.ports.admin | quote }}
  {{- if .Values.postgresql.enabled }}
  STORAGE_BACKEND: "postgres"
  RUN_MIGRATIONS: "true"
  DATABASE_URL: {{ printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable" .Values.postgresql.auth.username .Values.postgresql.auth.password .Release.Name (required "postgresql.auth.database must be set when postgresql.enabled=true" .Values.postgresql.auth.database) | quote }}
  {{- end }}
  {{- range $key, $value := .Values.config }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
  {{- with .Values.observability.otelEndpoint }}
  OTEL_EXPORTER_OTLP_ENDPOINT: {{ . | quote }}
  {{- end }}
  {{- with .Values.observability.pyroscopeAddress }}
  PYROSCOPE_SERVER_ADDRESS: {{ . | quote }}
  {{- end }}
  {{- with .Values.events.kafka.broker }}
  KAFKA_BROKERS: {{ . | quote }}
  {{- end }}
```

- [ ] **Step 2: Run helm template to verify**

```bash
helm template test charts/bookinfo-service --set events.kafka.broker="localhost:9092" | grep KAFKA_BROKERS
```

Expected: `KAFKA_BROKERS: "localhost:9092"`

- [ ] **Step 3: Verify it renders nothing when empty**

```bash
helm template test charts/bookinfo-service | grep KAFKA_BROKERS
```

Expected: no output (empty broker = no injection).

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/templates/configmap.yaml
git commit -m "feat(chart): inject KAFKA_BROKERS env var from events.kafka.broker"
```

---

### Task 10: Create kafka-eventsource.yaml Template

**Files:**
- Create: `charts/bookinfo-service/templates/kafka-eventsource.yaml`

- [ ] **Step 1: Write the Kafka EventSource template**

```yaml
{{/* charts/bookinfo-service/templates/kafka-eventsource.yaml */}}
{{- range $eventName, $event := .Values.events.exposed }}
---
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  eventBusName: {{ default "kafka" $event.eventBusName }}
  {{- with $.Values.observability.otelEndpoint }}
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ printf "%s-%s-eventsource" (include "bookinfo-service.fullname" $) $eventName | quote }}
  {{- end }}
  kafka:
    {{ $eventName }}:
      url: {{ required "events.kafka.broker must be set when events.exposed is defined" $.Values.events.kafka.broker }}
      topic: {{ required (printf "events.exposed.%s.topic is required" $eventName) $event.topic }}
      jsonBody: true
      {{- with $event.contentType }}
      contentType: {{ . }}
      {{- end }}
{{- end }}
```

- [ ] **Step 2: Dry-run render with a test value**

```bash
helm template test charts/bookinfo-service \
  --set events.kafka.broker="kafka:9092" \
  --set events.exposed.raw-books-details.topic=raw_books_details \
  --set events.exposed.raw-books-details.eventBusName=kafka \
  --set events.exposed.raw-books-details.contentType=application/json \
  | grep -A 20 "kind: EventSource"
```

Expected: renders an EventSource with `kafka:` source type (not `webhook:`), topic `raw_books_details`.

- [ ] **Step 3: Verify no EventSource is rendered when events.exposed is empty**

```bash
helm template test charts/bookinfo-service | grep "kind: EventSource"
```

Expected: no output (default values have empty `events.exposed`).

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/templates/kafka-eventsource.yaml
git commit -m "feat(chart): add kafka-eventsource.yaml template for events.exposed"
```

---

### Task 11: Create consumer-sensor.yaml Template

**Files:**
- Create: `charts/bookinfo-service/templates/consumer-sensor.yaml`

- [ ] **Step 1: Write the Consumer Sensor template**

This template reuses the same trigger patterns as the existing CQRS `sensor.yaml` (self URL resolution, passthrough/structured payload, retry, DLQ) but generates a separate Sensor resource.

```yaml
{{/* charts/bookinfo-service/templates/consumer-sensor.yaml */}}
{{- $hasConsumed := false }}
{{- range $_, $event := .Values.events.consumed }}
{{- if $event.triggers }}
{{- $hasConsumed = true }}
{{- end }}
{{- end }}
{{- if $hasConsumed }}
{{- $consumedCount := 0 }}
{{- range $_, $event := .Values.events.consumed }}
{{- $consumedCount = add $consumedCount 1 }}
{{- end }}
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: {{ include "bookinfo-service.consumerSensorName" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
spec:
  eventBusName: {{ .Values.cqrs.eventBusName }}
  {{- with .Values.observability.otelEndpoint }}
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ include "bookinfo-service.consumerSensorName" $ | quote }}
  {{- end }}
  dependencies:
    {{- range $eventName, $event := $.Values.events.consumed }}
    - name: {{ $eventName }}-dep
      eventSourceName: {{ $event.eventSourceName }}
      eventName: {{ $event.eventName }}
    {{- end }}
  triggers:
    {{- range $eventName, $event := $.Values.events.consumed }}
    {{- $retryStrategy := default $.Values.sensor.retryStrategy $event.retryStrategy }}
    {{- range $trigger := $event.triggers }}
    {{- /* Resolve trigger URL */ -}}
    {{- $triggerURL := $trigger.url }}
    {{- if eq $trigger.url "self" }}
    {{- if $.Values.cqrs.enabled }}
    {{- $triggerURL = printf "http://%s-write.%s.svc.cluster.local%s" (include "bookinfo-service.fullname" $) $.Release.Namespace (required (printf "events.consumed.%s.triggers[].path is required when url is 'self'" $eventName) $trigger.path) }}
    {{- else }}
    {{- $triggerURL = printf "http://%s.%s.svc.cluster.local%s" (include "bookinfo-service.fullname" $) $.Release.Namespace (required (printf "events.consumed.%s.triggers[].path is required when url is 'self'" $eventName) $trigger.path) }}
    {{- end }}
    {{- end }}
    {{- /* Resolve trigger method */ -}}
    {{- $triggerMethod := default "POST" $trigger.method }}
    - template:
        name: {{ $trigger.name }}
        {{- if gt $consumedCount 1 }}
        conditions: {{ $eventName }}-dep
        {{- end }}
        http:
          url: {{ $triggerURL }}
          method: {{ $triggerMethod }}
          headers:
            Content-Type: application/json
            {{- with $trigger.headers }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          payload:
            {{- range $p := $trigger.payload }}
            {{- if eq (kindOf $p) "string" }}
            - src:
                dependencyName: {{ $eventName }}-dep
                dataKey: body
              dest: ""
            {{- else }}
            - src:
                dependencyName: {{ default (printf "%s-dep" $eventName) $p.src.dependencyName }}
                {{- with $p.src.dataKey }}
                dataKey: {{ . }}
                {{- end }}
                {{- with $p.src.value }}
                value: {{ . | quote }}
                {{- end }}
                {{- with $p.src.contextKey }}
                contextKey: {{ . }}
                {{- end }}
              dest: {{ $p.dest }}
            {{- end }}
            {{- end }}
      atLeastOnce: {{ $.Values.sensor.atLeastOnce }}
      retryStrategy:
        steps: {{ $retryStrategy.steps }}
        duration: {{ $retryStrategy.duration }}
        factor: {{ $retryStrategy.factor }}
        jitter: {{ $retryStrategy.jitter }}
      {{- /* DLQ trigger */ -}}
      {{- $dlq := default $.Values.sensor.dlq $event.dlq }}
      {{- if $dlq.enabled }}
      {{- $dlqRetry := default $.Values.sensor.dlq.retryStrategy $dlq.retryStrategy }}
      dlqTrigger:
        template:
          name: dlq-{{ $trigger.name }}
          http:
            url: {{ $dlq.url }}
            method: POST
            headers:
              Content-Type: application/json
            payload:
              - src:
                  dependencyName: {{ $eventName }}-dep
                  dataKey: body
                dest: original_payload
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: id
                dest: event_id
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: type
                dest: event_type
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: source
                dest: event_source
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: subject
                dest: event_subject
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: time
                dest: event_timestamp
              - src:
                  dependencyName: {{ $eventName }}-dep
                  contextKey: datacontenttype
                dest: datacontenttype
              - src:
                  dependencyName: {{ $eventName }}-dep
                  dataKey: header
                dest: original_headers
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ include "bookinfo-service.consumerSensorName" $ | quote }}
                dest: sensor_name
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $trigger.name | quote }}
                dest: failed_trigger
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $event.eventSourceName | quote }}
                dest: eventsource_name
              - src:
                  dependencyName: {{ $eventName }}-dep
                  value: {{ $.Release.Namespace | quote }}
                dest: namespace
        atLeastOnce: true
        retryStrategy:
          steps: {{ $dlqRetry.steps }}
          duration: {{ $dlqRetry.duration }}
          factor: {{ $dlqRetry.factor }}
          jitter: {{ $dlqRetry.jitter }}
      {{- end }}
    {{- end }}
    {{- end }}
{{- end }}
```

- [ ] **Step 2: Verify no Sensor is rendered when events.consumed is empty**

```bash
helm template test charts/bookinfo-service | grep "consumer-sensor"
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/consumer-sensor.yaml
git commit -m "feat(chart): add consumer-sensor.yaml template for events.consumed"
```

---

### Task 12: Add CI Test Values Files

**Files:**
- Create: `charts/bookinfo-service/ci/values-ingestion-kafka.yaml`
- Create: `charts/bookinfo-service/ci/values-details-consumer.yaml`

- [ ] **Step 1: Create CI test values for exposed events (ingestion)**

```yaml
# charts/bookinfo-service/ci/values-ingestion-kafka.yaml
# ct install test: Kafka producer + EventSource (no CQRS)
serviceName: ingestion
fullnameOverride: ingestion
image:
  repository: event-driven-bookinfo/ingestion
  tag: latest

config:
  LOG_LEVEL: "debug"
  POLL_INTERVAL: "5m"
  SEARCH_QUERIES: "programming"
  MAX_RESULTS_PER_QUERY: "5"

events:
  kafka:
    broker: "kafka-bootstrap:9092"
  exposed:
    raw-books-details:
      topic: raw_books_details
      eventBusName: kafka
      contentType: application/json
```

- [ ] **Step 2: Create CI test values for consumed events (details)**

```yaml
# charts/bookinfo-service/ci/values-details-consumer.yaml
# ct install test: CQRS + consumed events + DLQ
serviceName: details
fullnameOverride: details
image:
  repository: event-driven-bookinfo/details
  tag: latest

cqrs:
  enabled: true
  endpoints:
    book-added:
      port: 12000
      method: POST
      endpoint: /v1/details
      triggers:
        - name: create-detail
          url: self
          payload:
            - passthrough

sensor:
  dlq:
    url: "http://dlq-eventsource-svc:12004/v1/events"

events:
  kafka:
    broker: "kafka-bootstrap:9092"
  consumed:
    raw-books-details:
      eventSourceName: ingestion-raw-books-details
      eventName: raw-books-details
      triggers:
        - name: ingest-book-detail
          url: self
          path: /v1/details
          method: POST
          payload:
            - passthrough
      dlq:
        enabled: true
        url: "http://dlq-eventsource-svc:12004/v1/events"
```

- [ ] **Step 3: Run helm lint with both CI values**

```bash
helm lint charts/bookinfo-service -f charts/bookinfo-service/ci/values-ingestion-kafka.yaml && \
helm lint charts/bookinfo-service -f charts/bookinfo-service/ci/values-details-consumer.yaml
```

Expected: both lint successfully.

- [ ] **Step 4: Dry-run render the ingestion values and verify Kafka EventSource**

```bash
helm template ingestion charts/bookinfo-service -f charts/bookinfo-service/ci/values-ingestion-kafka.yaml
```

Expected output should include:
- A ConfigMap with `KAFKA_BROKERS: "kafka-bootstrap:9092"`
- An EventSource named `ingestion-raw-books-details` with `kafka:` source type
- No webhook EventSource, no Sensor (ingestion is producer-only)

- [ ] **Step 5: Dry-run render the details values and verify both Sensors coexist**

```bash
helm template details charts/bookinfo-service -f charts/bookinfo-service/ci/values-details-consumer.yaml
```

Expected output should include:
- CQRS Sensor named `details-sensor` (from `cqrs.endpoints`)
- Consumer Sensor named `details-consumer-sensor` (from `events.consumed`)
- Webhook EventSource `book-added` (from CQRS)
- No Kafka EventSource (details is a consumer, not a producer)

- [ ] **Step 6: Commit**

```bash
git add charts/bookinfo-service/ci/values-ingestion-kafka.yaml charts/bookinfo-service/ci/values-details-consumer.yaml
git commit -m "test(chart): add CI values for Kafka EventSource and consumer Sensor"
```

---

### Task 13: Update Deploy Values (Ingestion + Details)

**Files:**
- Modify: `deploy/ingestion/values-local.yaml`
- Modify: `deploy/details/values-local.yaml`

- [ ] **Step 1: Update deploy/ingestion/values-local.yaml**

Replace the full file content with:

```yaml
# deploy/ingestion/values-local.yaml
serviceName: ingestion
fullnameOverride: ingestion
image:
  repository: event-driven-bookinfo/ingestion
  tag: local

config:
  LOG_LEVEL: "debug"
  POLL_INTERVAL: "5m"
  SEARCH_QUERIES: "programming,golang,kubernetes,python,devops"
  MAX_RESULTS_PER_QUERY: "10"

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    raw-books-details:
      topic: raw_books_details
      eventBusName: kafka
      contentType: application/json

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"
```

Key change: `GATEWAY_URL` removed from `config`, `events` block added.

- [ ] **Step 2: Update deploy/details/values-local.yaml**

Add the `events` block at the end of the file, after the existing `gateway` section:

```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  consumed:
    raw-books-details:
      eventSourceName: ingestion-raw-books-details
      eventName: raw-books-details
      triggers:
        - name: ingest-book-detail
          url: self
          path: /v1/details
          method: POST
          payload:
            - passthrough
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
```

All existing content (CQRS endpoints, sensor, gateway, etc.) stays unchanged.

- [ ] **Step 3: Verify both render correctly**

```bash
helm template ingestion charts/bookinfo-service -f deploy/ingestion/values-local.yaml && \
helm template details charts/bookinfo-service -f deploy/details/values-local.yaml
```

Expected: clean render, no errors.

- [ ] **Step 4: Run full helm lint**

```bash
make helm-lint
```

Expected: all per-service values lint successfully.

- [ ] **Step 5: Commit**

```bash
git add deploy/ingestion/values-local.yaml deploy/details/values-local.yaml
git commit -m "feat(deploy): update ingestion and details values for Kafka event pipeline"
```

---

### Task 14: Run Full Test Suite and Lint

**Files:** none (verification only)

- [ ] **Step 1: Run all Go tests with race detection**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 2: Run linter**

```bash
make lint
```

Expected: no lint errors.

- [ ] **Step 3: Run helm lint**

```bash
make helm-lint
```

Expected: all charts lint successfully.

- [ ] **Step 4: Build all service binaries**

```bash
make build-all
```

Expected: all 7 binaries built to `bin/`.

- [ ] **Step 5: Build Docker images**

```bash
make docker-build-all
```

Expected: all 7 images built successfully.

---

### Task 15: Bump Chart Version

**Files:**
- Modify: `charts/bookinfo-service/Chart.yaml`

- [ ] **Step 1: Bump chart version**

The chart version needs bumping since templates changed. Increment the minor version:

In `charts/bookinfo-service/Chart.yaml`, change `version: 0.2.0` to `version: 0.3.0`.

- [ ] **Step 2: Rebuild chart dependencies**

```bash
helm dependency build charts/bookinfo-service
```

Expected: dependencies downloaded successfully.

- [ ] **Step 3: Run helm lint one final time**

```bash
make helm-lint
```

Expected: `0 chart(s) failed`

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/Chart.yaml charts/bookinfo-service/Chart.lock
git commit -m "chore(chart): bump bookinfo-service chart version to 0.3.0"
```

---

### Task 16: Update Documentation (README.md, CLAUDE.md)

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

README.md needs updates in these locations (line numbers approximate — verify before editing):

- [ ] **Step 1: Update README services table — ingestion row**

Change the ingestion description from:
```
Polls Open Library for books on a configurable interval and publishes `book-added` events to the Gateway. Stateless; no storage adapters.
```
To:
```
Polls Open Library for books on a configurable interval and publishes CloudEvents directly to Kafka (`raw_books_details` topic). Stateless; no storage adapters.
```

- [ ] **Step 2: Update README intro paragraph**

In the intro paragraph (~line 66), change:
```
A standalone `ingestion` service demonstrates the pipeline as a self-hosted event broker — it polls the Open Library API and publishes synthetic book-added events through the same Gateway → EventSource → Kafka → Sensor path used by the UI.
```
To:
```
A standalone `ingestion` service demonstrates the pipeline as a self-hosted event broker — it polls the Open Library API and publishes CloudEvents directly to a dedicated Kafka topic (`raw_books_details`). Downstream services consume these events by declaring an Argo Events Kafka EventSource and Sensor, independent of the CQRS webhook pipeline used by the UI.
```

- [ ] **Step 3: Update README "Data Ingestion" section**

Replace the entire "Data Ingestion" section (~lines 555-561) with:

```markdown
## Data Ingestion

The `ingestion` service is a synthetic-data generator that publishes CloudEvents directly to a dedicated Kafka topic (`raw_books_details`) using a native Go Kafka client (franz-go). It runs as a single stateless deployment with no storage adapters and no CQRS split.

A background poll loop ticks every `POLL_INTERVAL` (default 5m). For each query in `SEARCH_QUERIES`, the service calls `GET https://openlibrary.org/search.json`, validates each returned book (title, ISBN, authors, and publish year are required), and publishes accepted books as CloudEvents to Kafka. Each event carries a deterministic idempotency key `ingestion-isbn-<ISBN>` and CloudEvents headers (`ce_type: com.bookinfo.ingestion.book-added`, `ce_source: ingestion`). On-demand scrapes are also available via `POST /v1/ingestion/trigger`.

The Helm chart creates a Kafka EventSource (`ingestion-raw-books-details`) that reads from the topic and makes it available on the EventBus. Downstream services (like `details`) declare `events.consumed` in their Helm values to create a Consumer Sensor with triggers, independently of the CQRS webhook Sensor.

Configuration via `KAFKA_BROKERS`, `KAFKA_TOPIC`, `POLL_INTERVAL`, `SEARCH_QUERIES`, and `MAX_RESULTS_PER_QUERY` environment variables. The topic is auto-created on startup if it doesn't exist. Because idempotency is enforced at the write service, replays and overlapping cycles are safe.
```

- [ ] **Step 4: Update README "Run Locally" ingestion example**

Replace the ingestion run command (~lines 254-260) with:

```bash
# Optional standalone demo — publishes CloudEvents to Kafka
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  KAFKA_BROKERS=localhost:9092 \
  KAFKA_TOPIC=raw_books_details \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  ./bin/ingestion
```

- [ ] **Step 5: Update README directory tree — ingestion outbound adapters**

Change (~line 670):
```
│               └── outbound/   # openlibrary/ (BookFetcher), gateway/ (EventPublisher)
```
To:
```
│               └── outbound/   # openlibrary/ (BookFetcher), kafka/ (EventPublisher)
```

- [ ] **Step 6: Update CLAUDE.md — ingestion service description**

In the Services table, change ingestion description from:
```
Open Library scraper; polls on interval, publishes book-added events to the Gateway
```
To:
```
Open Library scraper; polls on interval, publishes CloudEvents to Kafka (`raw_books_details` topic)
```

- [ ] **Step 7: Update CLAUDE.md — Architecture section**

In the Architecture section, change the ingestion bullet from:
```
- **Ingestion**: `ingestion` service polls Open Library on `POLL_INTERVAL` → for each query in `SEARCH_QUERIES` → `POST {GATEWAY_URL}/v1/details` with `idempotency_key=ingestion-isbn-<ISBN>`. Stateless, single deployment, no CQRS split, no EventSource/Sensor of its own. Exercises the full write pipeline end-to-end.
```
To:
```
- **Ingestion**: `ingestion` service polls Open Library on `POLL_INTERVAL` → produces CloudEvents to Kafka topic `raw_books_details` via franz-go. Stateless, single deployment, no CQRS split. Creates a Kafka EventSource (`ingestion-raw-books-details`) for downstream consumers.
- **Event consumption**: services declare `events.consumed` in Helm values to create a Consumer Sensor with triggers and DLQ, independent of the CQRS Sensor. The `details` service consumes `raw_books_details` events via the `details-consumer-sensor`.
```

- [ ] **Step 8: Update CLAUDE.md — Run Locally ingestion block**

Replace the ingestion run command with:

```bash
**ingestion** (publishes to Kafka; requires Kafka broker reachable):
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  KAFKA_BROKERS=localhost:9092 \
  KAFKA_TOPIC=raw_books_details \
  POLL_INTERVAL=5m \
  SEARCH_QUERIES=programming,golang \
  MAX_RESULTS_PER_QUERY=10 \
  go run ./services/ingestion/cmd/
```

- [ ] **Step 9: Update CLAUDE.md — Optional env vars**

Change the ingestion-specific env vars line from:
```
Ingestion-specific: `GATEWAY_URL`, `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`.
```
To:
```
Ingestion-specific: `KAFKA_BROKERS`, `KAFKA_TOPIC` (default `raw_books_details`), `POLL_INTERVAL`, `SEARCH_QUERIES`, `MAX_RESULTS_PER_QUERY`.
```

- [ ] **Step 10: Update CLAUDE.md — Helm Commands section**

Add after the existing `helm upgrade` example:

```markdown
## Helm Events Configuration

Services can declare Kafka event pipelines independent of CQRS:

- **`events.exposed`**: creates a Kafka-type EventSource (used by producer services like ingestion)
- **`events.consumed`**: creates a Consumer Sensor with triggers and DLQ (used by consuming services like details)
- **`events.kafka.broker`**: injects `KAFKA_BROKERS` env var into ConfigMap

These coexist with the existing CQRS pattern (`cqrs.endpoints`) without modification.
```

- [ ] **Step 11: Commit documentation updates**

```bash
git add README.md CLAUDE.md
git commit -m "docs: update ingestion and Helm chart documentation for Kafka producer architecture"
```

---

### Task 17: Full Local Kubernetes Validation

**Files:** none (validation only)

**Prerequisites:** All code and chart changes from Tasks 1-16 are committed.

- [ ] **Step 1: Tear down existing cluster**

```bash
make stop-k8s
```

Expected: k3d cluster deleted.

- [ ] **Step 2: Bring up full cluster from scratch**

```bash
make run-k8s
```

Expected: cluster created, platform installed (Strimzi, Kafka, Argo Events, Gateway), observability installed (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy), all services deployed, databases seeded. This takes several minutes.

- [ ] **Step 3: Verify all pods are running**

```bash
make k8s-status
```

Expected: all pods in `bookinfo`, `platform`, `observability`, and `envoy-gateway-system` namespaces are Running/Ready. Specifically check:
- `ingestion` pod is Running (single deployment, no CQRS)
- `details-read` and `details-write` pods are Running
- `details-consumer-sensor` controller pod exists (Argo Events Sensor)
- `ingestion-raw-books-details-eventsource` controller pod exists (Argo Events EventSource)

- [ ] **Step 4: Verify Kafka EventSource and Consumer Sensor CRDs**

```bash
kubectl get eventsources -n bookinfo --context=k3d-bookinfo-local
```

Expected: should include both existing webhook EventSources (`book-added`, `rating-submitted`, etc.) AND the new Kafka EventSource (`ingestion-raw-books-details`).

```bash
kubectl get sensors -n bookinfo --context=k3d-bookinfo-local
```

Expected: should include both CQRS sensors (`details-sensor`, `ratings-sensor`, etc.) AND the new `details-consumer-sensor`.

- [ ] **Step 5: Verify Kafka topic auto-creation**

```bash
kubectl exec -n platform bookinfo-kafka-kafka-0 --context=k3d-bookinfo-local -- \
  bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
```

Expected: `raw_books_details` topic should exist (auto-created by ingestion on startup). The `argo-events` topic should also still exist.

```bash
kubectl exec -n platform bookinfo-kafka-kafka-0 --context=k3d-bookinfo-local -- \
  bin/kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic raw_books_details
```

Expected: topic with 3 partitions, RF 1.

---

### Task 18: Smoke Tests — Ingestion Pipeline Validation

**Files:** none (validation only)

- [ ] **Step 1: Wait for first ingestion cycle to complete**

The ingestion service polls every 5 minutes. Check its logs:

```bash
kubectl logs -n bookinfo -l app=ingestion --tail=50 --context=k3d-bookinfo-local
```

Expected: logs showing "poll loop: starting ingestion cycle" and "published book-added event to Kafka" entries. If not enough time has passed, trigger manually:

```bash
curl -s -X POST http://localhost:8080/v1/ingestion/trigger \
  -H "Content-Type: application/json" \
  -d '{"queries":["golang"]}' | jq .
```

Expected: JSON response with `books_found > 0` and `events_published > 0`.

- [ ] **Step 2: Verify ingestion status**

```bash
curl -s http://localhost:8080/v1/ingestion/status | jq .
```

Expected: `state: "idle"`, `last_result.events_published > 0`, `last_result.errors` should be 0 or low.

- [ ] **Step 3: Verify Kafka messages exist in topic**

```bash
kubectl exec -n platform bookinfo-kafka-kafka-0 --context=k3d-bookinfo-local -- \
  bin/kafka-console-consumer.sh --bootstrap-server localhost:9092 \
  --topic raw_books_details --from-beginning --max-messages 3 --timeout-ms 10000
```

Expected: JSON messages with CloudEvents headers. Each message should contain book data with `title`, `isbn`, `idempotency_key` fields.

- [ ] **Step 4: Verify details service received events via Consumer Sensor**

```bash
curl -s http://localhost:8080/v1/details | jq '.[] | {id, title, isbn_10, isbn_13}' | head -30
```

Expected: details created by the ingestion pipeline — books with ISBNs matching what was published. These came through: Kafka → Kafka EventSource → Consumer Sensor → details-write.

- [ ] **Step 5: Verify CQRS webhook pipeline still works (no regression)**

Post a book directly through the Gateway (CQRS path):

```bash
curl -s -X POST http://localhost:8080/v1/details \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Smoke Test Book",
    "author": "Smoke Tester",
    "year": 2026,
    "type": "paperback",
    "pages": 100,
    "publisher": "Test Press",
    "language": "en",
    "isbn_10": "0000000001",
    "idempotency_key": "smoke-test-cqrs-path"
  }' | jq .
```

Wait 5 seconds for async pipeline, then verify:

```bash
curl -s http://localhost:8080/v1/details | jq '.[] | select(.isbn_10=="0000000001")'
```

Expected: the smoke test book appears — confirming the CQRS webhook path (Gateway → book-added EventSource → details-sensor → details-write) is unbroken.

- [ ] **Step 6: Submit a rating through CQRS path (cross-service check)**

```bash
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": "smoke-test-product",
    "reviewer": "smoke-tester",
    "stars": 5,
    "idempotency_key": "smoke-test-rating"
  }' | jq .
```

Wait 5 seconds, then:

```bash
curl -s http://localhost:8080/v1/ratings/smoke-test-product | jq .
```

Expected: rating created — confirming other CQRS pipelines (ratings, notifications) are untouched.

---

### Task 19: Grafana Traces Validation — Kafka Trace Continuity

**Files:** none (validation only)

This task verifies that distributed traces are NOT broken when events flow through Kafka. The ingestion producer should create spans, and the consumer sensor triggers should propagate trace context.

- [ ] **Step 1: Trigger an ingestion cycle with tracing context**

```bash
curl -s -X POST http://localhost:8080/v1/ingestion/trigger \
  -H "Content-Type: application/json" \
  -d '{"queries":["kubernetes"]}' | jq .
```

Note the time. Wait 10 seconds for the full pipeline to process.

- [ ] **Step 2: Open Grafana and check Tempo traces**

Open `http://localhost:3000` in a browser. Navigate to Explore → Tempo.

Search for traces with:
- Service name: `ingestion`
- Operation: `PublishBookAdded` or Kafka produce spans
- Time range: last 5 minutes

Expected: traces showing spans from the ingestion service producing to Kafka. Look for:
- `ingestion` service spans (HTTP handler → service → Kafka produce)
- Verify the Kafka produce operation has a span

- [ ] **Step 3: Check details-write traces from Consumer Sensor**

In Grafana Tempo, search for:
- Service name: `details`
- Operation: containing `POST /v1/details`
- Time range: last 5 minutes

Expected: traces showing the details-write service receiving HTTP POSTs from the Consumer Sensor. Look for:
- An HTTP server span on details-write
- The span should have attributes showing it was triggered by the sensor

- [ ] **Step 4: Verify span metrics in Prometheus**

Open `http://localhost:9090` (Prometheus). Query:

```promql
traces_spanmetrics_calls_total{service="ingestion"}
```

Expected: non-zero counter — ingestion is producing traced events.

```promql
traces_spanmetrics_calls_total{service="details", span_kind="SPAN_KIND_SERVER"}
```

Expected: non-zero counter — details is receiving traced requests (from both CQRS and consumer sensor paths).

- [ ] **Step 5: Check app-observability dashboard**

Open `http://localhost:3000/d/app-observability` (Grafana dashboard).

Expected:
- `ingestion` service appears in the request rate panel
- `details` service shows request rate from both CQRS sensor and consumer sensor triggers
- No error spikes in the error rate panel
- Latency metrics are populated for both services

---

### Task 20: k6 Load Test Validation

**Files:** none (validation only)

- [ ] **Step 1: Run k6 load test**

```bash
make k8s-load DURATION=2m BASE_RATE=2
```

Expected: k6 runs successfully, all thresholds pass:
- `http_req_duration` p95 < 2000ms
- `http_req_failed` rate < 10%

The load test exercises the full read path (productpage → details → reviews → ratings). Details will return books ingested via the Kafka pipeline, confirming the data is accessible through the normal read path.

- [ ] **Step 2: Verify k6 metrics in Grafana**

Open `http://localhost:3000/d/k6-load-testing` (k6 dashboard).

Expected:
- Virtual users, request rate, and duration panels are populated
- No error spike during the test
- Latency distribution shows reasonable p95/p99 values

- [ ] **Step 3: Run DLQ resilience test (regression check)**

```bash
make k8s-dlq-test
```

Expected: both inject and replay phases pass with 100% check rate. This confirms:
- The CQRS sensor DLQ triggers still work correctly
- The DLQ replay mechanism is unbroken
- The ratings pipeline is not affected by the ingestion changes

---

### Task 21: Open Pull Request

**Files:** none (git/GitHub operations only)

- [ ] **Step 1: Push feature branch**

```bash
git push -u origin HEAD
```

- [ ] **Step 2: Create pull request**

```bash
gh pr create --title "feat(ingestion): native Kafka producer with events.exposed/consumed Helm abstractions" --body "$(cat <<'EOF'
## Summary

- Refactored ingestion service from HTTP Gateway publisher to native Kafka producer (franz-go)
- Ingestion publishes CloudEvents directly to dedicated Kafka topic (`raw_books_details`) with topic auto-creation
- Added `events.exposed` Helm chart abstraction — creates Kafka-type EventSources (Argo Events)
- Added `events.consumed` Helm chart abstraction — creates Consumer Sensors with triggers and DLQ, independent of CQRS Sensors
- Added `events.kafka.broker` for Kafka broker address injection via ConfigMap
- Details service consumes `raw_books_details` via `details-consumer-sensor`
- Existing CQRS webhook pattern (EventSources, Sensors, HTTPRoutes) is completely untouched
- Updated README.md and CLAUDE.md documentation

## Changes

### Go Code
- New: `services/ingestion/internal/adapter/outbound/kafka/producer.go` — franz-go producer with CloudEvents headers, topic auto-creation via kadm
- Removed: `services/ingestion/internal/adapter/outbound/gateway/publisher.go` — replaced by Kafka adapter
- Modified: `services/ingestion/cmd/main.go` — wires Kafka producer, removes gateway dependency
- Modified: `pkg/config/config.go` — adds KafkaBrokers + KafkaTopic fields
- New dependencies: `github.com/twmb/franz-go`, `github.com/cloudevents/sdk-go/v2`

### Helm Chart
- New template: `kafka-eventsource.yaml` — Kafka-type EventSource per `events.exposed` entry
- New template: `consumer-sensor.yaml` — separate Sensor per `events.consumed` with full trigger/DLQ support
- Modified: `configmap.yaml` — injects `KAFKA_BROKERS` when `events.kafka.broker` set
- Modified: `values.yaml` — `events` defaults
- Modified: `values.schema.json` — `events` schema validation
- New CI values: `values-ingestion-kafka.yaml`, `values-details-consumer.yaml`
- Chart version bumped to 0.3.0

### Deploy
- `deploy/ingestion/values-local.yaml` — GATEWAY_URL replaced with events.exposed config
- `deploy/details/values-local.yaml` — events.consumed added alongside existing CQRS config

## Test plan

- [ ] CI passes (lint, vet, test, build, docker, e2e, helm-lint)
- [ ] OSSF scorecard >= 7.0
- [ ] Local k8s: `make run-k8s` → all pods running
- [ ] Kafka topic `raw_books_details` auto-created (3 partitions)
- [ ] Ingestion publishes CloudEvents to Kafka successfully
- [ ] Details receives events via Consumer Sensor pipeline
- [ ] CQRS webhook pipeline still works (no regression)
- [ ] Distributed traces visible in Grafana/Tempo for both pipelines
- [ ] k6 load test passes (p95 < 2s, error rate < 10%)
- [ ] DLQ resilience test passes (inject + replay phases)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Wait for CI checks to complete**

```bash
gh pr checks --watch
```

Expected: all checks pass:
- Lint, Vet, Test (with race), Build, Docker build + Trivy scan
- E2E tests (memory + postgres backends)
- Helm lint/test
- Gitleaks, Govulncheck

- [ ] **Step 4: Verify OSSF Scorecard**

After the scorecard PR check completes:

```bash
gh pr checks | grep -i scorecard
```

Expected: scorecard check passes with score >= 7.0. If the score drops, investigate the PR comment for which checks regressed and fix.

If the score is at risk (new dependencies can affect Pinned-Dependencies or Vulnerabilities checks):
- Verify `go.sum` has valid checksums for new dependencies
- Ensure no new CVEs in franz-go or cloudevents-sdk-go
- Check that the new go dependencies are well-maintained (Scorecard evaluates upstream project health)

- [ ] **Step 5: Address any CI failures**

If any check fails, fix and push. Common issues:
- Helm lint may require `ct lint` to pass with new CI values files
- New Go dependencies may trigger govulncheck warnings
- Docker image scan may flag vulnerabilities in new dependencies
