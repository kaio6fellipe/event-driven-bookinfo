# NATS JetStream EventBus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add NATS JetStream as an alternative EventBus alongside Kafka, selectable at `make run-k8s eventbus=jetstream` time, deployed to a separate k3d cluster, with all producer services able to publish to either backend via runtime env vars.

**Architecture:** Introduce a `Publisher` interface (`pkg/eventsmessaging`) with two implementations (`kafkapub`, `natspub`). Each service's `cmd/main.go` selects backend via `EVENT_BACKEND` env. The Helm chart gains `events.bus.type` to render either a kafka or a jetstream EventSource. Two clusters (`bookinfo-kafka-local`, `bookinfo-jetstream-local`) are mutually exclusive; the Makefile drives platform install + values-file selection via an `EVENTBUS` variable. AsyncAPI specs grow a second server entry. Spec source: `docs/superpowers/specs/2026-04-29-jetstream-eventbus-design.md` (sha `124254b`).

**Tech Stack:** Go 1.22+, `nats.go` JetStream SDK, franz-go (existing Kafka), Argo Events v1.9+ (with custom CRDs from `argoproj/argo-events#3961`+`#3983`), Helm 3, k3d, NATS server (Helm chart `nats-io/nats`), Strimzi (existing).

---

## File Structure Map

### New files

```
pkg/eventsmessaging/publisher.go
pkg/eventsmessaging/publisher_test.go
pkg/eventsmessaging/natspub/producer.go
pkg/eventsmessaging/natspub/producer_test.go
pkg/telemetry/nats.go
pkg/telemetry/nats_test.go

charts/bookinfo-service/templates/jetstream-eventsource.yaml
charts/bookinfo-service/ci/values-details-jetstream.yaml
charts/bookinfo-service/ci/values-reviews-jetstream.yaml
charts/bookinfo-service/ci/values-ratings-jetstream.yaml
charts/bookinfo-service/ci/values-ingestion-jetstream.yaml
charts/bookinfo-service/ci/values-notification-jetstream.yaml
charts/bookinfo-service/ci/values-dlqueue-jetstream.yaml
charts/bookinfo-service/ci/values-productpage-jetstream.yaml

deploy/platform/local/jetstream/nats-values.yaml
deploy/platform/local/jetstream/nats-token-secret.yaml
deploy/platform/local/jetstream/eventbus-jetstream.yaml

deploy/details/values-local-jetstream.yaml
deploy/reviews/values-local-jetstream.yaml
deploy/ratings/values-local-jetstream.yaml
deploy/ingestion/values-local-jetstream.yaml
deploy/notification/values-local-jetstream.yaml
deploy/dlqueue/values-local-jetstream.yaml
deploy/productpage/values-local-jetstream.yaml

docs/kafka-eventbus.md
docs/jetstream-eventbus.md
```

### Renamed (git mv)

```
pkg/eventskafka/                  → pkg/eventsmessaging/kafkapub/
services/details/internal/adapter/outbound/kafka/      → .../messaging/
services/reviews/internal/adapter/outbound/kafka/      → .../messaging/
services/ratings/internal/adapter/outbound/kafka/      → .../messaging/
services/ingestion/internal/adapter/outbound/kafka/    → .../messaging/
charts/bookinfo-service/templates/kafka-eventsource-rbac.yaml → .../eventsource-rbac.yaml
deploy/platform/local/eventbus.yaml                    → eventbus-kafka.yaml
deploy/details/values-local.yaml                       → values-local-kafka.yaml
deploy/reviews/values-local.yaml                       → values-local-kafka.yaml
deploy/ratings/values-local.yaml                       → values-local-kafka.yaml
deploy/ingestion/values-local.yaml                     → values-local-kafka.yaml
deploy/notification/values-local.yaml                  → values-local-kafka.yaml
deploy/dlqueue/values-local.yaml                       → values-local-kafka.yaml
deploy/productpage/values-local.yaml                   → values-local-kafka.yaml
```

### Modified

```
go.mod, go.sum                                                    (add nats.go)
services/{details,reviews,ratings,ingestion}/cmd/main.go          (EVENT_BACKEND switch)
services/{details,reviews,ratings,ingestion}/internal/adapter/outbound/messaging/producer.go
                                                                  (Publisher field, no embed)
charts/bookinfo-service/values.yaml                               (events.bus.type schema)
charts/bookinfo-service/templates/kafka-eventsource.yaml          (gated on bus.type=kafka)
charts/bookinfo-service/templates/configmap.yaml                  (EVENT_BACKEND, NATS_URL)
charts/bookinfo-service/templates/deployment.yaml                 (NATS_TOKEN env)
charts/bookinfo-service/templates/deployment-write.yaml           (NATS_TOKEN env)
charts/bookinfo-service/templates/eventsource.yaml                (eventBusName: default)
charts/bookinfo-service/templates/sensor.yaml                     (eventBusName: default)
charts/bookinfo-service/templates/consumer-sensor.yaml            (eventBusName: default)
deploy/platform/local/eventbus-kafka.yaml                         (metadata.name → default)
deploy/{all-services}/values-local-kafka.yaml                     (EVENT_BACKEND: kafka)
deploy/{all-services}/values-generated.yaml                       (header comment path)
tools/specgen/internal/runner/metadata.go                         (AsyncAPIServers map)
tools/specgen/internal/asyncapi/asyncapi.go                       (multi-server, comment path)
tools/specgen/internal/asyncapi/asyncapi_test.go                  (testdata regen)
tools/specgen/internal/values/values.go                           (comment path)
services/*/api/asyncapi.yaml                                      (regenerated)
Makefile                                                          (EVENTBUS var, branching)
README.md                                                         (bus selection sections)
CLAUDE.md                                                         (cluster name + bus selection)
.github/workflows/helm-lint-test.yml                              (run ct twice)
```

---

# Phase 1 — Go Publisher abstraction (kafka path stays green)

Goal of phase: refactor without changing observable behaviour. After phase 1, `make build-all && make test && make helm-lint` and `make run-k8s eventbus=kafka` (still default cluster `bookinfo-local` at this point) all pass. NATS not yet wired.

## Task 1.1: Define `Publisher` interface and tests

**Files:**
- Create: `pkg/eventsmessaging/publisher.go`
- Create: `pkg/eventsmessaging/publisher_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/eventsmessaging/publisher_test.go
package eventsmessaging_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
)

// stubPublisher confirms the interface compiles with a minimal impl.
type stubPublisher struct {
	calls int
}

func (s *stubPublisher) Publish(_ context.Context, _ events.Descriptor, _ any, _, _ string) error {
	s.calls++
	return nil
}

func (s *stubPublisher) Close() {}

func TestPublisherInterface(t *testing.T) {
	var p eventsmessaging.Publisher = &stubPublisher{}
	if err := p.Publish(context.Background(), events.Descriptor{}, nil, "k", "ik"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	p.Close()
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./pkg/eventsmessaging/...
```

Expected: FAIL with "package eventsmessaging is not in std" or "no Go files in pkg/eventsmessaging".

- [ ] **Step 3: Write the interface**

```go
// pkg/eventsmessaging/publisher.go
//
// Package eventsmessaging defines the bus-agnostic Publisher contract used
// by every producer service. Concrete impls live in subpackages
// (kafkapub for Kafka, natspub for NATS JetStream); cmd/main.go selects
// one at startup via the EVENT_BACKEND env var.
package eventsmessaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
)

// Publisher abstracts the per-record send path for any messaging backend.
// Implementations are responsible for marshalling the payload, building
// CloudEvents-binary headers from the descriptor, and propagating OTel
// trace context.
type Publisher interface {
	Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error
	Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./pkg/eventsmessaging/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/eventsmessaging/publisher.go pkg/eventsmessaging/publisher_test.go
git commit -m "feat(pkg/eventsmessaging): introduce backend-agnostic Publisher interface"
```

## Task 1.2: Rename `pkg/eventskafka` → `pkg/eventsmessaging/kafkapub`

**Files:**
- Move: `pkg/eventskafka/*` → `pkg/eventsmessaging/kafkapub/*`
- Modify: every `.go` file that imports `pkg/eventskafka`

- [ ] **Step 1: Move files**

```bash
mkdir -p pkg/eventsmessaging/kafkapub
git mv pkg/eventskafka/producer.go pkg/eventsmessaging/kafkapub/producer.go
git mv pkg/eventskafka/producer_test.go pkg/eventsmessaging/kafkapub/producer_test.go
# delete the now-empty package dir
rmdir pkg/eventskafka 2>/dev/null || true
```

- [ ] **Step 2: Rewrite package declaration**

In `pkg/eventsmessaging/kafkapub/producer.go` and `producer_test.go`, change the first non-comment line:

```diff
-package eventskafka
+package kafkapub
```

Update the package doc comment in `producer.go`:

```diff
-// Package eventskafka provides a shared Kafka producer that builds
-// CloudEvents-binary records from an events.Descriptor. Each service's
-// outbound kafka adapter wraps *Producer with typed methods that pick
-// the right descriptor for each domain event.
-package eventskafka
+// Package kafkapub provides a shared Kafka producer (franz-go) that
+// builds CloudEvents-binary records from an events.Descriptor. It
+// implements eventsmessaging.Publisher; each service's outbound
+// messaging adapter wraps *Producer with typed methods that pick the
+// right descriptor for each domain event.
+package kafkapub
```

- [ ] **Step 3: Find every importer and rewrite the import path**

```bash
grep -rln "kaio6fellipe/event-driven-bookinfo/pkg/eventskafka" \
  services pkg tools
```

For each file the grep returns, edit:

```diff
-"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventskafka"
+"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
```

And rewrite usages: `eventskafka.Producer` → `kafkapub.Producer`, `eventskafka.Client` → `kafkapub.Client`, `eventskafka.NewProducer` → `kafkapub.NewProducer`, `eventskafka.NewProducerWithClient` → `kafkapub.NewProducerWithClient`.

Expected importers (from earlier survey): all four producer services (`details`, `reviews`, `ratings`, `ingestion`) under `cmd/main.go` and `internal/adapter/outbound/kafka/`.

