# Event-Driven Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace parallel `notify-*` sensor triggers with CloudEvents emitted by write services after successful persistence; notification consumes via Argo Events with `ce_type` filtering. Notifications fire iff the corresponding write succeeded.

**Architecture:** Each write service (details, reviews, ratings) gains a `franz-go` Kafka producer wired through a new `EventPublisher` outbound port, called unconditionally after the idempotency branch. Returning 500 on publish failure triggers Argo Events sensor retries — idempotency dedupes the second Save while the publish retries. Notification stays HTTP-only; its Consumer Sensor filters events by `ce_type` and maps payload fields to the notification API.

**Tech Stack:** Go 1.22, `github.com/twmb/franz-go`, Argo Events 1.9+, Helm 3, CloudEvents v1.0 over Kafka headers.

**Spec reference:** `docs/superpowers/specs/2026-04-24-event-driven-notifications-design.md`

---

## File Structure

### New files

```
charts/bookinfo-service/templates/
  (no new files — existing templates extended)

services/details/internal/
  core/domain/event.go                          # BookAddedEvent type
  core/port/publisher.go                        # EventPublisher port
  adapter/outbound/kafka/producer.go            # franz-go adapter
  adapter/outbound/kafka/producer_test.go       # adapter unit test
  adapter/outbound/kafka/noop.go                # no-op publisher

services/reviews/internal/
  core/domain/event.go                          # ReviewSubmittedEvent, ReviewDeletedEvent
  core/port/publisher.go
  adapter/outbound/kafka/producer.go
  adapter/outbound/kafka/producer_test.go
  adapter/outbound/kafka/noop.go

services/ratings/internal/
  core/domain/event.go                          # RatingSubmittedEvent
  core/port/publisher.go
  adapter/outbound/kafka/producer.go
  adapter/outbound/kafka/producer_test.go
  adapter/outbound/kafka/noop.go
```

### Modified files

```
charts/bookinfo-service/templates/kafka-eventsource.yaml   # add eventTypes annotation
charts/bookinfo-service/templates/consumer-sensor.yaml     # add filter.ceType
charts/bookinfo-service/values.yaml                        # document new fields

deploy/details/values-local.yaml         # add events.exposed, KAFKA_*, drop notify-book-added
deploy/reviews/values-local.yaml         # add events.exposed, KAFKA_*, drop notify-review-*
deploy/ratings/values-local.yaml         # add events.exposed, KAFKA_*, drop notify-rating-submitted
deploy/notification/values-local.yaml    # add events.consumed × 4

services/details/cmd/main.go                                  # wire publisher
services/details/internal/core/service/detail_service.go      # always-publish semantics
services/details/internal/core/service/detail_service_test.go # fake publisher assertions

services/reviews/cmd/main.go
services/reviews/internal/core/service/review_service.go      # + idempotent DeleteReview
services/reviews/internal/core/service/review_service_test.go # (if exists) or handler test
services/reviews/internal/adapter/inbound/http/handler.go     # (if needed) return 204 on delete dedup

services/ratings/cmd/main.go
services/ratings/internal/core/service/rating_service.go
services/ratings/internal/core/service/rating_service_test.go
```

Each service's `kafka` adapter package follows ingestion's layout (`services/ingestion/internal/adapter/outbound/kafka/`) exactly — fake `Client` interface for testability, CloudEvents headers, `ensureTopic` on startup.

---

## Phase 1 — Chart extensions

Low risk, low coupling. Done first so Helm values written later can rely on the new fields.

### Task 1: Render `eventTypes` list as EventSource annotation

**Files:**
- Modify: `charts/bookinfo-service/templates/kafka-eventsource.yaml`

- [ ] **Step 1: Read current template to confirm structure**

Run: `cat charts/bookinfo-service/templates/kafka-eventsource.yaml`

Confirms current metadata block:
```yaml
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
```

- [ ] **Step 2: Add annotations block under metadata**

Replace the metadata block with:
```yaml
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
  {{- with $event.eventTypes }}
  annotations:
    bookinfo.io/emitted-ce-types: {{ join "," . | quote }}
  {{- end }}
```

- [ ] **Step 3: Verify chart still renders for ingestion (which has no eventTypes yet)**

Run: `helm template test charts/bookinfo-service -f deploy/ingestion/values-local.yaml | grep -A3 "kind: EventSource"`

Expected: EventSource `ingestion-raw-books-details` renders without an `annotations:` block (since `eventTypes` not defined).

- [ ] **Step 4: Verify with a temporary values snippet that annotation renders when eventTypes present**

Run:
```bash
helm template test charts/bookinfo-service -f deploy/ingestion/values-local.yaml \
  --set 'events.exposed.raw-books-details.eventTypes[0]=com.bookinfo.ingestion.book-added' \
  | grep -A5 "kind: EventSource"
```

Expected: rendered EventSource contains:
```yaml
annotations:
  bookinfo.io/emitted-ce-types: "com.bookinfo.ingestion.book-added"
```

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/templates/kafka-eventsource.yaml
git commit -m "feat(chart): render eventTypes list as EventSource annotation"
```

---

### Task 2: Add `filter.ceType` support to consumer-sensor dependencies

**Files:**
- Modify: `charts/bookinfo-service/templates/consumer-sensor.yaml`

- [ ] **Step 1: Locate the dependencies block**

Open `charts/bookinfo-service/templates/consumer-sensor.yaml` — dependencies render around lines 30-36:
```yaml
dependencies:
  {{- range $eventName, $event := $.Values.events.consumed }}
  - name: {{ $eventName }}-dep
    eventSourceName: {{ $event.eventSourceName }}
    eventName: {{ $event.eventName }}
  {{- end }}
```

- [ ] **Step 2: Add optional `filters.context.type` clause**

Replace the dependencies block with:
```yaml
dependencies:
  {{- range $eventName, $event := $.Values.events.consumed }}
  - name: {{ $eventName }}-dep
    eventSourceName: {{ $event.eventSourceName }}
    eventName: {{ $event.eventName }}
    {{- with $event.filter }}
    {{- with .ceType }}
    filters:
      context:
        type: {{ . | quote }}
    {{- end }}
    {{- end }}
  {{- end }}
```

- [ ] **Step 3: Verify no regression in details service (consumes ingestion without a filter)**

Run: `helm template test charts/bookinfo-service -f deploy/details/values-local.yaml | grep -A3 "raw-books-details-dep"`

Expected: `raw-books-details-dep` renders with `eventSourceName` and `eventName`, no `filters:` block.

- [ ] **Step 4: Verify filter renders when set**

Run:
```bash
helm template test charts/bookinfo-service -f deploy/details/values-local.yaml \
  --set 'events.consumed.raw-books-details.filter.ceType=com.example.test' \
  | grep -A5 "raw-books-details-dep"
```

Expected: dependency includes:
```yaml
filters:
  context:
    type: "com.example.test"
```

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/templates/consumer-sensor.yaml
git commit -m "feat(chart): optional ce_type filter on consumer sensor dependencies"
```

---

### Task 3: Document new fields in chart `values.yaml`

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`

- [ ] **Step 1: Locate events schema (around line 151)**

Current block:
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
    #   ...
```

- [ ] **Step 2: Update commented schema with the new optional fields**

Replace the `events:` block with:
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
    #   eventTypes:                        # optional, rendered as bookinfo.io/emitted-ce-types annotation on the EventSource
    #     - com.bookinfo.<service>.<event>
  consumed: {}
    # event-name:
    #   eventSourceName: producer-event-name
    #   eventName: event-name
    #   filter:                             # optional
    #     ceType: com.bookinfo.<service>.<event>   # matches Argo Events filters.context.type
    #   triggers:
    #     - name: trigger-name
    #       url: self
    #       path: /v1/endpoint             # required when url is "self"
    #       method: POST
    #       payload:
    #         - passthrough
    #   retryStrategy: {}
    #   dlq:
    #     enabled: true
    #     url: ""
```

- [ ] **Step 3: Run chart lint**

Run: `make helm-lint`

Expected: `==> Linting charts/bookinfo-service` followed by `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -m "docs(chart): document events.exposed.eventTypes and events.consumed.filter.ceType"
```

---

## Phase 2 — Details service producer

Implements the pattern end-to-end for one service. Reviews and ratings mirror this phase.

### Task 4: Add domain event type and outbound port

**Files:**
- Create: `services/details/internal/core/domain/event.go`
- Create: `services/details/internal/core/port/publisher.go`

- [ ] **Step 1: Create `event.go`**

Write this file exactly:
```go
// Package domain contains pure domain types for the details service.
package domain