- [ ] **Step 4: Verify build + tests**

```
go build ./...
go test ./pkg/... ./services/...
```

Expected: PASS for both. Behaviour identical to before — only package paths changed.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(pkg): rename eventskafka -> eventsmessaging/kafkapub

Lays the groundwork for adding a NATS JetStream sibling
(pkg/eventsmessaging/natspub). Behaviour unchanged."
```

## Task 1.3: Make `kafkapub.Producer` satisfy `eventsmessaging.Publisher`

The existing `*kafkapub.Producer` has a `Publish(ctx, d, payload, recordKey, idempotencyKey) error` method and a `Close()` method — already satisfies the interface structurally. This task adds a compile-time interface check so a future signature drift fails at build time.

**Files:**
- Modify: `pkg/eventsmessaging/kafkapub/producer.go`

- [ ] **Step 1: Add the interface check**

After the `Producer` struct declaration, add:

```go
// Compile-time check that *Producer satisfies the Publisher contract.
var _ eventsmessaging.Publisher = (*Producer)(nil)
```

Add the import at the top of the file:

```go
import (
	// ... existing ...
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
)
```

- [ ] **Step 2: Run build + tests**

```
go build ./...
go test ./pkg/eventsmessaging/...
```

Expected: PASS. If the build fails with "interface contains method X but type lacks it", the existing signature has already drifted — fix it before continuing.

- [ ] **Step 3: Commit**

```bash
git add pkg/eventsmessaging/kafkapub/producer.go
git commit -m "feat(kafkapub): assert Publisher interface compliance"
```

## Task 1.4: Rename `outbound/kafka/` → `outbound/messaging/` per service

Repeat for **each producer service**: `details`, `reviews`, `ratings`, `ingestion`. Steps below show `details`; replicate verbatim for the others (substitute service name).

**Files (per service):**
- Move: `services/details/internal/adapter/outbound/kafka/*` → `.../messaging/*`
- Modify: `services/details/cmd/main.go`

- [ ] **Step 1: Move folder**

```bash
git mv services/details/internal/adapter/outbound/kafka services/details/internal/adapter/outbound/messaging
```

- [ ] **Step 2: Rewrite package declaration**

In every `.go` file under `services/details/internal/adapter/outbound/messaging/`, change:

```diff
-package kafka
+package messaging
```

- [ ] **Step 3: Rewrite import paths in importers**

```bash
grep -rln "services/details/internal/adapter/outbound/kafka" services/details
```

For each file, edit:

```diff
-"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/kafka"
+"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/messaging"
```

And rewrite call sites: `kafka.NewProducer` → `messaging.NewProducer`, `kafka.NoopProducer` → `messaging.NoopProducer`, etc. (Service-specific — check `cmd/main.go`.)

- [ ] **Step 4: Verify build + tests**

```
go build ./services/details/...
go test ./services/details/...
```

Expected: PASS.

- [ ] **Step 5: Commit (one commit per service)**

```bash
git add services/details
git commit -m "refactor(details): rename outbound/kafka -> outbound/messaging

The adapter is no longer Kafka-specific — it now holds a
Publisher interface that may resolve to kafka or jetstream."
```

- [ ] **Step 6: Repeat steps 1–5 for reviews, ratings, ingestion** (one commit per service).

## Task 1.5: Make typed wrappers depend on `Publisher` interface

Currently each service's `outbound/messaging/producer.go` embeds `*kafkapub.Producer` directly. Switch to a struct field of type `eventsmessaging.Publisher` so future backends drop in cleanly.

Repeat for **each producer service**: `details`, `reviews`, `ratings`, `ingestion`. Example uses ingestion since its file is fully shown in the spec; the others follow the same shape.

**Files (per service):**
- Modify: `services/<svc>/internal/adapter/outbound/messaging/producer.go`
- Modify: `services/<svc>/internal/adapter/outbound/messaging/producer_test.go`

- [ ] **Step 1: Update `producer.go`**

For `services/ingestion/internal/adapter/outbound/messaging/producer.go`:

```go
// Package messaging implements the EventPublisher port using a backend
// chosen at startup (kafka or jetstream).
package messaging

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// BookEvent is the marshaled record value for a book-added CloudEvent.
type BookEvent struct {
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

// Producer wraps an eventsmessaging.Publisher with service-specific
// typed methods. The Publisher impl is chosen by cmd/main.go.
type Producer struct {
	pub eventsmessaging.Publisher
}

// NewProducer builds a Producer from a Publisher. main.go decides which
// concrete impl to pass.
func NewProducer(pub eventsmessaging.Publisher) *Producer {
	return &Producer{pub: pub}
}

// Close releases the underlying publisher.
func (p *Producer) Close() { p.pub.Close() }

// PublishBookAdded sends a book-added CloudEvent to the configured backend.
func (p *Producer) PublishBookAdded(ctx context.Context, book domain.Book) error {
	isbn10, isbn13 := classifyISBN(book.ISBN)
	idempotencyKey := fmt.Sprintf("ingestion-isbn-%s", book.ISBN)

	evt := BookEvent{
		Title:          book.Title,
		Author:         strings.Join(book.Authors, ", "),
		Year:           book.PublishYear,
		Type:           "paperback",
		Pages:          book.Pages,
		Publisher:      book.Publisher,
		Language:       book.Language,
		ISBN10:         isbn10,
		ISBN13:         isbn13,
		IdempotencyKey: idempotencyKey,
	}

	d := events.Find(Exposed, "book-added")
	return p.pub.Publish(ctx, d, evt, book.ISBN, idempotencyKey)
}

func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
```

(Other services: same shape — replace `BookEvent` with each service's typed payload, `PublishBookAdded` with each service's typed method, `events.Find(Exposed, "<name>")` to pick the right descriptor.)

- [ ] **Step 2: Update `producer_test.go`**

Replace any direct `kafkapub` reliance with a fake `eventsmessaging.Publisher`:

```go
// services/ingestion/internal/adapter/outbound/messaging/producer_test.go
package messaging

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

type fakePub struct {
	last events.Descriptor
	key  string
	idem string
	body any
	err  error
}

func (f *fakePub) Publish(_ context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	f.last = d
	f.key = recordKey
	f.idem = idempotencyKey
	f.body = payload
	return f.err
}

func (f *fakePub) Close() {}

func TestPublishBookAdded_BuildsCorrectPayload(t *testing.T) {
	fp := &fakePub{}
	prod := NewProducer(fp)
	book := domain.Book{Title: "X", Authors: []string{"A"}, PublishYear: 2020, ISBN: "1234567890123"}
	if err := prod.PublishBookAdded(context.Background(), book); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if fp.last.CEType != "com.bookinfo.ingestion.book-added" {
		t.Fatalf("ce type = %q", fp.last.CEType)
	}
	be := fp.body.(BookEvent)
	if be.ISBN13 != "1234567890123" {
		t.Fatalf("isbn13 = %q", be.ISBN13)
	}
}
```

(Adapt for other services using their existing test cases as starting points.)

- [ ] **Step 3: Run service tests**

```
go test ./services/ingestion/...
```

Expected: PASS.

- [ ] **Step 4: Commit (one commit per service)**

```bash
git add services/ingestion
git commit -m "refactor(ingestion): hold eventsmessaging.Publisher instead of franz-go embed"
```

- [ ] **Step 5: Repeat for details, reviews, ratings.**

## Task 1.6: Wire `EVENT_BACKEND` switch in `cmd/main.go`

Repeat per service. Example uses `ingestion`; details/reviews/ratings follow the same shape.

**Files (per service):**
- Modify: `services/<svc>/cmd/main.go`

- [ ] **Step 1: Add backend switch**

Locate the kafka producer construction in `services/ingestion/cmd/main.go` (currently calls something like `kafka.NewProducer(ctx, cfg.KafkaBrokers, topic)` after the rename). Replace with:

```go
import (
	// ... existing imports ...
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/kafkapub"
	// natspub is added in Phase 2
)

// ... inside main() after cfg load ...

backend := os.Getenv("EVENT_BACKEND")
var pub eventsmessaging.Publisher
switch backend {
case "kafka", "":
	if cfg.KafkaBrokers == "" {
		logger.Warn("KAFKA_BROKERS empty, using no-op publisher")
		pub = messaging.NewNoopProducer() // existing pattern; satisfies Publisher structurally
	} else {
		kp, err := kafkapub.NewProducer(ctx, cfg.KafkaBrokers, cfg.KafkaTopic)
		if err != nil {
			logger.Error("init kafka producer", "err", err)
			os.Exit(1)
		}
		pub = kp
	}
case "jetstream":
	logger.Error("EVENT_BACKEND=jetstream not yet wired (phase 2)")
	os.Exit(1)
default:
	logger.Error("unknown EVENT_BACKEND", "value", backend)
	os.Exit(1)
}
defer pub.Close()

producer := messaging.NewProducer(pub)
```

(Adjust `cfg.KafkaTopic` to whatever your service uses. For services that don't have a single static topic — e.g. ones that publish to topics derived per descriptor — pass the descriptor's `Topic` field at the call site instead. Check the existing code for the exact pattern.)

- [ ] **Step 2: Build + run service tests**

```
go build ./services/ingestion/...
go test ./services/ingestion/...
```

Expected: PASS.

- [ ] **Step 3: Commit (one commit per service)**

```bash
git add services/ingestion/cmd/main.go
git commit -m "feat(ingestion): wire EVENT_BACKEND switch (kafka path only)"
```

- [ ] **Step 4: Repeat for details, reviews, ratings.**

## Task 1.7: Phase-1 verification gate

- [ ] **Step 1: Run full test suite**

```
make build-all
make test
```

Expected: PASS for both.

- [ ] **Step 2: Run linter**

```
make lint
```

Expected: PASS.

- [ ] **Step 3: Run helm-lint**

```
make helm-lint
```

Expected: PASS — chart still uses `events.kafka.broker` field; chart changes come in Phase 3.

- [ ] **Step 4: Smoke test the existing kafka cluster**

```
make stop-k8s 2>/dev/null || true
make run-k8s
make k8s-status
```

Expected: kafka cluster comes up, all pods Ready, productpage reachable on `http://localhost:8080`. Send one POST to confirm the write path still works end-to-end.

- [ ] **Step 5: Commit phase tag**

```bash
git tag phase-1-complete-jetstream-eventbus
```

(Local-only tag; not pushed.)

---

# Phase 2 — NATS JetStream Publisher implementation

Goal of phase: ship `natspub` with full test coverage. After phase 2, `EVENT_BACKEND=jetstream` is a working code path (running locally against a local NATS server) but no chart/platform plumbing yet exposes it.

## Task 2.1: Add `nats.go` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the module**

```
go get github.com/nats-io/nats.go@latest
go get github.com/nats-io/nats-server/v2@latest
go mod tidy
```

(`nats-server/v2` is for the embedded test server, only used in `_test.go`.)

- [ ] **Step 2: Verify build**

```
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(go.mod): add nats.go and nats-server/v2 (test) deps"
```

## Task 2.2: Add NATS telemetry helpers (must come before natspub — natspub imports them)

**Files:**
- Create: `pkg/telemetry/nats.go`
- Create: `pkg/telemetry/nats_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/telemetry/nats_test.go
package telemetry_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestInjectTraceContextNATS_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	msg := &nats.Msg{Header: nats.Header{}}
	telemetry.InjectTraceContextNATS(ctx, msg)
	_ = msg.Header.Get("traceparent")
}

func TestStartNATSProducerSpan_ReturnsValidSpan(t *testing.T) {
	ctx, span := telemetry.StartNATSProducerSpan(context.Background(), "subject.test", "idem-1")
	defer span.End()
	if ctx == nil {
		t.Fatal("ctx should not be nil")
	}
	if span == nil {
		t.Fatal("span should not be nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./pkg/telemetry/...
```

Expected: FAIL with "undefined StartNATSProducerSpan / InjectTraceContextNATS".

- [ ] **Step 3: Implement**

```go
// pkg/telemetry/nats.go
package telemetry

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string { return nats.Header(c).Get(key) }
func (c natsHeaderCarrier) Set(key, val string)   { nats.Header(c).Set(key, val) }
func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// StartNATSProducerSpan opens a span around a JetStream publish.
func StartNATSProducerSpan(ctx context.Context, subject, idempotencyKey string) (context.Context, trace.Span) {
	tracer := otel.Tracer("eventsmessaging/natspub")
	return tracer.Start(ctx, "jetstream.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "jetstream"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.message.idempotency_key", idempotencyKey),
		),
	)
}

// InjectTraceContextNATS writes the active span context into msg.Header
// using the configured global propagator.
func InjectTraceContextNATS(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))
}
```

- [ ] **Step 4: Run tests**

```
go test ./pkg/telemetry/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry/nats.go pkg/telemetry/nats_test.go
git commit -m "feat(pkg/telemetry): NATS producer span + traceparent injection"
```

## Task 2.3: Implement `natspub` producer (TDD)

**Files:**
- Create: `pkg/eventsmessaging/natspub/producer.go`
- Create: `pkg/eventsmessaging/natspub/producer_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// pkg/eventsmessaging/natspub/producer_test.go
package natspub_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/natspub"
)

func runJetStreamServer(t *testing.T) (*server.Server, string) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // random port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natstest.RunServer(&opts)
	return s, s.ClientURL()
}

func TestProducer_PublishCreatesStreamAndDelivers(t *testing.T) {
	s, url := runJetStreamServer(t)
	defer s.Shutdown()

	d := events.Descriptor{
		Name:        "book-added",
		Topic:       "raw_books_details",
		CEType:      "com.bookinfo.ingestion.book-added",
		CESource:    "ingestion",
		Version:     "1.0",
		ContentType: "application/json",
	}

	p, err := natspub.NewProducer(context.Background(), url, "" /*no token*/, d.Topic, d.Topic)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	defer p.Close()

	if err := p.Publish(context.Background(), d, map[string]string{"hello": "world"}, "key-1", "idem-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Verify a message landed on the subject.
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	sub, err := js.SubscribeSync(d.Topic, nats.OrderedConsumer())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("next msg: %v", err)
	}
	if got := msg.Header.Get("ce-type"); got != d.CEType {
		t.Errorf("ce-type header = %q, want %q", got, d.CEType)
	}
	if got := msg.Header.Get("ce-source"); got != d.CESource {
		t.Errorf("ce-source header = %q, want %q", got, d.CESource)
	}
	if got := msg.Header.Get("ce-subject"); got != "key-1" {
		t.Errorf("ce-subject header = %q, want %q", got, "key-1")
	}
}

func TestProducer_StreamEnsureIdempotent(t *testing.T) {
	s, url := runJetStreamServer(t)
	defer s.Shutdown()

	for i := 0; i < 2; i++ {
		p, err := natspub.NewProducer(context.Background(), url, "", "raw_books_details", "raw_books_details")
		if err != nil {
			t.Fatalf("new producer iter %d: %v", i, err)
		}
		p.Close()
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./pkg/eventsmessaging/natspub/...
```

Expected: FAIL with "natspub" package missing.

- [ ] **Step 3: Implement the producer**

```go
// pkg/eventsmessaging/natspub/producer.go
//
// Package natspub implements eventsmessaging.Publisher on top of NATS
// JetStream. NewProducer ensures the stream exists (idempotent) and
// publishes CloudEvents-binary messages with NATS headers and OTel
// trace-context propagation.
package natspub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/codes"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

// Producer publishes CloudEvents-binary messages to a JetStream stream.
type Producer struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	subject string
}

// Compile-time interface compliance check.
var _ eventsmessaging.Publisher = (*Producer)(nil)

// NewProducer connects to NATS, ensures the JetStream stream exists, and
// returns a Publisher bound to the configured subject.
//
// streamName is the JetStream stream name. subject is the publish target;
// for current usage they are the same string (e.g. "raw_books_details").
// token is optional — empty string skips token auth (local-dev mode).
func NewProducer(ctx context.Context, url, token, streamName, subject string) (*Producer, error) {
	opts := []nats.Option{nats.Name("event-driven-bookinfo")}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS at %s: %w", url, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("acquiring JetStream context: %w", err)
	}
	if err := ensureStream(js, streamName, subject); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensuring stream %q: %w", streamName, err)
	}
	return &Producer{nc: nc, js: js, subject: subject}, nil
}

// Publish marshals payload to JSON and emits a NATS JetStream message
// with CloudEvents-binary headers + OTel traceparent.
func (p *Producer) Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", d.Name, err)
	}

	now := time.Now().UTC()
	hdr := nats.Header{}
	hdr.Set("ce-specversion", d.Version)
	hdr.Set("ce-type", d.CEType)
	hdr.Set("ce-source", d.CESource)
	hdr.Set("ce-id", uuid.New().String())
	hdr.Set("ce-time", now.Format(time.RFC3339))
	hdr.Set("ce-subject", recordKey)
	hdr.Set("content-type", d.ContentType)

	msg := &nats.Msg{
		Subject: p.subject,
		Data:    value,
		Header:  hdr,
	}

	ctx, span := telemetry.StartNATSProducerSpan(ctx, p.subject, idempotencyKey)
	defer span.End()
	telemetry.InjectTraceContextNATS(ctx, msg)

	if _, err := p.js.PublishMsg(msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publishing to JetStream subject %q: %w", p.subject, err)
	}

	logger.Debug("published event",
		"subject", p.subject,
		"ce_type", d.CEType,
		"idempotency_key", idempotencyKey,
	)
	return nil
}

// Close drains and closes the NATS connection.
func (p *Producer) Close() {
	if p.nc != nil {
		_ = p.nc.Drain()
	}
}

func ensureStream(js nats.JetStreamContext, name, subject string) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     name,
		Subjects: []string{subject},
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil
	}
	return err
}
```

(`telemetry.StartNATSProducerSpan` and `InjectTraceContextNATS` were defined in Task 2.2.)

- [ ] **Step 4: Run tests**

```
go test ./pkg/eventsmessaging/natspub/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/eventsmessaging/natspub/
git commit -m "feat(natspub): NATS JetStream Publisher with stream ensure + CE headers"
```

## Task 2.4: Wire `natspub` into per-service `cmd/main.go`

Repeat per producer service. Example uses ingestion.

**Files (per service):**
- Modify: `services/<svc>/cmd/main.go`

- [ ] **Step 1: Add jetstream branch**

In `services/ingestion/cmd/main.go`, replace the placeholder error from Task 1.6 step 1:

```go
import (
	// ... existing ...
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/natspub"
)

// ... in the switch ...
case "jetstream":
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		logger.Error("NATS_URL must be set when EVENT_BACKEND=jetstream")
		os.Exit(1)
	}
	token := os.Getenv("NATS_TOKEN")
	d := events.Find(messaging.Exposed, "book-added") // adapt per service
	np, err := natspub.NewProducer(ctx, natsURL, token, d.Topic, d.Topic)
	if err != nil {
		logger.Error("init nats producer", "err", err)
		os.Exit(1)
	}
	pub = np
```

(For services that publish multiple descriptors, choose the canonical stream/subject pair from the service's `Exposed` slice — typically all share one topic per service.)

- [ ] **Step 2: Build + test**

```
go build ./services/ingestion/...
go test ./services/ingestion/...
```

Expected: PASS.

- [ ] **Step 3: Commit (one commit per service)**

```bash
git add services/ingestion/cmd/main.go
git commit -m "feat(ingestion): wire EVENT_BACKEND=jetstream branch via natspub"
```

- [ ] **Step 4: Repeat for details, reviews, ratings.**

## Task 2.5: Phase-2 verification gate

- [ ] **Step 1: Full test suite**

```
make test
make lint
```

Expected: PASS.

- [ ] **Step 2: Manual integration smoke (optional)**

Run a local NATS server in docker and test ingestion can publish:

```bash
docker run --rm -d --name nats-smoke -p 4222:4222 nats:2-alpine -js
SERVICE_NAME=ingestion HTTP_PORT=8086 ADMIN_PORT=9096 \
  EVENT_BACKEND=jetstream NATS_URL=nats://localhost:4222 \
  KAFKA_TOPIC=raw_books_details \
  POLL_INTERVAL=10s SEARCH_QUERIES=test MAX_RESULTS_PER_QUERY=1 \
  go run ./services/ingestion/cmd/