// BookAddedEvent is the domain event emitted after a successful AddDetail.
// Carries the business payload shape consumed by downstream services (e.g. notification).
type BookAddedEvent struct {
	ID             string
	Title          string
	Author         string
	Year           int
	Type           string
	Pages          int
	Publisher      string
	Language       string
	ISBN10         string
	ISBN13         string
	IdempotencyKey string
}
```

- [ ] **Step 2: Create `publisher.go`**

Write this file exactly:
```go
// file: services/details/internal/core/port/publisher.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// EventPublisher is the outbound port for emitting domain events to a message broker.
type EventPublisher interface {
	PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error
}
```

- [ ] **Step 3: Run go build to verify compile**

Run: `go build ./services/details/...`

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add services/details/internal/core/domain/event.go services/details/internal/core/port/publisher.go
git commit -m "feat(details): add BookAddedEvent domain type and EventPublisher port"
```

---

### Task 5: Kafka producer adapter — write failing test first

**Files:**
- Create: `services/details/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the failing test**

Create the test file:
```go
// file: services/details/internal/adapter/outbound/kafka/producer_test.go
package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
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

type errorClient struct{}

func (e *errorClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	var results kgo.ProduceResults
	for _, r := range rs {
		results = append(results, kgo.ProduceResult{Record: r, Err: context.DeadlineExceeded})
	}
	return results
}

func (e *errorClient) Close() {}

func TestPublishBookAdded(t *testing.T) {
	t.Parallel()

	evt := domain.BookAddedEvent{
		ID:             "det_123",
		Title:          "The Go Programming Language",
		Author:         "Alan Donovan, Brian Kernighan",
		Year:           2015,
		Type:           "paperback",
		Pages:          380,
		Publisher:      "Addison-Wesley",
		Language:       "en",
		ISBN10:         "",
		ISBN13:         "9780134190440",
		IdempotencyKey: "det-idem-123",
	}

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_details_events")

	if err := p.PublishBookAdded(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	if len(fc.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fc.records))
	}
	r := fc.records[0]

	if r.Topic != "bookinfo_details_events" {
		t.Errorf("topic = %q, want %q", r.Topic, "bookinfo_details_events")
	}
	if string(r.Key) != evt.IdempotencyKey {
		t.Errorf("key = %q, want %q", string(r.Key), evt.IdempotencyKey)
	}

	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.details.book-added" {
		t.Errorf("ce_type = %q, want %q", headers["ce_type"], "com.bookinfo.details.book-added")
	}
	if headers["ce_source"] != "details" {
		t.Errorf("ce_source = %q, want %q", headers["ce_source"], "details")
	}
	if headers["ce_specversion"] != "1.0" {
		t.Errorf("ce_specversion = %q, want %q", headers["ce_specversion"], "1.0")
	}
	if headers["ce_subject"] != evt.IdempotencyKey {
		t.Errorf("ce_subject = %q, want %q", headers["ce_subject"], evt.IdempotencyKey)
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

	var body map[string]interface{}
	if err := json.Unmarshal(r.Value, &body); err != nil {
		t.Fatalf("failed to unmarshal record value: %v", err)
	}
	if body["title"] != evt.Title {
		t.Errorf("body title = %q, want %q", body["title"], evt.Title)
	}
	if body["author"] != evt.Author {
		t.Errorf("body author = %q, want %q", body["author"], evt.Author)
	}
	if body["year"].(float64) != float64(evt.Year) {
		t.Errorf("body year = %v, want %v", body["year"], evt.Year)
	}
	if body["isbn_13"] != evt.ISBN13 {
		t.Errorf("body isbn_13 = %q, want %q", body["isbn_13"], evt.ISBN13)
	}
	if body["idempotency_key"] != evt.IdempotencyKey {
		t.Errorf("body idempotency_key = %q, want %q", body["idempotency_key"], evt.IdempotencyKey)
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	p := kafkaadapter.NewProducerWithClient(&errorClient{}, "bookinfo_details_events")

	err := p.PublishBookAdded(context.Background(), domain.BookAddedEvent{
		Title: "t", Author: "a", Year: 2024, Type: "paperback",
		IdempotencyKey: "idem-x",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails (producer not defined yet)**

Run: `go test ./services/details/internal/adapter/outbound/kafka/... -v`

Expected: compile error `undefined: kafkaadapter.NewProducerWithClient` or similar.

---

### Task 6: Implement Kafka producer adapter

**Files:**
- Create: `services/details/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Create producer implementation**

Write this file exactly:
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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

const (
	ceTypeBookAdded = "com.bookinfo.details.book-added"
	ceSource        = "details"
	ceVersion       = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

// bookAddedBody is the marshaled Kafka record value for a BookAddedEvent.
type bookAddedBody struct {
	ID             string `json:"id,omitempty"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Year           int    `json:"year"`
	Type           string `json:"type"`
	Pages          int    `json:"pages,omitempty"`
	Publisher      string `json:"publisher,omitempty"`
	Language       string `json:"language,omitempty"`
	ISBN10         string `json:"isbn_10,omitempty"`
	ISBN13         string `json:"isbn_13,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer implements port.EventPublisher by producing to Kafka.
type Producer struct {
	client Client
	topic  string
}

// NewProducer creates a real Kafka producer connecting to the given brokers.
// Auto-creates the topic if it does not exist.
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
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishBookAdded sends a book-added CloudEvent to Kafka.
func (p *Producer) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	logger := logging.FromContext(ctx)

	body := bookAddedBody{
		ID:             evt.ID,
		Title:          evt.Title,
		Author:         evt.Author,
		Year:           evt.Year,
		Type:           evt.Type,
		Pages:          evt.Pages,
		Publisher:      evt.Publisher,
		Language:       evt.Language,
		ISBN10:         evt.ISBN10,
		ISBN13:         evt.ISBN13,
		IdempotencyKey: evt.IdempotencyKey,
	}

	value, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling book-added event: %w", err)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(evt.IdempotencyKey),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceTypeBookAdded)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(evt.IdempotencyKey)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published book-added event to Kafka", "topic", p.topic, "idempotency_key", evt.IdempotencyKey)
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
		if t.Err != nil && t.ErrMessage != "" && !isTopicExistsError(t.ErrMessage) {
			return fmt.Errorf("topic %q: %s", t.Topic, t.ErrMessage)
		}
	}

	return nil
}

func isTopicExistsError(msg string) bool {
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "TOPIC_ALREADY_EXISTS")
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./services/details/internal/adapter/outbound/kafka/... -v`

Expected: both tests PASS.

- [ ] **Step 3: Commit**

```bash
git add services/details/internal/adapter/outbound/kafka/producer.go \
        services/details/internal/adapter/outbound/kafka/producer_test.go
git commit -m "feat(details): add franz-go Kafka producer for BookAddedEvent"
```

---

### Task 7: Add no-op publisher for docker-compose / unit tests

**Files:**
- Create: `services/details/internal/adapter/outbound/kafka/noop.go`

- [ ] **Step 1: Create no-op adapter**

Write this file exactly:
```go
// file: services/details/internal/adapter/outbound/kafka/noop.go
package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher but performs no work.
// Used when KAFKA_BROKERS is not set (e.g. docker-compose, unit tests).
type NoopPublisher struct{}

// NewNoopPublisher returns a publisher that silently succeeds.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishBookAdded logs at debug and returns nil.
func (n *NoopPublisher) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: book-added event discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./services/details/...`

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add services/details/internal/adapter/outbound/kafka/noop.go
git commit -m "feat(details): add no-op publisher for broker-less environments"
```

---

### Task 8: Wire publisher into `DetailService` with always-publish semantics

**Files:**
- Modify: `services/details/internal/core/service/detail_service.go`
- Modify: `services/details/internal/core/service/detail_service_test.go`

- [ ] **Step 1: Write the new service test (TDD)**

Open `services/details/internal/core/service/detail_service_test.go`. Add these imports and helpers at the top:

```go
import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)

type fakePublisher struct {
	mu       sync.Mutex
	calls    []domain.BookAddedEvent
	forceErr error
}

func (f *fakePublisher) PublishBookAdded(_ context.Context, evt domain.BookAddedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, evt)
	return f.forceErr
}
```

Replace the existing `TestAddDetail_Success` with one that also constructs a `fakePublisher` and asserts publish was called exactly once:

```go
func TestAddDetail_Success(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

	detail, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID == "" {
		t.Error("expected non-empty ID")
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].Title != "The Art of Go" {
		t.Errorf("event Title = %q, want %q", pub.calls[0].Title, "The Art of Go")
	}
}
```

Replace `TestAddDetail_ValidationError`, `TestGetDetail_Found`, `TestGetDetail_NotFound` to pass the publisher:
```go
func TestAddDetail_ValidationError(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	_, err := svc.AddDetail(context.Background(),
		"", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
}

func TestGetDetail_Found(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	created, err := svc.AddDetail(context.Background(),
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "",
	)
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	found, err := svc.GetDetail(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error getting: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("ID = %q, want %q", found.ID, created.ID)
	}
}

func TestGetDetail_NotFound(t *testing.T) {
	repo := memory.NewDetailRepository()
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})

	_, err := svc.GetDetail(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent detail")
	}
}
```

Add a new test asserting publish runs on idempotency dedup (Option A semantics):

```go
func TestAddDetail_PublishesOnIdempotencyDedup(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

	args := []interface{}{
		"The Art of Go", "Jane Doe", 2024, "paperback",
		350, "Go Press", "English", "1234567890", "1234567890123", "fixed-key",
	}

	_, err := svc.AddDetail(context.Background(),
		args[0].(string), args[1].(string), args[2].(int), args[3].(string),
		args[4].(int), args[5].(string), args[6].(string), args[7].(string), args[8].(string), args[9].(string),
	)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Second call with same idempotency key
	_, err = svc.AddDetail(context.Background(),
		args[0].(string), args[1].(string), args[2].(int), args[3].(string),
		args[4].(int), args[5].(string), args[6].(string), args[7].(string), args[8].(string), args[9].(string),
	)
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Fatalf("second call: expected ErrAlreadyProcessed, got %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.calls) != 2 {
		t.Fatalf("expected 2 publish calls (one per attempt), got %d", len(pub.calls))
	}
}

func TestAddDetail_PublishErrorPropagates(t *testing.T) {
	repo := memory.NewDetailRepository()
	pub := &fakePublisher{forceErr: errors.New("kafka down")}
	svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), pub)

	_, err := svc.AddDetail(context.Background(),
		"Test", "Author", 2024, "paperback", 10, "Pub", "en", "", "9780000000002", "",
	)
	if err == nil {
		t.Fatal("expected publish error to propagate, got nil")
	}
}
```

- [ ] **Step 2: Run tests — expect compile fail (constructor signature mismatch)**

Run: `go test ./services/details/internal/core/service/... -v`

Expected: compile error `too many arguments in call to service.NewDetailService`.

- [ ] **Step 3: Update `DetailService` to accept and use publisher**

Replace `services/details/internal/core/service/detail_service.go` with:
```go
// Package service implements the business logic for the details service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// DetailService implements the port.DetailService interface.
type DetailService struct {
	repo        port.DetailRepository
	idempotency idempotency.Store
	publisher   port.EventPublisher
}