# In another shell, verify a stream was created:
docker exec nats-smoke nats stream ls
docker rm -f nats-smoke
```

Expected: stream `raw_books_details` listed.

- [ ] **Step 3: Tag**

```bash
git tag phase-2-complete-jetstream-eventbus
```

---

# Phase 3 — Helm chart updates for dual-bus

Goal of phase: chart can render either kafka or jetstream EventSources based on `events.bus.type`. After phase 3, `make helm-lint` passes against both kafka and jetstream values; deployment-time still uses the kafka cluster.

## Task 3.1: Update `values.yaml` schema

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`

- [ ] **Step 1: Replace the `events` block**

Find the existing block (lines ~155–185) and replace with:

```yaml
# -- Event pipeline (independent of CQRS)
events:
  # -- Bus selection. Templates branch on bus.type to render the right
  # EventSource (kafka or jetstream). EventBus CR name is hardcoded
  # to "default" inside the chart — only one EventBus per cluster.
  bus:
    type: kafka              # kafka | jetstream
  kafka:
    broker: ""               # bootstrap host:port; injected as KAFKA_BROKERS
  jetstream:
    url: ""                  # nats://host:port; injected as NATS_URL
    tokenSecret:
      name: nats-client-token
      key: token             # mounted as NATS_TOKEN env via valueFrom.secretKeyRef
  exposed: {}
    # event-name:
    #   topic: kafka_topic_or_nats_subject
    #   contentType: application/json
    #   eventTypes:
    #     - com.bookinfo.<service>.<event>
  consumed: {}
    # (unchanged shape; documented in deploy/<svc>/values-local-*.yaml examples)
```

- [ ] **Step 2: Render the chart against the existing values to confirm parse**

```
helm dependency build charts/bookinfo-service
helm template charts/bookinfo-service -f deploy/details/values-local.yaml --debug 2>&1 | head -40
```

Expected: a parseable render (warnings about missing `events.bus.type` from the values file are fine — values-local.yaml hasn't been split yet; the chart default `kafka` covers it).

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -m "feat(chart): add events.bus.type schema for kafka|jetstream"
```

## Task 3.2: Gate `kafka-eventsource.yaml` on `bus.type=kafka`

**Files:**
- Modify: `charts/bookinfo-service/templates/kafka-eventsource.yaml`

- [ ] **Step 1: Wrap the existing range**

```yaml
{{/* charts/bookinfo-service/templates/kafka-eventsource.yaml */}}
{{- if eq .Values.events.bus.type "kafka" }}
{{- range $eventName, $event := .Values.events.exposed }}
---
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
  {{- with $event.eventTypes }}
  annotations:
    bookinfo.io/emitted-ce-types: {{ join "," . | quote }}
  {{- end }}
spec:
  eventBusName: default
  template:
    serviceAccountName: {{ include "bookinfo-service.serviceAccountName" $ }}
    {{- with $.Values.observability.otelEndpoint }}
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ printf "%s-%s-eventsource" (include "bookinfo-service.fullname" $) $eventName | quote }}
    {{- end }}
  kafka:
    {{ $eventName }}:
      url: {{ required "events.kafka.broker must be set when bus.type=kafka and events.exposed is defined" $.Values.events.kafka.broker }}
      topic: {{ required (printf "events.exposed.%s.topic is required" $eventName) $event.topic }}
      consumerGroup:
        groupName: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
      jsonBody: true
      {{- with $event.contentType }}
      contentType: {{ . }}
      {{- end }}
{{- end }}
{{- end }}
```

(Two changes: outer `{{- if eq .Values.events.bus.type "kafka" }}` ... `{{- end }}` wrapper, and `eventBusName: default` instead of the templated value.)

- [ ] **Step 2: Render**

```
helm template charts/bookinfo-service -f deploy/ingestion/values-local.yaml -f deploy/ingestion/values-generated.yaml --debug | grep -A3 "kind: EventSource" | head -20
```

Expected: the kafka EventSource still rendered (because no values file has changed `bus.type` from default `kafka`).

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/kafka-eventsource.yaml
git commit -m "feat(chart): gate kafka-eventsource on bus.type=kafka; pin eventBusName=default"
```

## Task 3.3: Add `jetstream-eventsource.yaml` template

**Files:**
- Create: `charts/bookinfo-service/templates/jetstream-eventsource.yaml`

- [ ] **Step 1: Write the template**

```yaml
{{/* charts/bookinfo-service/templates/jetstream-eventsource.yaml */}}
{{- if eq .Values.events.bus.type "jetstream" }}
{{- range $eventName, $event := .Values.events.exposed }}
---
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
  {{- with $event.eventTypes }}
  annotations:
    bookinfo.io/emitted-ce-types: {{ join "," . | quote }}
  {{- end }}
spec:
  eventBusName: default
  template:
    serviceAccountName: {{ include "bookinfo-service.serviceAccountName" $ }}
    {{- with $.Values.observability.otelEndpoint }}
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: {{ . | quote }}
        - name: OTEL_SERVICE_NAME
          value: {{ printf "%s-%s-eventsource" (include "bookinfo-service.fullname" $) $eventName | quote }}
    {{- end }}
  jetstream:
    {{ $eventName }}:
      url: {{ required "events.jetstream.url must be set when bus.type=jetstream and events.exposed is defined" $.Values.events.jetstream.url | quote }}
      accessSecret:
        name: {{ $.Values.events.jetstream.tokenSecret.name | quote }}
        key:  {{ $.Values.events.jetstream.tokenSecret.key  | quote }}
      subject: {{ required (printf "events.exposed.%s.topic is required (used as JetStream subject)" $eventName) $event.topic | quote }}
      jsonBody: true
{{- end }}
{{- end }}
```

- [ ] **Step 2: Render against a synthetic jetstream values file**

```
cat > /tmp/jetstream-test.yaml <<EOF
events:
  bus:
    type: jetstream
  jetstream:
    url: nats://nats.platform.svc.cluster.local:4222
    tokenSecret:
      name: nats-client-token
      key: token
  exposed:
    raw-books-details:
      topic: raw_books_details
      contentType: application/json
      eventTypes:
        - com.bookinfo.ingestion.book-added
EOF
helm template charts/bookinfo-service -f /tmp/jetstream-test.yaml --debug | grep -B1 -A20 "jetstream:"
```

Expected: a `kind: EventSource` with `spec.jetstream.raw-books-details.{url, accessSecret, subject, jsonBody}` rendered.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/jetstream-eventsource.yaml
git commit -m "feat(chart): add jetstream-eventsource template (renders when bus.type=jetstream)"
```

## Task 3.4: Rename `kafka-eventsource-rbac.yaml` → `eventsource-rbac.yaml`

The leader-election RBAC is bus-agnostic — render whenever any EventSource will exist.

**Files:**
- Move: `charts/bookinfo-service/templates/kafka-eventsource-rbac.yaml` → `eventsource-rbac.yaml`

- [ ] **Step 1: Rename**

```bash
git mv charts/bookinfo-service/templates/kafka-eventsource-rbac.yaml charts/bookinfo-service/templates/eventsource-rbac.yaml
```

- [ ] **Step 2: Render to confirm template still resolves**

```
helm template charts/bookinfo-service -f deploy/details/values-local.yaml | grep -A2 "leader-election"
```

Expected: Role + RoleBinding still rendered.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/eventsource-rbac.yaml
git commit -m "refactor(chart): rename kafka-eventsource-rbac.yaml -> eventsource-rbac.yaml

The leader-election RBAC is bus-agnostic; rendering it under a
kafka-prefixed name was misleading."
```

## Task 3.5: Update `configmap.yaml` (EVENT_BACKEND, NATS_URL conditional)

**Files:**
- Modify: `charts/bookinfo-service/templates/configmap.yaml`

- [ ] **Step 1: Edit the ConfigMap data block**

Replace lines ~26–28 (the `with .Values.events.kafka.broker` block) with:

```yaml
  EVENT_BACKEND: {{ .Values.events.bus.type | quote }}
  {{- if eq .Values.events.bus.type "kafka" }}
  {{- with .Values.events.kafka.broker }}
  KAFKA_BROKERS: {{ . | quote }}
  {{- end }}
  {{- else if eq .Values.events.bus.type "jetstream" }}
  {{- with .Values.events.jetstream.url }}
  NATS_URL: {{ . | quote }}
  {{- end }}
  {{- end }}
```

- [ ] **Step 2: Render and grep**

```
helm template charts/bookinfo-service -f /tmp/jetstream-test.yaml | grep -E "EVENT_BACKEND|NATS_URL|KAFKA_BROKERS"
helm template charts/bookinfo-service -f deploy/details/values-local.yaml | grep -E "EVENT_BACKEND|NATS_URL|KAFKA_BROKERS"
```

Expected: jetstream values yield `EVENT_BACKEND: "jetstream"` + `NATS_URL`; kafka values yield `EVENT_BACKEND: "kafka"` + `KAFKA_BROKERS`.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/configmap.yaml
git commit -m "feat(chart): emit EVENT_BACKEND + bus-specific connection env"
```

## Task 3.6: Add `NATS_TOKEN` env via secretKeyRef in deployment templates

**Files:**
- Modify: `charts/bookinfo-service/templates/deployment.yaml`
- Modify: `charts/bookinfo-service/templates/deployment-write.yaml`

- [ ] **Step 1: Add the env block**

In both files, find the container `env:` list (or the `envFrom: configMapRef` block) and add a sibling `env:` entry that injects `NATS_TOKEN` only when `bus.type=jetstream`:

```yaml
        env:
          {{- if eq .Values.events.bus.type "jetstream" }}
          - name: NATS_TOKEN
            valueFrom:
              secretKeyRef:
                name: {{ .Values.events.jetstream.tokenSecret.name | quote }}
                key:  {{ .Values.events.jetstream.tokenSecret.key  | quote }}
          {{- end }}