// NewDetailService creates a new DetailService with the given repository, idempotency store, and event publisher.
func NewDetailService(repo port.DetailRepository, idem idempotency.Store, publisher port.EventPublisher) *DetailService {
	return &DetailService{repo: repo, idempotency: idem, publisher: publisher}
}

// GetDetail returns a book detail by ID.
func (s *DetailService) GetDetail(ctx context.Context, id string) (*domain.Detail, error) {
	detail, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding detail %s: %w", id, err)
	}
	return detail, nil
}

// AddDetail creates and persists a new book detail, then publishes a BookAddedEvent.
// Always publishes (even on idempotency dedup) so that a retry after a Kafka blip still delivers the event.
func (s *DetailService) AddDetail(ctx context.Context, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13, idempotencyKey string) (*domain.Detail, error) {
	key := idempotency.Resolve(idempotencyKey, title, author, strconv.Itoa(year), bookType, strconv.Itoa(pages), publisher, language, isbn10, isbn13)

	detail, err := domain.NewDetail(title, author, year, bookType, pages, publisher, language, isbn10, isbn13)
	if err != nil {
		return nil, fmt.Errorf("creating detail: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}

	if !alreadyProcessed {
		if err := s.repo.Save(ctx, detail); err != nil {
			return nil, fmt.Errorf("saving detail: %w", err)
		}
	} else {
		logger := logging.FromContext(ctx)
		logger.Info("detail save skipped: already processed", slog.String("idempotency_key", key))
	}

	evt := domain.BookAddedEvent{
		ID:             detail.ID,
		Title:          detail.Title,
		Author:         detail.Author,
		Year:           detail.Year,
		Type:           detail.Type,
		Pages:          detail.Pages,
		Publisher:      detail.Publisher,
		Language:       detail.Language,
		ISBN10:         detail.ISBN10,
		ISBN13:         detail.ISBN13,
		IdempotencyKey: key,
	}
	if err := s.publisher.PublishBookAdded(ctx, evt); err != nil {
		return nil, fmt.Errorf("publishing book-added event: %w", err)
	}

	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}
	return detail, nil
}

// ListDetails returns all stored book details.
func (s *DetailService) ListDetails(ctx context.Context) ([]*domain.Detail, error) {
	return s.repo.FindAll(ctx)
}
```

- [ ] **Step 4: Check existing idempotency test file compiles**

Open `services/details/internal/core/service/detail_service_idempotency_test.go` and update any `NewDetailService` call to pass a `&fakePublisher{}` as the third argument. If the fake is in the main test file in the same package, it is reachable without changes. Check for `NewDetailService(...)` occurrences:

Run: `grep -n "NewDetailService(" services/details/internal/core/service/detail_service_idempotency_test.go`

If hits found, update each call site. Example:
```go
svc := service.NewDetailService(repo, idempotency.NewMemoryStore(), &fakePublisher{})
```

- [ ] **Step 5: Run tests**

Run: `go test ./services/details/internal/core/service/... -race -v`

Expected: all tests PASS. Expect `TestAddDetail_PublishesOnIdempotencyDedup` reports `2 publish calls`.

- [ ] **Step 6: Commit**

```bash
git add services/details/internal/core/service/detail_service.go \
        services/details/internal/core/service/detail_service_test.go \
        services/details/internal/core/service/detail_service_idempotency_test.go
git commit -m "feat(details): always-publish BookAddedEvent from AddDetail"
```

---

### Task 9: Wire publisher in `cmd/main.go`

**Files:**
- Modify: `services/details/cmd/main.go`

- [ ] **Step 1: Add kafka adapter import and publisher wiring**

Open `services/details/cmd/main.go`. Add import:
```go
kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/kafka"
```
And `"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"` is already imported.

Before `svc := service.NewDetailService(repo, idemStore)`, add:
```go
var publisher port.EventPublisher
if cfg.KafkaBrokers != "" {
	kafkaTopic := cfg.KafkaTopic
	if kafkaTopic == "" {
		kafkaTopic = "bookinfo_details_events"
	}
	kProd, err := kafkaadapter.NewProducer(ctx, cfg.KafkaBrokers, kafkaTopic)
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer kProd.Close()
	publisher = kProd
	logger.Info("kafka publisher enabled", "topic", kafkaTopic)
} else {
	publisher = kafkaadapter.NewNoopPublisher()
	logger.Info("kafka publisher disabled — using no-op")
}
```

Change the service constructor call:
```go
svc := service.NewDetailService(repo, idemStore, publisher)
```

- [ ] **Step 2: Run go build**

Run: `go build ./services/details/cmd/`

Expected: no output, exit 0.

- [ ] **Step 3: Run full tests for details**

Run: `go test ./services/details/... -race -count=1`

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add services/details/cmd/main.go
git commit -m "feat(details): wire Kafka publisher in composition root"
```

---

### Task 10: Update `deploy/details/values-local.yaml`

**Files:**
- Modify: `deploy/details/values-local.yaml`

- [ ] **Step 1: Add `KAFKA_*` to config block**

Update the `config:` block:
```yaml
config:
  LOG_LEVEL: "debug"
  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  KAFKA_TOPIC: "bookinfo_details_events"
```

- [ ] **Step 2: Add `events.exposed.events` block**

Below the existing `events.kafka` and `events.consumed.raw-books-details` blocks, add `events.exposed.events`. The full `events:` block should be:
```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_details_events
      eventBusName: kafka
      contentType: application/json
      eventTypes:
        - com.bookinfo.details.book-added
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

- [ ] **Step 3: Remove `notify-book-added` trigger from CQRS endpoint**

Update `cqrs.endpoints.book-added.triggers` to contain only `create-detail`:
```yaml
cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
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
```

- [ ] **Step 4: Lint + template check**

Run: `make helm-lint && make helm-template SERVICE=details | grep -A5 "details-events"`

Expected: lint passes; rendered `EventSource/details-events` contains:
```yaml
annotations:
  bookinfo.io/emitted-ce-types: "com.bookinfo.details.book-added"
```

- [ ] **Step 5: Verify no `notify-book-added` remains**

Run: `make helm-template SERVICE=details | grep notify-book-added`

Expected: zero matches.

- [ ] **Step 6: Commit**

```bash
git add deploy/details/values-local.yaml
git commit -m "feat(details): emit book-added event, drop notify trigger from CQRS sensor"
```

---

### Task 11: Add book-added consumer in notification values

**Files:**
- Modify: `deploy/notification/values-local.yaml`

- [ ] **Step 1: Add events block with book-added consumer**

Append to `deploy/notification/values-local.yaml`:
```yaml

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  consumed:
    book-added:
      eventSourceName: details-events
      eventName: events
      filter:
        ceType: com.bookinfo.details.book-added
      triggers:
        - name: notify-book-added
          url: self
          path: /v1/notifications
          method: POST
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body.title
              dest: subject
            - src:
                value: "New book added"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
```

- [ ] **Step 2: Template check for dependency + filter rendering**

Run: `make helm-template SERVICE=notification | grep -A4 "book-added-dep"`

Expected output contains:
```yaml
- name: book-added-dep
  eventSourceName: details-events
  eventName: events
  filters:
    context:
      type: "com.bookinfo.details.book-added"
```

- [ ] **Step 3: Commit**

```bash
git add deploy/notification/values-local.yaml
git commit -m "feat(notification): consume book-added events via details-events EventSource"
```

---

## Phase 3 — Reviews service producer

### Task 12: Add domain events and port for reviews

**Files:**
- Create: `services/reviews/internal/core/domain/event.go`
- Create: `services/reviews/internal/core/port/publisher.go`

- [ ] **Step 1: Create `event.go`**

```go
// file: services/reviews/internal/core/domain/event.go
package domain

// ReviewSubmittedEvent is emitted after a successful SubmitReview.
type ReviewSubmittedEvent struct {
	ID             string
	ProductID      string
	Reviewer       string
	Text           string
	IdempotencyKey string
}

// ReviewDeletedEvent is emitted after a successful DeleteReview.
type ReviewDeletedEvent struct {
	ReviewID       string
	ProductID      string
	IdempotencyKey string
}
```

- [ ] **Step 2: Create `publisher.go`**

```go
// file: services/reviews/internal/core/port/publisher.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// EventPublisher is the outbound port for emitting review domain events.
type EventPublisher interface {
	PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error
	PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error
}
```

- [ ] **Step 3: Compile check + commit**

Run: `go build ./services/reviews/...`

```bash
git add services/reviews/internal/core/domain/event.go services/reviews/internal/core/port/publisher.go
git commit -m "feat(reviews): add ReviewSubmitted/Deleted domain events and EventPublisher port"
```

---

### Task 13: Reviews Kafka producer — TDD

**Files:**
- Create: `services/reviews/internal/adapter/outbound/kafka/producer_test.go`
- Create: `services/reviews/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Write failing test**

```go
// file: services/reviews/internal/adapter/outbound/kafka/producer_test.go
package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
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

func TestPublishReviewSubmitted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	evt := domain.ReviewSubmittedEvent{
		ID: "rev_1", ProductID: "prod-42", Reviewer: "alice", Text: "Great",
		IdempotencyKey: "rev-idem-1",
	}
	if err := p.PublishReviewSubmitted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fc.records))
	}
	r := fc.records[0]
	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.reviews.review-submitted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	if headers["ce_source"] != "reviews" {
		t.Errorf("ce_source = %q", headers["ce_source"])
	}
	var body map[string]interface{}
	if err := json.Unmarshal(r.Value, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["product_id"] != "prod-42" {
		t.Errorf("body product_id = %v, want %q", body["product_id"], "prod-42")
	}
	if body["reviewer"] != "alice" {
		t.Errorf("body reviewer = %v", body["reviewer"])
	}
}

func TestPublishReviewDeleted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	evt := domain.ReviewDeletedEvent{ReviewID: "rev_99", ProductID: "prod-42", IdempotencyKey: "del-idem-1"}
	if err := p.PublishReviewDeleted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	r := fc.records[0]
	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.reviews.review-deleted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	var body map[string]interface{}
	_ = json.Unmarshal(r.Value, &body)
	if body["review_id"] != "rev_99" {
		t.Errorf("body review_id = %v", body["review_id"])
	}
}
```

> Note: the test imports `github.com/kaio6fellipe/event-driven-bookinfo/internal/adapter/outbound/kafka` in the snippet above but the actual path is `github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/kafka`. Use the actual path.

Correct the import line:
```go
kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/kafka"
```

- [ ] **Step 2: Run test — expect compile fail**

Run: `go test ./services/reviews/internal/adapter/outbound/kafka/... -v`

Expected: `no Go files` or `undefined: kafkaadapter.NewProducerWithClient`.

- [ ] **Step 3: Implement producer**

```go
// file: services/reviews/internal/adapter/outbound/kafka/producer.go
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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

const (
	ceTypeReviewSubmitted = "com.bookinfo.reviews.review-submitted"
	ceTypeReviewDeleted   = "com.bookinfo.reviews.review-deleted"
	ceSource              = "reviews"
	ceVersion             = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

type submittedBody struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

type deletedBody struct {
	ReviewID       string `json:"review_id"`
	ProductID      string `json:"product_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer emits reviews domain events to Kafka.
type Producer struct {
	client Client
	topic  string
}

// NewProducer creates a real producer connecting to the brokers. Creates topic if missing.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	seeds := strings.Split(brokers, ",")
	client, err := kgo.NewClient(kgo.SeedBrokers(seeds...), kgo.DefaultProduceTopic(topic))
	if err != nil {
		return nil, fmt.Errorf("creating Kafka client: %w", err)
	}
	if err := ensureTopic(ctx, client, topic); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring topic %q: %w", topic, err)
	}
	return &Producer{client: client, topic: topic}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for tests).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishReviewSubmitted sends a review-submitted CloudEvent to Kafka.
func (p *Producer) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	body := submittedBody{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Text:           evt.Text,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.produce(ctx, ceTypeReviewSubmitted, evt.IdempotencyKey, evt.ProductID, body)
}

// PublishReviewDeleted sends a review-deleted CloudEvent to Kafka.
func (p *Producer) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	body := deletedBody{
		ReviewID:       evt.ReviewID,
		ProductID:      evt.ProductID,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.produce(ctx, ceTypeReviewDeleted, evt.IdempotencyKey, evt.ProductID, body)
}

func (p *Producer) produce(ctx context.Context, ceType, key, partitionHint string, body any) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	recordKey := []byte(partitionHint)
	if len(recordKey) == 0 {
		recordKey = []byte(key)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   recordKey,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceType)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(key)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published reviews event", "topic", p.topic, "ce_type", ceType, "idempotency_key", key)
	return nil
}

// Close flushes and closes the underlying client.
func (p *Producer) Close() { p.client.Close() }

func ensureTopic(ctx context.Context, client *kgo.Client, topic string) error {
	admin := kadm.NewClient(client)
	resp, err := admin.CreateTopics(ctx, int32(defaultPartitions), int16(defaultReplicationFactor), nil, topic)
	if err != nil {
		return fmt.Errorf("creating topic: %w", err)
	}
	for _, t := range resp.Sorted() {
		if t.Err != nil && t.ErrMessage != "" && !isTopicExistsError(t.ErrMessage) {
			return fmt.Errorf("topic %q: %s", t.Topic, t.ErrMessage)
		}
	}
	return nil
}

func isTopicExistsError(msg string) bool {
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "TOPIC_ALREADY_EXISTS")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./services/reviews/internal/adapter/outbound/kafka/... -race -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/outbound/kafka/
git commit -m "feat(reviews): add Kafka producer for review-submitted and review-deleted events"
```

---

### Task 14: No-op publisher for reviews

**Files:**
- Create: `services/reviews/internal/adapter/outbound/kafka/noop.go`

- [ ] **Step 1: Create no-op adapter**

```go
// file: services/reviews/internal/adapter/outbound/kafka/noop.go
package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher without side effects.
type NoopPublisher struct{}

// NewNoopPublisher returns a no-op publisher.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishReviewSubmitted logs at debug and returns nil.
func (n *NoopPublisher) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: review-submitted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}

// PublishReviewDeleted logs at debug and returns nil.
func (n *NoopPublisher) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: review-deleted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
```

- [ ] **Step 2: Build + commit**

Run: `go build ./services/reviews/...`

```bash
git add services/reviews/internal/adapter/outbound/kafka/noop.go
git commit -m "feat(reviews): add no-op publisher"
```

---

### Task 15: Wire publisher into `ReviewService` + make `DeleteReview` idempotent

**Files:**
- Modify: `services/reviews/internal/core/service/review_service.go`
- Find / modify / create tests to match new constructor

- [ ] **Step 1: Inspect current review tests**

Run: `ls services/reviews/internal/core/service/`

Note each `_test.go` file that calls `service.NewReviewService(...)` — all call sites need a 4th arg (the publisher).

- [ ] **Step 2: Update `review_service.go`**

Replace with:
```go
// Package service implements the business logic for the reviews service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
	idempotency   idempotency.Store
	publisher     port.EventPublisher
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient, idem idempotency.Store, publisher port.EventPublisher) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
		idempotency:   idem,
		publisher:     publisher,
	}
}