```

(If the deployment already uses `envFrom` exclusively, add a parallel `env:` block at the same indent. If it already has `env:`, append the new entry.)

- [ ] **Step 2: Render**

```
helm template charts/bookinfo-service -f /tmp/jetstream-test.yaml | grep -B2 -A4 NATS_TOKEN
```

Expected: secretKeyRef block present.

```
helm template charts/bookinfo-service -f deploy/details/values-local.yaml | grep NATS_TOKEN
```

Expected: empty (no NATS_TOKEN on kafka path).

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/deployment.yaml charts/bookinfo-service/templates/deployment-write.yaml
git commit -m "feat(chart): inject NATS_TOKEN via secretKeyRef when bus.type=jetstream"
```

## Task 3.7: Pin `eventBusName: default` in remaining templates

**Files:**
- Modify: `charts/bookinfo-service/templates/eventsource.yaml`
- Modify: `charts/bookinfo-service/templates/sensor.yaml`
- Modify: `charts/bookinfo-service/templates/consumer-sensor.yaml`

- [ ] **Step 1: Replace the templated busName**

In each file, find:

```yaml
  eventBusName: {{ .Values.events.busName }}
```

(or `{{ $.Values.events.busName }}`) and replace with:

```yaml
  eventBusName: default
```

- [ ] **Step 2: Render against both values files**

```
helm template charts/bookinfo-service -f deploy/details/values-local.yaml | grep -E "eventBusName"
helm template charts/bookinfo-service -f /tmp/jetstream-test.yaml | grep -E "eventBusName"
```

Expected: every line shows `eventBusName: default`.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/templates/
git commit -m "refactor(chart): pin eventBusName=default across all templates

One EventBus per cluster — the name is a chart constant, not a value."
```

## Task 3.8: Add `ci/values-<svc>-jetstream.yaml` test fixtures

**Files (per service — 7 total):**
- Create: `charts/bookinfo-service/ci/values-<svc>-jetstream.yaml`

- [ ] **Step 1: For each service in (details, reviews, ratings, ingestion, productpage, notification, dlqueue), copy the existing kafka fixture and swap the bus stanza**

Example for ingestion:

```yaml
# charts/bookinfo-service/ci/values-ingestion-jetstream.yaml
serviceName: ingestion
fullnameOverride: ingestion
image:
  repository: event-driven-bookinfo/ingestion
  tag: ci

config:
  LOG_LEVEL: "debug"
  EVENT_BACKEND: "jetstream"
  NATS_URL: "nats://nats.platform.svc.cluster.local:4222"

events:
  bus:
    type: jetstream
  jetstream:
    url: "nats://nats.platform.svc.cluster.local:4222"
    tokenSecret:
      name: nats-client-token
      key: token
  exposed:
    raw-books-details:
      topic: raw_books_details
      contentType: application/json
      eventTypes:
        - com.bookinfo.ingestion.book-added
```

(Replicate for each service — same shape, swap service-specific exposed/consumed blocks per the existing kafka fixture.)

- [ ] **Step 2: Run `ct lint`**

```
ct lint --charts charts/bookinfo-service \
  --target-branch main \
  --validate-maintainers=false \
  --check-version-increment=false
```

Expected: PASS for both kafka and jetstream fixture sets.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/ci/
git commit -m "test(chart): add jetstream ci values for chart-testing across all 7 services"
```

## Task 3.9: Phase-3 verification gate

- [ ] **Step 1: helm-lint full sweep**

```
make helm-lint
```

Expected: PASS.

- [ ] **Step 2: Tag**

```bash
git tag phase-3-complete-jetstream-eventbus
```

---

# Phase 4 — Specgen multi-server output

Goal of phase: regenerated `services/*/api/asyncapi.yaml` declares both kafka and jetstream servers; comment paths reflect the messaging folder rename.

## Task 4.1: Add `Protocol` to `ServerEntry` and a multi-server map

**Files:**
- Modify: `tools/specgen/internal/runner/metadata.go`

- [ ] **Step 1: Update the metadata struct**

Find `AsyncAPIServer ServerEntry` (line ~13 per earlier grep) and the seed values (line ~33 area). Replace the field declaration and the seed:

```go
// metadata.go (excerpt — adjust to actual surrounding code)
type SpecMetadata struct {
	// ... other fields ...
	AsyncAPIServers map[string]ServerEntry
}

type ServerEntry struct {
	URL         string
	Protocol    string
	Description string
}

// ... in the constructor that builds default metadata ...
AsyncAPIServers: map[string]ServerEntry{
	"kafka": {
		URL:         "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092",
		Protocol:    "kafka",
		Description: "Local Kafka bootstrap (kafka cluster)",
	},
	"jetstream": {
		URL:         "nats://nats.platform.svc.cluster.local:4222",
		Protocol:    "nats",
		Description: "Local NATS JetStream (jetstream cluster)",
	},
},
```

(Locate the existing single-server seed and replace; remove the old `AsyncAPIServer` field.)

- [ ] **Step 2: Build**

```
go build ./tools/specgen/...
```

Expected: compile errors in `asyncapi.go` referring to the old `AsyncAPIServer` field. Fix in Task 4.2.

- [ ] **Step 3: Commit**