// GetProductReviews returns paginated reviews for a product, enriched with ratings data.
func (s *ReviewService) GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error) {
	offset := (page - 1) * pageSize

	reviews, total, err := s.repo.FindByProductID(ctx, productID, offset, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("finding reviews for product %s: %w", productID, err)
	}

	ratingData, err := s.ratingsClient.GetProductRatings(ctx, productID)
	if err != nil {
		logger := logging.FromContext(ctx)
		logger.Warn("failed to fetch ratings, returning reviews without ratings",
			slog.String("product_id", productID),
			slog.String("error", err.Error()),
		)
		return reviews, total, nil
	}

	for i := range reviews {
		reviews[i].Rating = &domain.ReviewRating{
			Stars:   ratingData.IndividualRatings[reviews[i].Reviewer],
			Average: ratingData.Average,
			Count:   ratingData.Count,
		}
	}

	return reviews, total, nil
}

// SubmitReview creates and persists a new review, then publishes a ReviewSubmittedEvent.
// Always publishes (even on idempotency dedup) for retry safety.
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, text)

	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}

	if !alreadyProcessed {
		if err := s.repo.Save(ctx, review); err != nil {
			return nil, fmt.Errorf("saving review: %w", err)
		}
	} else {
		logging.FromContext(ctx).Info("review submit skipped: already processed", slog.String("idempotency_key", key))
	}

	evt := domain.ReviewSubmittedEvent{
		ID:             review.ID,
		ProductID:      review.ProductID,
		Reviewer:       review.Reviewer,
		Text:           review.Text,
		IdempotencyKey: key,
	}
	if err := s.publisher.PublishReviewSubmitted(ctx, evt); err != nil {
		return nil, fmt.Errorf("publishing review-submitted event: %w", err)
	}

	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}
	return review, nil
}

// DeleteReview removes a review by its ID and publishes a ReviewDeletedEvent.
// Idempotent: if the review does not exist (already deleted), returns nil and still publishes.
// This is required for Option B retry semantics — a retry after a Kafka blip must succeed + re-publish.
func (s *ReviewService) DeleteReview(ctx context.Context, id string) error {
	productID := ""
	existing, err := s.repo.FindByID(ctx, id)
	switch {
	case err == nil:
		productID = existing.ProductID
	case errors.Is(err, domain.ErrNotFound):
		logging.FromContext(ctx).Info("delete review: already absent, treating as idempotent success", slog.String("review_id", id))
	default:
		return fmt.Errorf("looking up review %s before delete: %w", id, err)
	}

	if err := s.repo.DeleteByID(ctx, id); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}

	evt := domain.ReviewDeletedEvent{
		ReviewID:       id,
		ProductID:      productID,
		IdempotencyKey: "review-deleted-" + id,
	}
	if err := s.publisher.PublishReviewDeleted(ctx, evt); err != nil {
		return fmt.Errorf("publishing review-deleted event: %w", err)
	}

	return nil
}
```

> NOTE: this change assumes `ReviewRepository` has a `FindByID` method. If it doesn't, fall back to directly calling `repo.DeleteByID` and setting `productID = ""` — the notification consumer sensor maps `subject: body.product_id`, so an empty product_id on a retry-delete is acceptable for the notification content.

- [ ] **Step 3: Check repo interface**

Run: `grep -n "FindByID" services/reviews/internal/core/port/outbound.go`

If no match, either:
- Drop the `FindByID` call and rely solely on `DeleteByID` returning success/ErrNotFound.
- Or add the method to the interface + memory + postgres implementations.

For simplicity, if FindByID is absent, use this minimal variant for `DeleteReview`:
```go
func (s *ReviewService) DeleteReview(ctx context.Context, id string) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}

	evt := domain.ReviewDeletedEvent{
		ReviewID:       id,
		IdempotencyKey: "review-deleted-" + id,
	}
	if err := s.publisher.PublishReviewDeleted(ctx, evt); err != nil {
		return fmt.Errorf("publishing review-deleted event: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Update handler if it treats ErrNotFound as 404**

Open `services/reviews/internal/adapter/inbound/http/handler.go` and change `deleteReview` so that ErrNotFound is no longer returned as 404 (it can't happen now). Leave the handler unchanged if only `fmt.Errorf` errors flow through — keep only the `http.StatusInternalServerError` branch plus the 204 success:
```go
func (h *Handler) deleteReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req struct {
		ReviewID string `json:"review_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ReviewID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "review_id is required"})
		return
	}

	if err := h.svc.DeleteReview(r.Context(), req.ReviewID); err != nil {
		logger.Error("failed to delete review", "error", err, "review_id", req.ReviewID)
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		return
	}

	logger.Info("review deleted", "review_id", req.ReviewID)
	w.WriteHeader(http.StatusNoContent)
}
```

Remove the unused `domain` import if no longer referenced.

- [ ] **Step 5: Update all `NewReviewService` call sites in tests**

Find them:
```bash
grep -rn "NewReviewService(" services/reviews/
```

For each test file, add `&fakeReviewPublisher{}` (define a local fake matching `port.EventPublisher`) as the fourth argument. Example fake to add in each test file (or a shared helpers file at `services/reviews/internal/core/service/helpers_test.go`):

```go
// helpers_test.go in services/reviews/internal/core/service/
package service_test

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

type fakeReviewPublisher struct {
	mu        sync.Mutex
	submitted []domain.ReviewSubmittedEvent
	deleted   []domain.ReviewDeletedEvent
	forceErr  error
}

func (f *fakeReviewPublisher) PublishReviewSubmitted(_ context.Context, evt domain.ReviewSubmittedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, evt)
	return f.forceErr
}

func (f *fakeReviewPublisher) PublishReviewDeleted(_ context.Context, evt domain.ReviewDeletedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, evt)
	return f.forceErr
}
```

Then update constructors, e.g. `svc := service.NewReviewService(repo, ratingsClient, idempotency.NewMemoryStore(), &fakeReviewPublisher{})`.

- [ ] **Step 6: Add a test for DeleteReview publishing**

In the existing review_service_test.go (or a new one), add:
```go
func TestDeleteReview_PublishesEvent(t *testing.T) {
	repo := memory.NewReviewRepository()
	pub := &fakeReviewPublisher{}
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore(), pub)

	// Even deleting a non-existent review succeeds + publishes.
	if err := svc.DeleteReview(context.Background(), "rev_missing"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.deleted) != 1 {
		t.Fatalf("expected 1 deleted publish, got %d", len(pub.deleted))
	}
	if pub.deleted[0].ReviewID != "rev_missing" {
		t.Errorf("ReviewID = %q", pub.deleted[0].ReviewID)
	}
}
```

> If `memory.NewReviewRepository()` doesn't exist in the current codebase, use whatever fake the other tests use.

- [ ] **Step 7: Run tests**

Run: `go test ./services/reviews/... -race -count=1`

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add services/reviews/internal/core/service/ services/reviews/internal/adapter/inbound/http/handler.go
git commit -m "feat(reviews): always-publish review-submitted/deleted, idempotent DeleteReview"
```

---

### Task 16: Wire publisher in reviews `cmd/main.go`

**Files:**
- Modify: `services/reviews/cmd/main.go`

- [ ] **Step 1: Add imports**

```go
kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/kafka"
```

Ensure `port` is imported — it already is.

- [ ] **Step 2: Add publisher wiring before `svc := service.NewReviewService(...)`**

```go
var publisher port.EventPublisher
if cfg.KafkaBrokers != "" {
	topic := cfg.KafkaTopic
	if topic == "" {
		topic = "bookinfo_reviews_events"
	}
	kProd, err := kafkaadapter.NewProducer(ctx, cfg.KafkaBrokers, topic)
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer kProd.Close()
	publisher = kProd
	logger.Info("kafka publisher enabled", "topic", topic)
} else {
	publisher = kafkaadapter.NewNoopPublisher()
	logger.Info("kafka publisher disabled — using no-op")
}

svc := service.NewReviewService(repo, ratingsClient, idemStore, publisher)
```

- [ ] **Step 3: Build + test**

Run: `go build ./services/reviews/cmd/ && go test ./services/reviews/... -race -count=1`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add services/reviews/cmd/main.go
git commit -m "feat(reviews): wire Kafka publisher in composition root"
```

---

### Task 17: Reviews Helm values — emit events, drop notify triggers

**Files:**
- Modify: `deploy/reviews/values-local.yaml`

- [ ] **Step 1: Update `config:` with Kafka env vars**

```yaml
config:
  LOG_LEVEL: "debug"
  RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"
  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  KAFKA_TOPIC: "bookinfo_reviews_events"
```

- [ ] **Step 2: Add `events.exposed` block**

Add (alongside existing `cqrs:` and `sensor:` blocks):
```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_reviews_events
      eventBusName: kafka
      contentType: application/json
      eventTypes:
        - com.bookinfo.reviews.review-submitted
        - com.bookinfo.reviews.review-deleted
```

- [ ] **Step 3: Remove `notify-review-submitted` and `notify-review-deleted` from CQRS endpoints**

Replace the entire `cqrs.endpoints:` block with:
```yaml
  endpoints:
    review-submitted:
      port: 12001
      method: POST
      endpoint: /v1/reviews
      triggers:
        - name: create-review
          url: self
          payload:
            - passthrough
    review-deleted:
      port: 12003
      method: POST
      endpoint: /v1/reviews/delete
      triggers:
        - name: delete-review-write
          url: self
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
```

- [ ] **Step 4: Verify rendering**

```bash
make helm-lint
make helm-template SERVICE=reviews | grep notify-review
```

Expected: lint passes. Second command returns **zero** matches.

Run: `make helm-template SERVICE=reviews | grep -A3 "reviews-events"`

Expected: `reviews-events` EventSource annotation lists both review-submitted and review-deleted types.

- [ ] **Step 5: Commit**

```bash
git add deploy/reviews/values-local.yaml
git commit -m "feat(reviews): emit review events, drop notify-* from CQRS sensor"
```

---

### Task 18: Notification consumer — add reviews subscriptions

**Files:**
- Modify: `deploy/notification/values-local.yaml`

- [ ] **Step 1: Append review-submitted and review-deleted consumers**

Append to the `events.consumed:` block (created in Task 11):
```yaml
    review-submitted:
      eventSourceName: reviews-events
      eventName: events
      filter:
        ceType: com.bookinfo.reviews.review-submitted
      triggers:
        - name: notify-review-submitted
          url: self
          path: /v1/notifications
          method: POST
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New review submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
    review-deleted:
      eventSourceName: reviews-events
      eventName: events
      filter:
        ceType: com.bookinfo.reviews.review-deleted
      triggers:
        - name: notify-review-deleted
          url: self
          path: /v1/notifications
          method: POST
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "Review deleted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
```

- [ ] **Step 2: Template check**

```bash
make helm-template SERVICE=notification | grep -E "review-submitted-dep|review-deleted-dep" | head -10
```

Expected: each appears twice (once as dep, once in trigger). Filters include `com.bookinfo.reviews.review-submitted` / `.review-deleted`.

- [ ] **Step 3: Commit**

```bash
git add deploy/notification/values-local.yaml
git commit -m "feat(notification): consume review-submitted and review-deleted events"
```

---

## Phase 4 — Ratings service producer

Mirrors details; single event.

### Task 19: Ratings domain event + publisher port

**Files:**
- Create: `services/ratings/internal/core/domain/event.go`
- Create: `services/ratings/internal/core/port/publisher.go`

- [ ] **Step 1: Create `event.go`**

```go
// file: services/ratings/internal/core/domain/event.go
package domain

// RatingSubmittedEvent is emitted after a successful SubmitRating.
type RatingSubmittedEvent struct {
	ID             string
	ProductID      string
	Reviewer       string
	Stars          int
	IdempotencyKey string
}
```

- [ ] **Step 2: Create `publisher.go`**

```go
// file: services/ratings/internal/core/port/publisher.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// EventPublisher is the outbound port for emitting rating domain events.
type EventPublisher interface {
	PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error
}
```

- [ ] **Step 3: Build + commit**

Run: `go build ./services/ratings/...`

```bash
git add services/ratings/internal/core/domain/event.go services/ratings/internal/core/port/publisher.go
git commit -m "feat(ratings): add RatingSubmittedEvent domain type and EventPublisher port"
```

---

### Task 20: Ratings Kafka producer — TDD

**Files:**
- Create: `services/ratings/internal/adapter/outbound/kafka/producer_test.go`
- Create: `services/ratings/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Write test**

```go
// file: services/ratings/internal/adapter/outbound/kafka/producer_test.go
package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
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

func TestPublishRatingSubmitted(t *testing.T) {
	t.Parallel()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_ratings_events")

	evt := domain.RatingSubmittedEvent{
		ID: "rat_1", ProductID: "prod-42", Reviewer: "alice", Stars: 5,
		IdempotencyKey: "rat-idem-1",
	}
	if err := p.PublishRatingSubmitted(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fc.records))
	}
	r := fc.records[0]

	headers := map[string]string{}
	for _, h := range r.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["ce_type"] != "com.bookinfo.ratings.rating-submitted" {
		t.Errorf("ce_type = %q", headers["ce_type"])
	}
	if headers["ce_source"] != "ratings" {
		t.Errorf("ce_source = %q", headers["ce_source"])
	}

	var body map[string]interface{}
	_ = json.Unmarshal(r.Value, &body)
	if body["product_id"] != "prod-42" {
		t.Errorf("product_id = %v", body["product_id"])
	}
	if body["stars"].(float64) != 5 {
		t.Errorf("stars = %v", body["stars"])
	}
}
```

- [ ] **Step 2: Implement producer**

```go
// file: services/ratings/internal/adapter/outbound/kafka/producer.go
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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

const (
	ceTypeRatingSubmitted = "com.bookinfo.ratings.rating-submitted"
	ceSource              = "ratings"
	ceVersion             = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

type submittedBody struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer emits ratings domain events to Kafka.
type Producer struct {
	client Client
	topic  string
}

// NewProducer creates a real producer connecting to the brokers. Creates topic if missing.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	seeds := strings.Split(brokers, ",")
	client, err := kgo.NewClient(kgo.SeedBrokers(seeds...), kgo.DefaultProduceTopic(topic))
	if err != nil {
		return nil, fmt.Errorf("creating Kafka client: %w", err)
	}
	if err := ensureTopic(ctx, client, topic); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring topic %q: %w", topic, err)
	}
	return &Producer{client: client, topic: topic}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for tests).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishRatingSubmitted sends a rating-submitted CloudEvent to Kafka.
func (p *Producer) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	logger := logging.FromContext(ctx)

	body := submittedBody{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Stars:          evt.Stars,
		IdempotencyKey: evt.IdempotencyKey,
	}
	value, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling rating-submitted event: %w", err)
	}

	recordKey := []byte(evt.ProductID)
	if len(recordKey) == 0 {
		recordKey = []byte(evt.IdempotencyKey)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   recordKey,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceTypeRatingSubmitted)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(evt.IdempotencyKey)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published rating-submitted event", "topic", p.topic, "idempotency_key", evt.IdempotencyKey)
	return nil
}

// Close flushes pending messages and closes the Kafka client.
func (p *Producer) Close() { p.client.Close() }

func ensureTopic(ctx context.Context, client *kgo.Client, topic string) error {
	admin := kadm.NewClient(client)
	resp, err := admin.CreateTopics(ctx, int32(defaultPartitions), int16(defaultReplicationFactor), nil, topic)
	if err != nil {
		return fmt.Errorf("creating topic: %w", err)
	}
	for _, t := range resp.Sorted() {
		if t.Err != nil && t.ErrMessage != "" && !isTopicExistsError(t.ErrMessage) {
			return fmt.Errorf("topic %q: %s", t.Topic, t.ErrMessage)
		}
	}
	return nil
}

func isTopicExistsError(msg string) bool {
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "TOPIC_ALREADY_EXISTS")
}
```

- [ ] **Step 3: Tests pass + commit**

Run: `go test ./services/ratings/internal/adapter/outbound/kafka/... -race -v`

Expected: PASS.

```bash
git add services/ratings/internal/adapter/outbound/kafka/
git commit -m "feat(ratings): add Kafka producer for rating-submitted event"
```

---

### Task 21: Ratings no-op publisher + service wiring

**Files:**
- Create: `services/ratings/internal/adapter/outbound/kafka/noop.go`
- Modify: `services/ratings/internal/core/service/rating_service.go`
- Modify: `services/ratings/internal/core/service/rating_service_test.go` (and any sibling test files)
- Modify: `services/ratings/cmd/main.go`

- [ ] **Step 1: Create noop**

```go
// file: services/ratings/internal/adapter/outbound/kafka/noop.go
package kafka

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// NoopPublisher implements port.EventPublisher without side effects.
type NoopPublisher struct{}

// NewNoopPublisher returns a no-op publisher.
func NewNoopPublisher() *NoopPublisher { return &NoopPublisher{} }

// PublishRatingSubmitted logs at debug and returns nil.
func (n *NoopPublisher) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	logging.FromContext(ctx).Debug("noop publisher: rating-submitted discarded", "idempotency_key", evt.IdempotencyKey)
	return nil
}
```

- [ ] **Step 2: Update `rating_service.go` with always-publish semantics**