(Don't commit yet — build is broken. Move directly to Task 4.2.)

## Task 4.2: Render multiple servers in `asyncapi.go`

**Files:**
- Modify: `tools/specgen/internal/asyncapi/asyncapi.go`

- [ ] **Step 1: Replace the kafka-hardwired block**

Find lines ~315–322 (the block that builds `kafkaServerNode`) and replace with:

```go
// servers — AsyncAPI 3.x servers is a mapping (not a list like OpenAPI).
serversNode := yamlutil.Mapping()
for name, srv := range in.Metadata.AsyncAPIServers {
	serverNode := yamlutil.Mapping()
	yamlutil.AddScalar(serverNode, "host", srv.URL)
	yamlutil.AddScalar(serverNode, "protocol", srv.Protocol)
	yamlutil.AddScalar(serverNode, "description", srv.Description)
	yamlutil.AddMapping(serversNode, name, serverNode)
}
yamlutil.AddMapping(docNode, "servers", serversNode)
```

(Note: Go map iteration is randomised. If determinism matters for testdata diffs, sort the keys explicitly: `keys := slices.Sorted(maps.Keys(in.Metadata.AsyncAPIServers))` then range over `keys`.)

- [ ] **Step 2: Update the source-comment line (line ~357)**

Find:

```go
buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/outbound/kafka/exposed.go\n")
```

Replace with:

```go
buf.WriteString("# Source: services/" + in.ServiceName + "/internal/adapter/outbound/messaging/exposed.go\n")
```

- [ ] **Step 3: Build**

```
go build ./tools/specgen/...
```

Expected: PASS.

- [ ] **Step 4: Run specgen tests**

```
go test ./tools/specgen/...
```

Expected: testdata-fixture failures — fixtures still have only one server. Fix next.

## Task 4.3: Update `values.go` source-comment and regenerate testdata

**Files:**
- Modify: `tools/specgen/internal/values/values.go`
- Modify: `tools/specgen/internal/asyncapi/testdata/*` (regenerated)

- [ ] **Step 1: Edit `values/values.go` line 188**

Find:

```go
buf.WriteString("#   services/" + in.ServiceName + "/internal/adapter/outbound/kafka/exposed.go\n")
```

Replace `kafka` with `messaging`.

- [ ] **Step 2: Regenerate testdata**

Look at `tools/specgen/internal/asyncapi/asyncapi_test.go` to find the regenerate flag (commonly `-update` or `UPDATE=1`). Standard pattern:

```
go test ./tools/specgen/internal/asyncapi/... -update
```

(Or whatever flag the test uses — read `asyncapi_test.go` first.)

- [ ] **Step 3: Inspect the regenerated fixtures**

```
git diff tools/specgen/internal/asyncapi/testdata/
```

Expected: every fixture gains a `jetstream:` server block alongside `kafka:`.

- [ ] **Step 4: Run tests**

```
go test ./tools/specgen/...
```

Expected: PASS.

- [ ] **Step 5: Commit (Tasks 4.1, 4.2, 4.3 together — they're coupled)**

```bash
git add tools/specgen/
git commit -m "feat(specgen): emit kafka + jetstream servers in AsyncAPI output

Also updates the values-generated.yaml header comment path to
reflect the outbound/kafka -> outbound/messaging rename."
```

## Task 4.4: Regenerate service-level artifacts

- [ ] **Step 1: Run specgen against the whole repo**

```
make generate-specs
```

Expected: changes to:
- `services/*/api/asyncapi.yaml` — second `jetstream` server block.
- `deploy/*/values-generated.yaml` — comment `# from services/<svc>/internal/adapter/outbound/messaging/exposed.go`.

- [ ] **Step 2: Inspect**

```
git diff services/ | head -80
git diff deploy/ | head -40
```

- [ ] **Step 3: Commit**

```bash
git add services/ deploy/
git commit -m "chore(generated): regenerate AsyncAPI + values-generated for messaging path

- AsyncAPI now declares both kafka and jetstream servers.
- values-generated.yaml header references outbound/messaging."
```

## Task 4.5: Phase-4 verification gate

- [ ] **Step 1: Test sweep**

```
make test
make lint
```

Expected: PASS.

- [ ] **Step 2: Tag**

```bash
git tag phase-4-complete-jetstream-eventbus
```

---

# Phase 5 — Platform layer + Makefile

Goal of phase: `make run-k8s eventbus=jetstream` actually creates a working second cluster with NATS+JetStream and an Argo Events EventBus pointing at it. After phase 5, both buses are deployable and observable.

## Task 5.1: Rename + retarget kafka EventBus

**Files:**
- Move + Modify: `deploy/platform/local/eventbus.yaml` → `eventbus-kafka.yaml`

- [ ] **Step 1: Rename + flip name**

```bash
git mv deploy/platform/local/eventbus.yaml deploy/platform/local/eventbus-kafka.yaml
```

Edit the file:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: default
  namespace: bookinfo
spec:
  kafka:
    url: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    topic: argo-events
    version: "4.2.0"
    consumerBatchMaxWait: "0"
```

(Only change is `metadata.name: kafka` → `metadata.name: default`.)

- [ ] **Step 2: Commit**

```bash
git add deploy/platform/local/eventbus-kafka.yaml
git commit -m "refactor(platform): rename kafka eventbus to 'default'

Aligns with chart hardcoded eventBusName=default — one EventBus
per cluster, name is a constant."
```

## Task 5.2: Create NATS Helm values

**Files:**
- Create: `deploy/platform/local/jetstream/nats-values.yaml`

- [ ] **Step 1: Write the values**

```yaml
# deploy/platform/local/jetstream/nats-values.yaml
#
# Values for the official nats-io/nats Helm chart. JetStream enabled,
# single replica, file storage on emptyDir, token auth via the
# nats-client-token secret (created separately).
config:
  cluster:
    enabled: false
  jetstream:
    enabled: true
    fileStore:
      enabled: true
      dir: /data
      pvc:
        enabled: false
  resolver:
    type: full
container:
  image:
    repository: nats
    tag: 2.10-alpine
service:
  enabled: true
  ports:
    nats:
      enabled: true
      port: 4222
auth:
  enabled: true
  tokenAuth:
    secretName: nats-client-token
    secretKey: token
podTemplate:
  topologySpreadConstraints: []
```

(Cross-check actual key names against the upstream nats-io/nats chart README — values shape differs across chart versions.)

- [ ] **Step 2: Commit**

```bash
git add deploy/platform/local/jetstream/nats-values.yaml
git commit -m "feat(platform): NATS Helm values for jetstream local cluster"
```

## Task 5.3: Create NATS token secret manifest

**Files:**
- Create: `deploy/platform/local/jetstream/nats-token-secret.yaml`

- [ ] **Step 1: Write the manifest**

```yaml
# deploy/platform/local/jetstream/nats-token-secret.yaml
#
# Local-dev token shared between NATS server, EventBus accessSecret,
# EventSource accessSecret, and pod NATS_TOKEN env. Plain-text only
# acceptable in this local-only context.
---
apiVersion: v1
kind: Secret
metadata:
  name: nats-client-token
  namespace: platform
type: Opaque
stringData:
  token: "local-dev-token"
---
apiVersion: v1
kind: Secret
metadata:
  name: nats-client-token
  namespace: bookinfo
type: Opaque
stringData:
  token: "local-dev-token"
```

- [ ] **Step 2: Commit**

```bash
git add deploy/platform/local/jetstream/nats-token-secret.yaml
git commit -m "feat(platform): add nats-client-token secret for local jetstream"
```

## Task 5.4: Create jetstream EventBus manifest

**Files:**
- Create: `deploy/platform/local/jetstream/eventbus-jetstream.yaml`

- [ ] **Step 1: Write the manifest**

```yaml
# deploy/platform/local/jetstream/eventbus-jetstream.yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: default
  namespace: bookinfo
spec:
  jetstreamExotic:
    url: nats://nats.platform.svc.cluster.local:4222
    accessSecret:
      name: nats-client-token
      key: token
    streamConfig: ""
```

- [ ] **Step 2: Commit**

```bash
git add deploy/platform/local/jetstream/eventbus-jetstream.yaml
git commit -m "feat(platform): EventBus 'default' with jetstreamExotic for local cluster"
```

## Task 5.5: Update Makefile (EVENTBUS variable + branching)

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add the variable + cluster name**

Near the top of `Makefile`, locate `K8S_CLUSTER` and add `EVENTBUS` above it:

```make
EVENTBUS ?= kafka
ifeq ($(filter $(EVENTBUS),kafka jetstream),)
$(error EVENTBUS must be 'kafka' or 'jetstream', got '$(EVENTBUS)')
endif
K8S_CLUSTER := bookinfo-$(EVENTBUS)-local
```

- [ ] **Step 2: Replace every literal `bookinfo-local` with `$(K8S_CLUSTER)`**

```bash
grep -n "bookinfo-local\|k3d-bookinfo-local" Makefile
```

For each match, replace `bookinfo-local` → `$(K8S_CLUSTER)` and `k3d-bookinfo-local` → `k3d-$(K8S_CLUSTER)`.

- [ ] **Step 3: Add the refusal-to-coexist guard at top of `run-k8s`**

In the `run-k8s` target body, add the first action:

```make
.PHONY: run-k8s
run-k8s: ##@Kubernetes Full local k8s setup: cluster -> platform -> observability -> deploy
	@other=$$( [ "$(EVENTBUS)" = "kafka" ] && echo "bookinfo-jetstream-local" || echo "bookinfo-kafka-local" ); \
	if k3d cluster list $$other >/dev/null 2>&1; then \
		printf "$(BOLD)$(YELLOW)Refusing to start: cluster '%s' is running.$(NC)\n" "$$other"; \
		printf "Run: make stop-k8s EVENTBUS=%s\n" "$$( [ "$$other" = "bookinfo-jetstream-local" ] && echo "jetstream" || echo "kafka" )"; \
		exit 1; \
	fi
	# ... existing run-k8s body, now using $(K8S_CLUSTER) ...
```

- [ ] **Step 4: Branch `k8s-platform` on `$(EVENTBUS)`**

Locate `k8s-platform`. Wrap the Strimzi+Kafka steps in `ifeq`:

```make
.PHONY: k8s-platform
k8s-platform:
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Platform Layer ($(EVENTBUS)) ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_PLATFORM) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	# ... Envoy Gateway install (same on both buses) ...

ifeq ($(EVENTBUS),kafka)
	@printf "$(BOLD)[2/6] Installing Strimzi operator...$(NC)\n"
	# ... existing Strimzi steps ...
	@$(KUBECTL) apply -f deploy/platform/local/eventbus-kafka.yaml
else
	@printf "$(BOLD)[2/6] Installing NATS (JetStream)...$(NC)\n"
	@$(HELM) repo add nats https://nats-io.github.io/k8s/helm/charts/ --force-update 2>/dev/null || true
	@$(KUBECTL) create namespace $(K8S_NS_BOOKINFO) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@$(KUBECTL) apply -f deploy/platform/local/jetstream/nats-token-secret.yaml
	@$(HELM) upgrade --install nats nats/nats \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/jetstream/nats-values.yaml \
		--wait --timeout 180s
	@printf "  $(GREEN)NATS JetStream ready.$(NC)\n"
	@$(KUBECTL) apply -f deploy/platform/local/jetstream/eventbus-jetstream.yaml
	@printf "  $(GREEN)EventBus (jetstream) applied.$(NC)\n"
endif

	# ... Argo Events controller install (same on both buses) ...
	# ... Gateway base apply (same on both buses) ...
```

- [ ] **Step 5: Update `k8s-deploy` values-file selection**

Locate `k8s-deploy`. Change the per-service helm install line:

```make
		$(HELM) upgrade --install $$svc charts/bookinfo-service \
			--namespace $(K8S_NS_BOOKINFO) \
			$$gen \
			-f deploy/$$svc/values-local-$(EVENTBUS).yaml || exit 1; \
```

(Was `-f deploy/$$svc/values-local.yaml` — substitute `-$(EVENTBUS)`.)

- [ ] **Step 6: Update `stop-k8s` echo and `k8s-status` print**

Make sure both targets use `$(K8S_CLUSTER)` and announce which bus they're acting on.

- [ ] **Step 7: Test: dry-run kafka path**

```
make stop-k8s 2>/dev/null || true
EVENTBUS=kafka make -n run-k8s | head -50
```

Expected: dry-run output references `bookinfo-kafka-local` and `eventbus-kafka.yaml`.

- [ ] **Step 8: Test: dry-run jetstream path**

```
EVENTBUS=jetstream make -n run-k8s | head -50
```

Expected: references `bookinfo-jetstream-local`, `nats-values.yaml`, `eventbus-jetstream.yaml`.

- [ ] **Step 9: Commit**

```bash
git add Makefile
git commit -m "feat(make): EVENTBUS={kafka,jetstream} parameter for run-k8s

Adds per-bus cluster name (bookinfo-\$(EVENTBUS)-local), branches
k8s-platform on \$(EVENTBUS), selects values-local-\$(EVENTBUS).yaml
in k8s-deploy, and refuses to start if the other cluster exists."
```

## Task 5.6: Update CLAUDE.md cluster reference

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the "Context safety" line**

Find:

```
**Context safety:** All kubectl/helm calls use `--context=k3d-bookinfo-local`. Never mutates the user's active context.
```

Replace with:

```
**Context safety:** All kubectl/helm calls use `--context=k3d-bookinfo-$(EVENTBUS)-local` (default `kafka`). Never mutates the user's active context.
```

Also update the "Local Kubernetes" section header to mention `make run-k8s eventbus=jetstream` as an option.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(CLAUDE.md): document EVENTBUS=kafka|jetstream + per-bus cluster name"
```

## Task 5.7: Phase-5 verification gate (live cluster smoke)

- [ ] **Step 1: Bring up kafka cluster (default)**

```
make stop-k8s 2>/dev/null || true
make run-k8s
make k8s-status
```

Expected: cluster `bookinfo-kafka-local` up, all pods Ready, productpage reachable.

- [ ] **Step 2: Tear down + bring up jetstream cluster**

```
make stop-k8s
make run-k8s eventbus=jetstream
make k8s-status EVENTBUS=jetstream
```

Expected: cluster `bookinfo-jetstream-local` up, NATS pod Ready, all bookinfo pods Ready, productpage reachable.

- [ ] **Step 3: Smoke test event flow on jetstream cluster**

```
curl -X POST http://localhost:8080/v1/details \
  -H "Content-Type: application/json" \
  -d '{"title":"smoke","author":"a","year":2026,"type":"paperback"}'
sleep 5
kubectl --context=k3d-bookinfo-jetstream-local logs -n bookinfo deploy/details-write --tail=30
```

Expected: write recorded, reachable via GET.

- [ ] **Step 4: Verify Tempo trace**

Navigate Grafana → Tempo → search by service `details-write`. Expect a trace with producer + EventSource + Sensor spans connected.

If spans **don't connect** under jetstream, follow the argo-events fork update playbook (spec section "Risks and contingencies"). If they do connect, proceed.

- [ ] **Step 5: Tag**

```bash
git tag phase-5-complete-jetstream-eventbus
```

---

# Phase 6 — Per-service `values-local-{kafka,jetstream}.yaml` split

Goal of phase: deploy values are explicit per-bus. After phase 6, `make run-k8s eventbus=<bus>` selects the right file via Makefile changes from Phase 5.

## Task 6.1: Rename existing values to `-kafka` variant + add EVENT_BACKEND

For each of the 7 services (`details`, `reviews`, `ratings`, `ingestion`, `productpage`, `notification`, `dlqueue`):

**Files (per service):**
- Move: `deploy/<svc>/values-local.yaml` → `deploy/<svc>/values-local-kafka.yaml`

- [ ] **Step 1: Rename**

```bash
for svc in details reviews ratings ingestion productpage notification dlqueue; do
  git mv deploy/$svc/values-local.yaml deploy/$svc/values-local-kafka.yaml
done
```

- [ ] **Step 2: Add `EVENT_BACKEND: "kafka"` to each `config` block**

For each `deploy/<svc>/values-local-kafka.yaml`, edit the `config:` block to include:

```yaml
config:
  EVENT_BACKEND: "kafka"
  # ... existing keys ...
```

Also add the new `events.bus.type` (so the chart's bus.type evaluation is explicit; chart default is `kafka` but explicit is clearer):

```yaml
events:
  bus:
    type: kafka
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  # ... existing consumed: block stays unchanged ...
```

- [ ] **Step 3: Render-test**

```
helm template charts/bookinfo-service -f deploy/details/values-local-kafka.yaml -f deploy/details/values-generated.yaml --debug | head -40
```

Expected: parses, renders kafka EventSource.

- [ ] **Step 4: Commit (one combined commit is fine — change is mechanical)**

```bash
git add deploy/
git commit -m "refactor(deploy): rename values-local.yaml -> values-local-kafka.yaml

Adds EVENT_BACKEND=kafka to config and explicit events.bus.type=kafka."
```

## Task 6.2: Create `values-local-jetstream.yaml` per service

For each service:

**Files (per service):**
- Create: `deploy/<svc>/values-local-jetstream.yaml`

- [ ] **Step 1: Copy + modify**

```bash
cp deploy/details/values-local-kafka.yaml deploy/details/values-local-jetstream.yaml
```

Edit `deploy/details/values-local-jetstream.yaml`:

```diff
 config:
-  EVENT_BACKEND: "kafka"
-  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
+  EVENT_BACKEND: "jetstream"
+  NATS_URL: "nats://nats.platform.svc.cluster.local:4222"

 events:
   bus:
-    type: kafka
-  kafka:
-    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
+    type: jetstream
+  jetstream:
+    url: "nats://nats.platform.svc.cluster.local:4222"
+    tokenSecret:
+      name: nats-client-token
+      key: token
```

(`consumed:` blocks stay identical — the trigger spec is bus-agnostic. Postgres / observability / cqrs / sensor.dlq blocks unchanged.)

- [ ] **Step 2: Repeat for the other 6 services**

Same edits per service. Identify which services don't have `events.kafka.broker` (e.g. productpage, notification, dlqueue may have only `events.consumed`); their jetstream variant just sets `bus.type=jetstream` and `events.jetstream.{url, tokenSecret}` without a producer-side block.

- [ ] **Step 3: Render-test all**

```
for svc in details reviews ratings ingestion productpage notification dlqueue; do
  echo "=== $svc ==="
  helm template charts/bookinfo-service \
    -f deploy/$svc/values-local-jetstream.yaml \
    -f deploy/$svc/values-generated.yaml \
    --debug 2>&1 | grep -E "kind:|EVENT_BACKEND|NATS_URL|jetstream|kafka" | head -15
done
```

Expected: every service renders a `kind: EventSource` with `spec.jetstream.*` (where applicable) and `EVENT_BACKEND: "jetstream"` in the ConfigMap.

- [ ] **Step 4: Commit**

```bash
git add deploy/*/values-local-jetstream.yaml
git commit -m "feat(deploy): per-service values-local-jetstream.yaml

Mirrors values-local-kafka.yaml with bus.type=jetstream, NATS_URL,
and tokenSecret reference. consumed blocks unchanged
(bus-agnostic trigger specs)."
```

## Task 6.3: Phase-6 verification gate

- [ ] **Step 1: Full deploy on both clusters**

```
make stop-k8s 2>/dev/null || true
make run-k8s eventbus=kafka
# verify
make stop-k8s
make run-k8s eventbus=jetstream
# verify
```

Expected: both succeed.

- [ ] **Step 2: Tag**

```bash
git tag phase-6-complete-jetstream-eventbus
```

---

# Phase 7 — Documentation

## Task 7.1: Write `docs/kafka-eventbus.md`

**Files:**
- Create: `docs/kafka-eventbus.md`

- [ ] **Step 1: Write the doc**

```markdown
# Kafka EventBus in event-driven-bookinfo

This document covers how the Kafka path works in this repository. For the
NATS JetStream alternative, see [`jetstream-eventbus.md`](./jetstream-eventbus.md).

## Cluster topology

`make run-k8s eventbus=kafka` (the default) creates a k3d cluster named
`bookinfo-kafka-local` with these `platform`-namespace components:

- **Strimzi operator** (`strimzi/strimzi-kafka-operator`) — manages Kafka CRs.
- **Kafka cluster** (`Kafka` CR `bookinfo-kafka`, KRaft single-node) — exposes a bootstrap service at `bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092`.
- **Argo Events controller** (with custom CRDs from `argoproj/argo-events#3961`+`#3983`).
- **EventBus CR** named `default`, `spec.kafka.url` pointing at the bootstrap.

## EventBus shape

\`\`\`yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: default
  namespace: bookinfo
spec:
  kafka:
    url: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    topic: argo-events
    version: "4.2.0"
    consumerBatchMaxWait: "0"
\`\`\`

Source: `deploy/platform/local/eventbus-kafka.yaml`.

## Exposed-event flow

\`\`\`mermaid
flowchart LR
    Producer -->|JetStream-binary record| Topic[Kafka topic]
    Topic --> ESrc[kafka EventSource pod]
    ESrc --> EB[EventBus default]
    EB --> Sensor
    Sensor -->|HTTP trigger| Service
\`\`\`

The producer emits a CloudEvents-binary record with these headers:
`ce_specversion`, `ce_type`, `ce_source`, `ce_id`, `ce_time`,
`ce_subject`, `content-type`. The kafka EventSource (rendered from
`charts/bookinfo-service/templates/kafka-eventsource.yaml`) listens on the
topic and forwards each record into the Argo Events EventBus.

## ConfigMap env wiring

When `events.bus.type=kafka`, the chart emits two env vars on each pod:

- `EVENT_BACKEND=kafka`
- `KAFKA_BROKERS=<events.kafka.broker>`

`cmd/main.go` reads these and constructs a `kafkapub.Producer`.

## Idempotency + topic creation

`pkg/eventsmessaging/kafkapub/producer.go` calls `ensureTopic` on
startup. It tolerates "topic already exists" errors and creates the
topic with default partition / replication settings if missing.
Strimzi's auto-create-topics-on-produce setting also covers this case
in local dev.

## Where to look

- `pkg/eventsmessaging/kafkapub/` — Go producer.
- `pkg/eventsmessaging/publisher.go` — `Publisher` interface.
- `charts/bookinfo-service/templates/kafka-eventsource.yaml` — EventSource template.
- `deploy/platform/local/eventbus-kafka.yaml` — EventBus CR.
- `deploy/platform/local/kafka-cluster.yaml`, `kafka-nodepool.yaml`, `strimzi-values.yaml` — Strimzi platform.

## References

- [Strimzi Kafka operator](https://strimzi.io/)
- [franz-go](https://github.com/twmb/franz-go)
- [Argo Events Kafka EventSource](https://argoproj.github.io/argo-events/eventsources/setup/kafka/)
```

- [ ] **Step 2: Commit**

```bash
git add docs/kafka-eventbus.md
git commit -m "docs: how the Kafka EventBus path works in this repo"
```

## Task 7.2: Write `docs/jetstream-eventbus.md`

**Files:**
- Create: `docs/jetstream-eventbus.md`

- [ ] **Step 1: Write the doc**

```markdown
# NATS JetStream EventBus in event-driven-bookinfo

This document covers how the JetStream path works. For the Kafka alternative,
see [`kafka-eventbus.md`](./kafka-eventbus.md).

## Cluster topology

`make run-k8s eventbus=jetstream` creates a k3d cluster named
`bookinfo-jetstream-local` with these `platform`-namespace components:

- **NATS server** (Helm chart `nats-io/nats`, JetStream enabled, single replica, file storage on emptyDir) — exposes a `Service` `nats.platform.svc.cluster.local:4222`.
- **`nats-client-token` Secret** (mirrored into the `bookinfo` namespace) — shared token used by the EventBus, EventSources, and producer pods.
- **Argo Events controller** (same custom CRDs as the kafka cluster).
- **EventBus CR** named `default`, `spec.jetstreamExotic.url` pointing at NATS, `accessSecret` referencing the token secret.

## EventBus shape

\`\`\`yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: default
  namespace: bookinfo
spec:
  jetstreamExotic:
    url: nats://nats.platform.svc.cluster.local:4222
    accessSecret:
      name: nats-client-token
      key: token
    streamConfig: ""
\`\`\`

Source: `deploy/platform/local/jetstream/eventbus-jetstream.yaml`.

We deliberately use `jetstreamExotic` (pointing at our own NATS deployment)
rather than `jetstream` (which would have Argo Events run an embedded
in-EventBus NATS). Reasons: explicit topology, easier to inspect with
`nats` CLI, simpler observability story.

## Exposed-event flow

\`\`\`mermaid
flowchart LR
    Producer -->|JetStream PublishMsg| Stream[(JetStream stream)]
    Stream --> ESrc[jetstream EventSource pod]
    ESrc --> EB[EventBus default]
    EB --> Sensor
    Sensor -->|HTTP trigger| Service
\`\`\`

The producer publishes a NATS JetStream message with these headers:
`ce-specversion`, `ce-type`, `ce-source`, `ce-id`, `ce-time`,
`ce-subject`, `content-type`, plus `traceparent` for OTel propagation.

## Stream creation semantics

Unlike Strimzi+Kafka, JetStream does **not** auto-create streams on
publish. `pkg/eventsmessaging/natspub/producer.go` calls `ensureStream`
on startup using `js.AddStream` and tolerates the
"`stream name already in use`" error. The stream name and the publish
subject are equal (e.g. both `raw_books_details`).

If we ever scale to per-domain shared streams (e.g. `bookinfo.>`
subjects), the platform should switch to manifest-managed streams and
drop the producer-side ensure step.

## ConfigMap + secret env wiring

When `events.bus.type=jetstream`, the chart emits:

- `EVENT_BACKEND=jetstream` (ConfigMap)
- `NATS_URL=<events.jetstream.url>` (ConfigMap)
- `NATS_TOKEN` (env, sourced from `nats-client-token` secret via `valueFrom.secretKeyRef`)

`cmd/main.go` reads these and constructs a `natspub.Producer`.

## Where to look

- `pkg/eventsmessaging/natspub/` — Go producer.
- `pkg/eventsmessaging/publisher.go` — shared `Publisher` interface.
- `charts/bookinfo-service/templates/jetstream-eventsource.yaml` — EventSource template.
- `deploy/platform/local/jetstream/` — NATS values, token secret, EventBus.

## References

- [Argo Events JetStream EventSource](https://argoproj.github.io/argo-events/eventsources/setup/nats-streaming/)
- [nats.go JetStream API](https://pkg.go.dev/github.com/nats-io/nats.go/jetstream)
- [NATS JetStream concepts](https://docs.nats.io/nats-concepts/jetstream)

## Tracing risk

If trace spans don't connect across the JetStream EventSource pod,
follow the argo-events fork update playbook in
[`docs/superpowers/specs/2026-04-29-jetstream-eventbus-design.md`](./superpowers/specs/2026-04-29-jetstream-eventbus-design.md#risks-and-contingencies).
```

- [ ] **Step 2: Commit**

```bash
git add docs/jetstream-eventbus.md
git commit -m "docs: how the NATS JetStream EventBus path works in this repo"
```

## Task 7.3: Update `README.md`

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the architecture diagram annotation**

Find the top-level Mermaid diagram (around line 25 — the one with `Kafka[[Kafka EventBus]]`). Change:

```diff
-Kafka[[Kafka EventBus]]
+Bus[[Kafka **or** JetStream EventBus]]
```

(and update node references throughout that diagram from `Kafka` to `Bus`.)

- [ ] **Step 2: Add "Choosing an EventBus" subsection**

Under the "Local Kubernetes" section, after the current intro paragraph, add:

```markdown
### Choosing an EventBus

This repo supports two interchangeable EventBus implementations:

- **Kafka** (default): `make run-k8s eventbus=kafka` (or just `make run-k8s`). Strimzi operator + Kafka KRaft single-node. See [`docs/kafka-eventbus.md`](docs/kafka-eventbus.md).
- **NATS JetStream**: `make run-k8s eventbus=jetstream`. Standalone NATS Helm chart with JetStream enabled, EventBus uses `jetstreamExotic`. See [`docs/jetstream-eventbus.md`](docs/jetstream-eventbus.md).

The two clusters (`bookinfo-kafka-local`, `bookinfo-jetstream-local`)
are mutually exclusive — start one, `make stop-k8s` it before starting
the other. The runtime image is identical; backend selection happens
at pod startup via the `EVENT_BACKEND` env var.
```

- [ ] **Step 3: Update the make-target table cell for `run-k8s`**

```diff
-| `make run-k8s` | Full local k8s setup (cluster, platform, observability, deploy) |
+| `make run-k8s [eventbus=jetstream]` | Full local k8s setup; defaults to kafka, jetstream available as alternative |
```

- [ ] **Step 4: Update the cluster name reference**

Search for `bookinfo-local` and update:

```diff
-make k8s-status        # Pod status + access URLs
+make k8s-status        # Pod status + access URLs (defaults to kafka cluster; pass eventbus=jetstream for the other)
```

- [ ] **Step 5: Mention the dual-server AsyncAPI output**

In the AsyncAPI/specgen section (search for "asyncapi"), add a note:

```markdown
The generated `services/*/api/asyncapi.yaml` declares both `kafka` and
`jetstream` server entries — downstream tooling can render the same
channel against either bus.
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs(README): document EVENTBUS=kafka|jetstream + dual-server AsyncAPI"
```

## Task 7.4: Phase-7 verification gate

- [ ] **Step 1: Render docs locally (sanity check)**

Open `docs/kafka-eventbus.md` and `docs/jetstream-eventbus.md` in a Markdown preview; confirm Mermaid diagrams + links resolve. (No CLI step required.)

- [ ] **Step 2: Tag**

```bash
git tag phase-7-complete-jetstream-eventbus
```

---

# Phase 8 — CI + final verification

## Task 8.1: Update `helm-lint-test.yml` to run ct against both bus variants

**Files:**
- Modify: `.github/workflows/helm-lint-test.yml`

- [ ] **Step 1: Read the current workflow**

```
cat .github/workflows/helm-lint-test.yml
```

(Identify how `ct lint` and `ct install` are invoked — typically via `helm/chart-testing-action`.)

- [ ] **Step 2: Add a matrix or duplicate jobs**

Two options:

**(a) Matrix:**

```yaml
strategy:
  matrix:
    bus: [kafka, jetstream]
steps:
  # ... checkout, setup ct ...
  - name: ct lint (${{ matrix.bus }})
    run: |
      ct lint --charts charts/bookinfo-service \
        --target-branch main \
        --validate-maintainers=false \
        --check-version-increment=false \
        --helm-extra-set-args "--set events.bus.type=${{ matrix.bus }}"
```

**(b) Sequential steps in one job:**

```yaml
- name: ct lint (kafka fixtures)
  run: ct lint ... # uses ci/values-*-kafka if naming convention adopted, else default ci/ directory
- name: ct lint (jetstream fixtures)
  run: ct lint ... # picks up ci/values-*-jetstream
```

(`chart-testing` discovers all `ci/*.yaml` files automatically — both the existing kafka fixtures and the new jetstream ones get linted in one pass already. Verify by running `ct lint` locally before and after; if both fixture sets are picked up, this task is a docs-only change to the workflow comment. If not, add the matrix.)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/helm-lint-test.yml
git commit -m "ci: lint chart against both kafka and jetstream values fixtures"
```

## Task 8.2: Final end-to-end verification

- [ ] **Step 1: Full local sweep**

```
make build-all
make test
make lint
make helm-lint
```

Expected: PASS all four.

- [ ] **Step 2: Live cluster test — kafka**

```
make stop-k8s 2>/dev/null || true
make run-k8s eventbus=kafka
make k8s-status
# Smoke: POST + GET via productpage
curl -X POST http://localhost:8080/v1/details \
  -H "Content-Type: application/json" \
  -d '{"title":"final-kafka","author":"a","year":2026,"type":"paperback"}'
sleep 5
curl http://localhost:8080/ | grep -i "final-kafka" || echo "FAIL: not found"
```

Expected: PASS.

- [ ] **Step 3: Live cluster test — jetstream**

```
make stop-k8s
make run-k8s eventbus=jetstream
make k8s-status EVENTBUS=jetstream
curl -X POST http://localhost:8080/v1/details \
  -H "Content-Type: application/json" \
  -d '{"title":"final-jetstream","author":"a","year":2026,"type":"paperback"}'
sleep 5
curl http://localhost:8080/ | grep -i "final-jetstream" || echo "FAIL: not found"
```

Expected: PASS.

- [ ] **Step 4: Tempo trace check on jetstream**

In Grafana → Tempo, search by service `details-write` after the final POST. Verify the trace contains:
- producer span (`jetstream.publish`)
- jetstream EventSource span
- Sensor span
- HTTP trigger span (POST to `/v1/details`)

If spans aren't connected, follow the contingency playbook (rebase the argo-events fork's `feat/cloudevents-compliance-otel-tracing` on `argoproj:master`, resolve `go.mod`/`go.sum` via `go mod tidy`, add the JetStream tracing fix, cherry-pick to `feat/combined-prs-3961-3983`, `make image-multi`, push to `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`, force-push the fork branch with `--force-with-lease`).

- [ ] **Step 5: Tag final**

```bash
git tag phase-8-complete-jetstream-eventbus
git tag jetstream-eventbus-feature-complete
```

- [ ] **Step 6: Push branch**

```bash
git push -u origin feat/jetstream-eventbus
```

- [ ] **Step 7: Open PR**

```bash
gh pr create --title "feat: NATS JetStream as alternative EventBus" --body "$(cat <<'EOF'
## Summary

Adds NATS JetStream as a second, interchangeable EventBus alongside Kafka.
Selectable at `make run-k8s eventbus=jetstream` time; deploys to a
dedicated k3d cluster (`bookinfo-jetstream-local`). Application services
run from the same image and pick backend at startup via `EVENT_BACKEND`.

Spec: `docs/superpowers/specs/2026-04-29-jetstream-eventbus-design.md`
Plan: `docs/superpowers/plans/2026-04-29-jetstream-eventbus.md`

## Test plan

- [x] `make build-all && make test && make lint && make helm-lint`
- [x] `make run-k8s eventbus=kafka` — full smoke (POST+GET+Tempo trace)
- [x] `make run-k8s eventbus=jetstream` — full smoke (POST+GET+Tempo trace)
- [x] AsyncAPI dual-server output committed
- [x] CLAUDE.md + README.md updated
EOF
)"
```

---

## Done

After Phase 8, the feature is complete and pushed. Open the PR and request review.