Replace body with:
```go
// Package service implements the business logic for the ratings service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// RatingService implements the port.RatingService interface.
type RatingService struct {
	repo        port.RatingRepository
	idempotency idempotency.Store
	publisher   port.EventPublisher
}

// NewRatingService creates a new RatingService with the given repository, idempotency store, and event publisher.
func NewRatingService(repo port.RatingRepository, idem idempotency.Store, publisher port.EventPublisher) *RatingService {
	return &RatingService{repo: repo, idempotency: idem, publisher: publisher}
}

// GetProductRatings returns all ratings aggregated for a product.
func (s *RatingService) GetProductRatings(ctx context.Context, productID string) (*domain.ProductRatings, error) {
	ratings, err := s.repo.FindByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("finding ratings for product %s: %w", productID, err)
	}
	return &domain.ProductRatings{ProductID: productID, Ratings: ratings}, nil
}

// SubmitRating creates and persists a new rating, then publishes a RatingSubmittedEvent.
// Always publishes (even on idempotency dedup) for retry safety.
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int, idempotencyKey string) (*domain.Rating, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, strconv.Itoa(stars))

	rating, err := domain.NewRating(productID, reviewer, stars)
	if err != nil {
		return nil, fmt.Errorf("creating rating: %w", err)
	}

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}

	if !alreadyProcessed {
		if err := s.repo.Save(ctx, rating); err != nil {
			return nil, fmt.Errorf("saving rating: %w", err)
		}
	} else {
		logging.FromContext(ctx).Info("rating submit skipped: already processed", slog.String("idempotency_key", key))
	}

	evt := domain.RatingSubmittedEvent{
		ID:             rating.ID,
		ProductID:      rating.ProductID,
		Reviewer:       rating.Reviewer,
		Stars:          rating.Stars,
		IdempotencyKey: key,
	}
	if err := s.publisher.PublishRatingSubmitted(ctx, evt); err != nil {
		return nil, fmt.Errorf("publishing rating-submitted event: %w", err)
	}

	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}
	return rating, nil
}
```

- [ ] **Step 3: Update ratings tests**

Find `NewRatingService(` usages:
```bash
grep -rn "NewRatingService(" services/ratings/
```

In each `_test.go` file, add a fake publisher:
```go
type fakeRatingPublisher struct {
	mu    sync.Mutex
	calls []domain.RatingSubmittedEvent
}

func (f *fakeRatingPublisher) PublishRatingSubmitted(_ context.Context, evt domain.RatingSubmittedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, evt)
	return nil
}
```

And update constructor calls to `service.NewRatingService(repo, idempotency.NewMemoryStore(), &fakeRatingPublisher{})`.

- [ ] **Step 4: Update `ratings/cmd/main.go`**

Add imports:
```go
kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/kafka"
```

Before `svc := service.NewRatingService(repo, idemStore)`:
```go
var publisher port.EventPublisher
if cfg.KafkaBrokers != "" {
	topic := cfg.KafkaTopic
	if topic == "" {
		topic = "bookinfo_ratings_events"
	}
	kProd, err := kafkaadapter.NewProducer(ctx, cfg.KafkaBrokers, topic)
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer kProd.Close()
	publisher = kProd
	logger.Info("kafka publisher enabled", "topic", topic)
} else {
	publisher = kafkaadapter.NewNoopPublisher()
	logger.Info("kafka publisher disabled — using no-op")
}

svc := service.NewRatingService(repo, idemStore, publisher)
```

- [ ] **Step 5: Build + tests**

Run: `go build ./services/ratings/cmd/ && go test ./services/ratings/... -race -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/ratings/
git commit -m "feat(ratings): always-publish rating-submitted event + no-op publisher + composition root wiring"
```

---

### Task 22: Ratings Helm values — emit events, drop notify trigger

**Files:**
- Modify: `deploy/ratings/values-local.yaml`

- [ ] **Step 1: Update config + add events.exposed + drop notify-rating-submitted**

Full file after edit:
```yaml
# deploy/ratings/values-local.yaml
serviceName: ratings
fullnameOverride: ratings
image:
  repository: event-driven-bookinfo/ratings
  tag: local

postgresql:
  enabled: true
  auth:
    database: "bookinfo_ratings"

config:
  LOG_LEVEL: "debug"
  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  KAFKA_TOPIC: "bookinfo_ratings_events"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_ratings_events
      eventBusName: kafka
      contentType: application/json
      eventTypes:
        - com.bookinfo.ratings.rating-submitted

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  endpoints:
    rating-submitted:
      port: 12002
      method: POST
      endpoint: /v1/ratings
      triggers:
        - name: create-rating
          url: self
          payload:
            - passthrough

sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"

gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

- [ ] **Step 2: Verify lint + rendering**

```bash
make helm-lint
make helm-template SERVICE=ratings | grep -E "notify-rating|ratings-events" | head
```

Expected: no `notify-rating` match; `ratings-events` EventSource present with `com.bookinfo.ratings.rating-submitted` annotation.

- [ ] **Step 3: Commit**

```bash
git add deploy/ratings/values-local.yaml
git commit -m "feat(ratings): emit rating-submitted event, drop notify trigger from CQRS sensor"
```

---

### Task 23: Notification consumer — add rating-submitted

**Files:**
- Modify: `deploy/notification/values-local.yaml`

- [ ] **Step 1: Append rating-submitted entry**

Append under `events.consumed:` (same block that now contains book-added + review-submitted + review-deleted):
```yaml
    rating-submitted:
      eventSourceName: ratings-events
      eventName: events
      filter:
        ceType: com.bookinfo.ratings.rating-submitted
      triggers:
        - name: notify-rating-submitted
          url: self
          path: /v1/notifications
          method: POST
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New rating submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
```

- [ ] **Step 2: Final rendering check — all four dependencies exist with filters**

Run:
```bash
make helm-template SERVICE=notification | grep -E "^\s+- name: .+-dep" | sort -u
```

Expected (exactly these four):
```
    - name: book-added-dep
    - name: rating-submitted-dep
    - name: review-deleted-dep
    - name: review-submitted-dep
```

Run:
```bash
make helm-template SERVICE=notification | grep "context:" -A1 | grep "type:"
```

Expected: four `type:` lines, one per ce_type.

- [ ] **Step 3: Commit**

```bash
git add deploy/notification/values-local.yaml
git commit -m "feat(notification): consume rating-submitted events via ratings-events EventSource"
```

---

## Phase 5 — End-to-end validation

### Task 24: Deploy to local k8s and verify rollout

- [ ] **Step 1: Tear down any previous cluster**

Run: `make stop-k8s`

Expected: `k3d` cluster `bookinfo-local` deleted (or "not found" if already gone).

- [ ] **Step 2: Full deploy**

Run: `make run-k8s`

Expected: `k3d` cluster created, platform (Kafka, Argo Events) installed, observability stack installed, app namespace `bookinfo` deployed. Takes ~5-10 min.

- [ ] **Step 3: Status check**

Run: `make k8s-status`

Expected: all Deployments `Ready`, no CrashLoopBackOff, access URLs printed.

- [ ] **Step 4: EventSource rollout**

Run:
```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get eventsource
```

Expected: `ingestion-raw-books-details`, `details-events`, `reviews-events`, `ratings-events` all present, age > 0s.

- [ ] **Step 5: Verify emitted-ce-types annotations**

Run:
```bash
for svc in details-events reviews-events ratings-events; do
  echo "-- $svc --"
  kubectl --context=k3d-bookinfo-local -n bookinfo get eventsource $svc -o jsonpath='{.metadata.annotations.bookinfo\.io/emitted-ce-types}'
  echo
done
```

Expected:
- `details-events`: `com.bookinfo.details.book-added`
- `reviews-events`: `com.bookinfo.reviews.review-submitted,com.bookinfo.reviews.review-deleted`
- `ratings-events`: `com.bookinfo.ratings.rating-submitted`

- [ ] **Step 6: Sensor dependencies + filters**

Run:
```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get sensor notification-consumer-sensor -o yaml | grep -A2 "type:"
```

Expected: four `type:` lines matching the four ce_types.

- [ ] **Step 7: Commit deployment verification evidence (optional)**

No commit needed — this is observation only.

---

### Task 25: Smoke tests for all four event paths

For each event, POST to the gateway and verify: entity persisted, Kafka event emitted, notification stored with matching subject.

- [ ] **Step 1: book-added**

```bash
BOOK_PAYLOAD='{"title":"Smoke Book","author":"Test Author","year":2026,"type":"paperback","pages":100,"publisher":"SP","language":"en","isbn_13":"9780000000001"}'
curl -sS -X POST http://localhost:8080/v1/details -H 'Content-Type: application/json' -d "$BOOK_PAYLOAD"
```

Verify detail:
```bash
curl -sS http://localhost:8080/v1/details | jq '.[] | select(.title=="Smoke Book")'
```
Expected: JSON record present.

Verify Kafka event:
```bash
kubectl --context=k3d-bookinfo-local -n platform exec bookinfo-kafka-cluster-kafka-0 -- \
  bin/kafka-console-consumer.sh --bootstrap-server localhost:9092 \
  --topic bookinfo_details_events --from-beginning --max-messages 1 --timeout-ms 10000 --property print.headers=true
```
Expected: one record with header `ce_type:com.bookinfo.details.book-added` and JSON body containing `"title":"Smoke Book"`.

Verify notification:
```bash
curl -sS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '.[] | select(.subject=="Smoke Book")'
```
Expected: non-empty notification record with `body: "New book added"`.

- [ ] **Step 2: review-submitted**

```bash
REV_PAYLOAD='{"product_id":"smoke-prod-1","reviewer":"smoke-user","text":"Great"}'
curl -sS -X POST http://localhost:8080/v1/reviews -H 'Content-Type: application/json' -d "$REV_PAYLOAD"
```

Verify notification:
```bash
curl -sS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '.[] | select(.subject=="smoke-prod-1" and .body=="New review submitted")'
```
Expected: non-empty.

- [ ] **Step 3: review-deleted**

Capture review_id from step 2 (or list reviews):
```bash
REVIEW_ID=$(curl -sS http://localhost:8080/v1/reviews/smoke-prod-1 | jq -r '.reviews[0].id')
curl -sS -X POST http://localhost:8080/v1/reviews/delete -H 'Content-Type: application/json' -d "{\"review_id\":\"$REVIEW_ID\"}"
```

Verify:
```bash
curl -sS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '.[] | select(.body=="Review deleted")'
```
Expected: non-empty.

- [ ] **Step 4: rating-submitted**

```bash
RAT_PAYLOAD='{"product_id":"smoke-prod-1","reviewer":"smoke-user","stars":5}'
curl -sS -X POST http://localhost:8080/v1/ratings -H 'Content-Type: application/json' -d "$RAT_PAYLOAD"
```

Verify:
```bash
curl -sS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '.[] | select(.body=="New rating submitted")'
```
Expected: non-empty.

- [ ] **Step 5: Record success, no commit needed**

If any step fails, tail logs for the corresponding `-write` service:
```bash
kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/details-write --tail=50
```

---

### Task 26: Failure-recovery smoke test (Kafka blip)

- [ ] **Step 1: Scale Kafka down**

```bash
kubectl --context=k3d-bookinfo-local -n platform scale strimzipodset bookinfo-kafka-cluster-kafka --replicas=0
kubectl --context=k3d-bookinfo-local -n platform wait --for=delete pod -l strimzi.io/name=bookinfo-kafka-cluster-kafka --timeout=60s || true
```

- [ ] **Step 2: POST a book — expect 500**

```bash
curl -i -sS -X POST http://localhost:8080/v1/details -H 'Content-Type: application/json' \
  -d '{"title":"Blip Test","author":"X","year":2026,"type":"paperback","isbn_13":"9780000000099"}'
```

Expected: HTTP 5xx from `details-write` (visible in the curl output) AND sensor retrying (`kubectl -n bookinfo describe sensor details-sensor`).

- [ ] **Step 3: Scale Kafka back up**

```bash
kubectl --context=k3d-bookinfo-local -n platform scale strimzipodset bookinfo-kafka-cluster-kafka --replicas=1
kubectl --context=k3d-bookinfo-local -n platform wait --for=condition=ready pod -l strimzi.io/name=bookinfo-kafka-cluster-kafka --timeout=120s
```

- [ ] **Step 4: Within retry window — verify exactly-once delivery**

Wait ~30s for sensor retries to reach the now-healthy service, then:
```bash
curl -sS http://localhost:8080/v1/details | jq '[.[] | select(.title=="Blip Test")] | length'
```
Expected: `1` (idempotency dedupe prevents duplicates).

```bash
curl -sS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '[.[] | select(.subject=="Blip Test")] | length'
```
Expected: `1` (notification natural-key dedupe prevents duplicates).

---

### Task 27: k6 load run + Tempo trace verification

- [ ] **Step 1: Run k6 load for 2 minutes**

```bash
DURATION=2m BASE_RATE=5 make k8s-load
```

Expected: docker-based k6 container drives traffic through the gateway. Prints a summary at the end with checks passed / failed.

- [ ] **Step 2: Inspect Grafana traces**

Open in a browser: `http://localhost:3000` → sign in (default admin/admin) → Explore → select the `Tempo` data source → query:

```
{ service.name = "details-write" && resource.service.name = "details-write" }
```
(or using `resource.service.name="details-write"` per Tempo TraceQL support)

Pick any recent span from a POST request.

- [ ] **Step 3: Verify full trace chain**

In the trace timeline, confirm spans from these services appear in order:
1. `envoy-gateway`
2. `details-eventsource` (CQRS EventSource)
3. `details-cqrs-sensor`
4. `details-write` (HTTP handler + DB save + **kafka producer span**)
5. `details-events` (Kafka EventSource consumer side)
6. `notification-consumer-sensor`
7. `notification-write` (HTTP handler + DB save)

Also confirm Kafka span attributes:
- `messaging.system = kafka`
- `messaging.destination.name = bookinfo_details_events`
- `messaging.kafka.message.key = <idempotency key>`

- [ ] **Step 4: Repeat spot-check for a review and a rating trace**

Filter Tempo by `resource.service.name = "reviews-write"` and `"ratings-events"` — confirm similar chains end at `notification-write`.

- [ ] **Step 5: No commit — validation only**

If trace chain is broken at any hop, check:
- Alloy / OpenTelemetry Collector logs in `observability` namespace
- Verify `OTEL_EXPORTER_OTLP_ENDPOINT` env var set on each service
- Compare to ingestion's producer trace (known-working baseline)

---

### Task 28: Final full sweep — lint, test, helm-lint

- [ ] **Step 1: Go lint + test**

```bash
make lint
make test
```

Expected: both pass with no errors.

- [ ] **Step 2: Helm lint**

```bash
make helm-lint
```

Expected: `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 3: Grep sweep for any remaining notify-* triggers**

```bash
grep -rn "notify-" deploy/details/values-local.yaml deploy/reviews/values-local.yaml deploy/ratings/values-local.yaml
```

Expected: zero matches.

- [ ] **Step 4: Acceptance criteria review**

Re-read `docs/superpowers/specs/2026-04-24-event-driven-notifications-design.md` § Acceptance criteria. For each numbered criterion, verify it holds. If all 7 pass, feature complete.

- [ ] **Step 5: Commit any final polish**

If lint or test adjustments were needed in this phase, commit them:
```bash
git add -A
git commit -m "chore: final polish for event-driven notifications"
```

---

## Self-Review

1. **Spec coverage:**
   - Decision 1 (all four events) → Tasks 9, 15, 21 publish-wiring + Tasks 10, 17, 22 values ✔
   - Decision 2 (return 500, sensor retries) → Task 8/15/21 service layer returns publish error ✔
   - Decision 3 (full entity + CE headers) → Tasks 6, 13, 20 ✔
   - Decision 4 (one topic per service, ce_type carried) → Task 17 (reviews carries 2), Tasks 10, 22 (1 each) ✔
   - Decision 5 (always-publish) → Service-level tests (Task 8) assert publish on idem dedup ✔
   - Decision 6 (chart unchanged naming) → No template prefix changes ✔
   - Decision 7 (ce_type contract annotation) → Task 1 chart template + eventTypes in each values file ✔
   - Component: notification consumer sensor filter → Task 2 template, Tasks 11/18/23 values ✔
   - Removed notify-* triggers → Tasks 10, 17, 22 ✔
   - Error handling cases (DB error, Kafka unset, topic missing, DLQ, dedupe) → Covered by Tasks 6-8, 13-15, 20-21 code + Phase 5 validation ✔
   - Testing (unit, helm, e2e, k6, Tempo) → Phases 4 (helm template checks per Helm task) + Phase 5 tasks ✔
   - Acceptance criteria 1-7 → Task 28 sweep ✔

2. **Placeholder scan:** No "TBD", "TODO", "similar to", or "implement later" remain. The repo method uncertainty in Task 15 step 3 is handled with a concrete alternative implementation.

3. **Type consistency:**
   - `port.EventPublisher` defined with `PublishBookAdded` (details), `PublishReviewSubmitted/Deleted` (reviews), `PublishRatingSubmitted` (ratings) — matches adapter method names and test assertions throughout.
   - Domain event types match JSON body field names (`product_id`, `isbn_13`, `idempotency_key`, etc).
   - Chart `events.consumed.<name>.filter.ceType` matches template path `filters.context.type`.
