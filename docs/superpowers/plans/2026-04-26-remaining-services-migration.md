# Remaining Services Migration to API Spec Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the API spec generation rollout by migrating the remaining six services (`ratings` AsyncAPI side, `notification`, `dlqueue`, `ingestion`, `details`, `reviews`) to the declarative slice pattern established by PR #55, and document `productpage` as an explicit catalog exception. After this plan, `make generate-specs` reports `OK` for all six migrated services with no `SKIPPED` messages, every `cqrs.endpoints.<key>.{method,endpoint}` and `events.exposed.<key>.{contentType,eventTypes}` block in the repo is sourced from generated YAML, and every Kafka producer reads its CloudEvents headers from a `[]events.Descriptor` slice instead of hardcoded `const` blocks.

**Architecture:** Same two declarative slices per service introduced by PR #55 — `Endpoints []api.Endpoint` in `services/<svc>/internal/adapter/inbound/http/endpoints.go`, and `Exposed []events.Descriptor` in `services/<svc>/internal/adapter/outbound/kafka/exposed.go`. Each kafka producer grows a single generic `Publish(ctx, descriptor, payload)` method whose CE headers are sourced from the descriptor; the existing typed `Publish<EventName>` methods become thin wrappers over `Publish` that pick the right element from `Exposed[…]`. `productpage` is intentionally skipped from the catalog because it is a BFF (HTML + HTMX) with a non-hex layout and no service-to-service REST surface — the spec is amended to record the exception.

**Tech Stack:** Go 1.25 (existing module), the `pkg/api` and `pkg/events` packages from PR #55, the `tools/specgen` binary from PR #55, the `make generate-specs` / `make lint-specs` / `make diff-specs` targets from PR #55. No new external dependencies.

**Spec reference:** `docs/superpowers/specs/2026-04-25-api-spec-generation-design.md` (this plan also adds a "Skipped: productpage" section to that spec)

**Foundation reference:** `docs/superpowers/plans/2026-04-25-api-spec-generation-foundation.md` (PR #55 — Tasks 1–18 are merged-ready when this plan begins)

**Repo:** `/Users/kaio.fellipe/Documents/git/others/go-http-server`
**Branch base:** `main`, rebased onto post-#55 `main` once that PR is merged. Open this work as a single feature branch (one PR shipping all six migrations).

**Out of scope:** SDK generation, semver coupling to per-service git tags, contract tests — all deferred per the spec's non-goals.

---

## File Structure

```text
# AsyncAPI completion for ratings (Phase 1)
services/ratings/internal/adapter/outbound/kafka/
├── exposed.go                                # NEW — Exposed []events.Descriptor + RatingSubmittedPayload export
├── producer.go                               # MODIFIED — drop ce* consts, add Publish(ctx, d, payload), refactor PublishRatingSubmitted
└── producer_test.go                          # MODIFIED — assert headers come from descriptor
services/ratings/api/asyncapi.yaml            # NEW (generated)
services/ratings/api/catalog-info.yaml        # MODIFIED (generated — gains -events API entity)

# notification HTTP (Phase 2)
services/notification/internal/adapter/inbound/http/
├── endpoints.go                              # NEW
└── handler.go                                # MODIFIED — RegisterRoutes via api.Register
services/notification/api/openapi.yaml        # NEW (generated)
services/notification/api/catalog-info.yaml   # NEW (generated)
deploy/notification/values-generated.yaml     # NEW (generated — empty cqrs block; notification has no CQRS)

# dlqueue HTTP (Phase 3)
services/dlqueue/internal/adapter/inbound/http/
├── endpoints.go                              # NEW
└── handler.go                                # MODIFIED — preserve stdhttp alias; RegisterRoutes via api.Register
services/dlqueue/api/openapi.yaml             # NEW (generated)
services/dlqueue/api/catalog-info.yaml        # NEW (generated)
deploy/dlqueue/values-generated.yaml          # NEW (generated — owns cqrs.endpoints.dlq-event-received)
deploy/dlqueue/values-local.yaml              # MODIFIED — strip method/endpoint subkeys

# ingestion full migration (Phase 4)
services/ingestion/internal/adapter/inbound/http/
├── endpoints.go                              # NEW
└── handler.go                                # MODIFIED
services/ingestion/internal/adapter/outbound/kafka/
├── exposed.go                                # NEW — exports BookEvent, declares Exposed
├── producer.go                               # MODIFIED — drop ce* consts, add Publish, refactor PublishBookAdded
└── producer_test.go                          # MODIFIED
services/ingestion/api/openapi.yaml           # NEW (generated)
services/ingestion/api/asyncapi.yaml          # NEW (generated)
services/ingestion/api/catalog-info.yaml      # NEW (generated)
deploy/ingestion/values-generated.yaml        # NEW (generated)
deploy/ingestion/values-local.yaml            # MODIFIED — strip contentType from raw-books-details

# details full migration (Phase 5)
services/details/internal/adapter/inbound/http/
├── endpoints.go                              # NEW
└── handler.go                                # MODIFIED
services/details/internal/adapter/outbound/kafka/
├── exposed.go                                # NEW — exports BookAddedPayload, declares Exposed
├── producer.go                               # MODIFIED
└── producer_test.go                          # MODIFIED
services/details/api/openapi.yaml             # NEW (generated)
services/details/api/asyncapi.yaml            # NEW (generated)
services/details/api/catalog-info.yaml        # NEW (generated)
deploy/details/values-generated.yaml          # NEW (generated)
deploy/details/values-local.yaml              # MODIFIED — strip method/endpoint + contentType/eventTypes

# reviews full migration (Phase 6)
services/reviews/internal/adapter/inbound/http/
├── endpoints.go                              # NEW
└── handler.go                                # MODIFIED
services/reviews/internal/adapter/outbound/kafka/
├── exposed.go                                # NEW — exports ReviewSubmittedPayload, ReviewDeletedPayload, declares Exposed (2 descriptors share ExposureKey "events")
├── producer.go                               # MODIFIED
└── producer_test.go                          # MODIFIED
services/reviews/api/openapi.yaml             # NEW (generated)
services/reviews/api/asyncapi.yaml            # NEW (generated)
services/reviews/api/catalog-info.yaml        # NEW (generated)
deploy/reviews/values-generated.yaml          # NEW (generated — eventTypes union of both CETypes)
deploy/reviews/values-local.yaml              # MODIFIED — strip both endpoint blocks' method/endpoint and the exposed.events.contentType/eventTypes

# productpage exception (Phase 7)
docs/superpowers/specs/2026-04-25-api-spec-generation-design.md  # MODIFIED — append "Skipped: productpage" section

# Verification (Phase 8)
# All generated artifacts re-checked under one command; no further file changes expected.
```

---

## Phase 1 — `ratings` AsyncAPI completion

PR #55 migrated the ratings HTTP side (`endpoints.go` + refactored `RegisterRoutes`) but left the ratings producer with a hardcoded `ceType`/`ceSource`/`ceVersion` block and no `exposed.go`. This phase finishes ratings, validating the producer-refactor pattern that the next four producers reuse.

### Task 1: Export the producer payload struct

**Files:**

- Modify: `services/ratings/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Read the current producer**

Open `services/ratings/internal/adapter/outbound/kafka/producer.go` and confirm the unexported payload struct is named `submittedBody` (lines 30–36). The new exported name will be `RatingSubmittedPayload`. The producer's public method is `PublishRatingSubmitted(ctx, evt domain.RatingSubmittedEvent)`.

- [ ] **Step 2: Rename `submittedBody` → `RatingSubmittedPayload`**

In `services/ratings/internal/adapter/outbound/kafka/producer.go`, rename the struct and update its single use in `PublishRatingSubmitted`:

```go
// RatingSubmittedPayload is the marshaled Kafka record value for a
// rating-submitted CloudEvent. Exported because the events.Descriptor in
// exposed.go references it as a JSONSchema source for tools/specgen.
type RatingSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key"`
}
```

In `PublishRatingSubmitted`, change `body := submittedBody{` to `body := RatingSubmittedPayload{`.

- [ ] **Step 3: Run existing producer tests to verify no regression**

Run:

```bash
go test ./services/ratings/internal/adapter/outbound/kafka/... -race -count=1
```

Expected: PASS — the rename is a pure refactor.

- [ ] **Step 4: Commit**

```bash
git add services/ratings/internal/adapter/outbound/kafka/producer.go
git commit -s -m "refactor(ratings/kafka): export RatingSubmittedPayload

Renames the unexported submittedBody to RatingSubmittedPayload so the
upcoming events.Descriptor in exposed.go can reference it as a
JSONSchema source for tools/specgen. No behavioral change."
```

---

### Task 2: Add `exposed.go` and refactor producer to use a generic `Publish`

**Files:**

- Create: `services/ratings/internal/adapter/outbound/kafka/exposed.go`
- Modify: `services/ratings/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Create `exposed.go`**

Create `services/ratings/internal/adapter/outbound/kafka/exposed.go`:

```go
package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer reads
// each Descriptor to build CE headers; tools/specgen reads the same slice
// to derive services/ratings/api/asyncapi.yaml and the events.exposed
// block in deploy/ratings/values-generated.yaml.
var Exposed = []events.Descriptor{
	{
		Name:        "rating-submitted",
		ExposureKey: "events",
		CEType:      "com.bookinfo.ratings.rating-submitted",
		CESource:    "ratings",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     RatingSubmittedPayload{},
		Description: "Emitted after a successful SubmitRating call.",
	},
}
```

- [ ] **Step 2: Add a generic `Publish(ctx, d, payload)` method on `Producer`**

In `services/ratings/internal/adapter/outbound/kafka/producer.go`, drop the `const` block (the four lines `ceTypeRatingSubmitted = …`, `ceSource = …`, `ceVersion = …`, and the surrounding `const (` / `)`) — the partition/replication constants stay. Replace with:

```go
const (
	defaultPartitions        = 3
	defaultReplicationFactor = 1
)
```

Add the generic publisher just below `NewProducerWithClient`:

```go
// Publish marshals payload to JSON, builds a CloudEvents-binary Kafka
// record using the descriptor's CEType/CESource/Version, and produces it
// to the producer's configured topic. recordKey is the partition key
// (typically the natural-key field of payload, e.g. ProductID); when
// empty, idempotencyKey is used as a fallback.
func (p *Producer) Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", d.Name, err)
	}

	keyBytes := []byte(recordKey)
	if len(keyBytes) == 0 {
		keyBytes = []byte(idempotencyKey)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   keyBytes,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(d.Version)},
			{Key: "ce_type", Value: []byte(d.CEType)},
			{Key: "ce_source", Value: []byte(d.CESource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(idempotencyKey)},
			{Key: "content-type", Value: []byte(d.ContentType)},
		},
	}

	ctx, span := telemetry.StartProducerSpan(ctx, p.topic, idempotencyKey)
	defer span.End()

	telemetry.InjectTraceContext(ctx, record)

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published event",
		"topic", p.topic,
		"ce_type", d.CEType,
		"idempotency_key", idempotencyKey,
	)
	return nil
}
```

Add the `pkg/events` import to the imports block (it sorts under the third group with the other `pkg/` imports):

```go
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
```

- [ ] **Step 3: Reduce `PublishRatingSubmitted` to a typed wrapper**

Replace the body of `PublishRatingSubmitted`:

```go
// PublishRatingSubmitted sends a rating-submitted CloudEvent to Kafka.
// Thin typed wrapper around Publish; the descriptor is the single source
// of truth for CE headers (exposed.go).
func (p *Producer) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	body := RatingSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Stars:          evt.Stars,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, Exposed[0], body, evt.ProductID, evt.IdempotencyKey)
}
```

`Exposed[0]` is safe here because `ratings` publishes exactly one CE type. Future ratings event types should be picked by `Name` via a small lookup helper rather than by index.

- [ ] **Step 4: Run producer tests**

Run:

```bash
go test ./services/ratings/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: every existing test (`TestPublishRatingSubmitted_Headers`, etc.) continues to PASS. The header values are identical; only their *origin* moved from constants to the descriptor.

- [ ] **Step 5: Commit**

```bash
git add services/ratings/internal/adapter/outbound/kafka/exposed.go services/ratings/internal/adapter/outbound/kafka/producer.go
git commit -s -m "refactor(ratings/kafka): descriptor-driven Publish

Replaces the hardcoded ce* const block with a Descriptor in exposed.go
and a generic Producer.Publish that reads its CE headers from the
descriptor. PublishRatingSubmitted is now a typed wrapper that picks
Exposed[0] and constructs the typed RatingSubmittedPayload.

The slice in exposed.go is the single source of truth for both runtime
header construction and tools/specgen — drift would surface as test
failures or AsyncAPI lint failures before the service could be
deployed."
```

---

### Task 3: Generate ratings AsyncAPI artifacts

**Files:**

- Create: `services/ratings/api/asyncapi.yaml`
- Modify: `services/ratings/api/catalog-info.yaml`
- Modify: `deploy/ratings/values-generated.yaml`
- Modify: `deploy/ratings/values-local.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: ratings OK` (and `specgen: <other-svc> SKIPPED: …` for the other six). Creates `services/ratings/api/asyncapi.yaml`, updates `services/ratings/api/catalog-info.yaml` to include the `kind: API` `ratings-events` entity, and rewrites `deploy/ratings/values-generated.yaml` to include the `events.exposed.events.{contentType,eventTypes}` block in addition to the existing `cqrs.endpoints.rating-submitted` block.

- [ ] **Step 2: Inspect the generated AsyncAPI**

Run:

```bash
cat services/ratings/api/asyncapi.yaml
cat services/ratings/api/catalog-info.yaml
cat deploy/ratings/values-generated.yaml
```

Expected: `asyncapi.yaml` declares one channel keyed by `events` with one message named `rating-submitted`; `catalog-info.yaml` lists `ratings-rest` and `ratings-events` under `providesApis`; `values-generated.yaml` contains both `cqrs.endpoints.rating-submitted` and `events.exposed.events`.

- [ ] **Step 3: Strip the now-duplicated subkeys from values-local.yaml**

In `deploy/ratings/values-local.yaml`, the `events.exposed.events` block currently has four subkeys (`topic`, `eventBusName`, `contentType`, `eventTypes`). The generator owns `contentType` and `eventTypes`. Update the file to keep only the local-only subkeys:

```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_ratings_events
      eventBusName: kafka
```

(The `cqrs.endpoints.rating-submitted` block in this file is already minimal — it has `port` and `triggers`, both local-only — so it stays unchanged.)

- [ ] **Step 4: Verify lint passes**

Run:

```bash
make lint-specs
```

Expected: spectral reports `0 errors`. Warnings about missing `operationId`/`tags`/`description` are acceptable per the foundation's policy.

- [ ] **Step 5: Verify Helm still renders with both files**

Run:

```bash
helm dependency build charts/bookinfo-service > /dev/null
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-generated.yaml \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo > /tmp/ratings-render.yaml
echo "render exit: $?"
grep -A4 'eventTypes:' /tmp/ratings-render.yaml | head -10
```

Expected: `render exit: 0`; the rendered manifests show `eventTypes: [com.bookinfo.ratings.rating-submitted]` from the generated values.

- [ ] **Step 6: Commit**

```bash
git add services/ratings/api/asyncapi.yaml services/ratings/api/catalog-info.yaml deploy/ratings/values-generated.yaml deploy/ratings/values-local.yaml
git commit -s -m "feat(ratings): generated asyncapi.yaml + catalog -events entity

Adds the AsyncAPI half of ratings' catalog: services/ratings/api/asyncapi.yaml
declares the events channel with the rating-submitted message;
catalog-info.yaml gains the ratings-events API entity;
values-generated.yaml owns events.exposed.events.{contentType,eventTypes}.

Strips the now-duplicated contentType/eventTypes subkeys from
deploy/ratings/values-local.yaml (the local file keeps only topic and
eventBusName)."
```

---

## Phase 2 — `notification` HTTP migration

`notification` is the simplest HTTP migration: three routes, no kafka producer, no `cqrs.endpoints` block (sensors call its `POST /v1/notifications` directly via `events.consumed`, not through the gateway). The generated `values-generated.yaml` will be empty for this service — `tools/specgen` already handles that case by emitting an empty file with the standard banner.

### Task 4: Add `Endpoints` slice and refactor `RegisterRoutes`

**Files:**

- Create: `services/notification/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/notification/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create `endpoints.go`**

Create `services/notification/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/notification/api/openapi.yaml.
//
// POST /v1/notifications has no EventName because notification is not
// behind the CQRS gateway split; sensors invoke this endpoint directly
// via events.consumed in deploy/notification/values-local.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "POST",
		Path:     "/v1/notifications",
		Summary:  "Dispatch a notification (called by Argo Events sensors)",
		Request:  DispatchNotificationRequest{},
		Response: NotificationResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/notifications/{id}",
		Summary:  "Get a single notification by ID",
		Response: NotificationResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/notifications",
		Summary:  "List notifications for a recipient (?recipient=<email>)",
		Response: NotificationsListResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
```

- [ ] **Step 2: Refactor `RegisterRoutes`**

In `services/notification/internal/adapter/inbound/http/handler.go`, add `pkg/api` to the imports:

```go
import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
)
```

Replace the body of `RegisterRoutes`:

```go
// RegisterRoutes registers the notification routes on the given mux by
// looping over the Endpoints slice in endpoints.go — single source of
// truth for runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"POST /v1/notifications":      h.dispatch,
		"GET /v1/notifications/{id}":  h.getByID,
		"GET /v1/notifications":       h.listByRecipient,
	})
}
```

- [ ] **Step 3: Run existing handler tests**

Run:

```bash
go test ./services/notification/... -race -count=1 -v
```

Expected: every existing handler test continues to PASS — `api.Register` panics at startup if any route in the slice has no handler, so the suite catches drift implicitly.

- [ ] **Step 4: Commit**

```bash
git add services/notification/internal/adapter/inbound/http/endpoints.go services/notification/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(notification): declare Endpoints slice; loop in RegisterRoutes

Routes now derive from a single declarative slice that tools/specgen
will read to generate services/notification/api/openapi.yaml. POST
/v1/notifications has no EventName because notification is invoked by
sensors directly (events.consumed), not through the CQRS gateway split."
```

---

### Task 5: Generate notification artifacts

**Files:**

- Create: `services/notification/api/openapi.yaml`
- Create: `services/notification/api/catalog-info.yaml`
- Create: `deploy/notification/values-generated.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: notification OK` in addition to ratings; the four unmigrated services still print `SKIPPED`. Creates the three artifacts under `services/notification/api/` and `deploy/notification/`.

- [ ] **Step 2: Inspect the generated files**

Run:

```bash
ls -la services/notification/api/ deploy/notification/values-generated.yaml
cat services/notification/api/openapi.yaml | head -30
cat deploy/notification/values-generated.yaml
```

Expected: `openapi.yaml` declares three operations (one POST, two GET); `catalog-info.yaml` has `notification-rest` only (no `-events` entity, no kafka producer); `values-generated.yaml` contains only the banner — `cqrs.endpoints` is empty because none of the endpoints carry an `EventName`, and `events.exposed` is empty because there's no kafka producer.

- [ ] **Step 3: Verify lint and helm template**

Run:

```bash
make lint-specs
helm template notification charts/bookinfo-service \
  -f deploy/notification/values-generated.yaml \
  -f deploy/notification/values-local.yaml \
  --namespace bookinfo > /tmp/notification-render.yaml
echo "render exit: $?"
```

Expected: spectral `0 errors`; `render exit: 0`.

- [ ] **Step 4: Commit**

```bash
git add services/notification/api deploy/notification/values-generated.yaml
git commit -s -m "feat(notification): generated openapi.yaml + catalog-info

Adds the notification service's OpenAPI 3.1 catalog entry and Backstage
catalog-info. values-generated.yaml is essentially empty (no CQRS, no
kafka producer) but is committed for consistency — the dual-f Helm
install in the Makefile expects every service to have one."
```

---

## Phase 3 — `dlqueue` HTTP migration

`dlqueue` has eight routes — the most of any service — and uses an aliased import `stdhttp "net/http"` because it predates the `pkg/api` package. The refactor must preserve the alias. Only `POST /v1/events` carries an `EventName` (it's the lone CQRS-routed POST, mapped to `cqrs.endpoints.dlq-event-received`); the other seven routes are direct admin operations.

### Task 6: Add `Endpoints` slice (preserving the `stdhttp` alias)

**Files:**

- Create: `services/dlqueue/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/dlqueue/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create `endpoints.go`**

Create `services/dlqueue/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/dlqueue/api/openapi.yaml.
//
// Only POST /v1/events carries an EventName — it is the destination of
// the dlq-event-received Sensor. The replay/resolve/reset/batch routes
// are direct admin operations not behind the CQRS split, so EventName is
// left empty for them.
var Endpoints = []api.Endpoint{
	{
		Method:    "POST",
		Path:      "/v1/events",
		Summary:   "Ingest a failed event from a Sensor's dlqTrigger",
		EventName: "dlq-event-received",
		Request:   IngestEventRequest{},
		Response:  DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/events",
		Summary:  "List DLQ events with filters and pagination",
		Response: ListEventsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/events/{id}",
		Summary:  "Get a single DLQ event by ID",
		Response: DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "POST",
		Path:     "/v1/events/{id}/replay",
		Summary:  "Replay a failed event back to its original target",
		Response: DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "POST",
		Path:     "/v1/events/{id}/resolve",
		Summary:  "Mark an event as resolved (terminal state)",
		Request:  ResolveRequest{},
		Response: DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "POST",
		Path:     "/v1/events/{id}/reset",
		Summary:  "Reset a poisoned event back to pending",
		Response: DLQEventResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "POST",
		Path:     "/v1/events/batch/replay",
		Summary:  "Replay all events matching a filter",
		Request:  BatchReplayRequest{},
		Response: BatchActionResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "POST",
		Path:     "/v1/events/batch/resolve",
		Summary:  "Resolve a batch of events by IDs",
		Request:  BatchResolveRequest{},
		Response: BatchActionResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
```

- [ ] **Step 2: Refactor `RegisterRoutes` (preserve `stdhttp` alias)**

In `services/dlqueue/internal/adapter/inbound/http/handler.go`, the existing imports already use `stdhttp "net/http"` (line 8). Add `pkg/api` to the imports below it (sorted under the project group):

```go
import (
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)
```

Replace the body of `RegisterRoutes` (note: the parameter type stays `*stdhttp.ServeMux`, and the handlers map values stay `stdhttp.HandlerFunc` — `pkg/api`'s `Register` is generic over the `http.ServeMux`/`http.HandlerFunc` from the standard `net/http` package, which is what `stdhttp` aliases):

```go
// RegisterRoutes registers all dlqueue routes on the given mux by looping
// over the Endpoints slice in endpoints.go — single source of truth for
// runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *stdhttp.ServeMux) {
	api.Register(mux, Endpoints, map[string]stdhttp.HandlerFunc{
		"POST /v1/events":                  h.ingestEvent,
		"GET /v1/events":                   h.listEvents,
		"GET /v1/events/{id}":              h.getEvent,
		"POST /v1/events/{id}/replay":      h.replayEvent,
		"POST /v1/events/{id}/resolve":     h.resolveEvent,
		"POST /v1/events/{id}/reset":       h.resetPoisoned,
		"POST /v1/events/batch/replay":     h.batchReplay,
		"POST /v1/events/batch/resolve":    h.batchResolve,
	})
}
```

The other handler methods inside this file already use `stdhttp.ResponseWriter` / `stdhttp.Request` and stay unchanged.

- [ ] **Step 3: Run existing tests**

Run:

```bash
go test ./services/dlqueue/... -race -count=1 -v
```

Expected: every existing handler/integration test continues to PASS.

- [ ] **Step 4: Commit**

```bash
git add services/dlqueue/internal/adapter/inbound/http/endpoints.go services/dlqueue/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(dlqueue): declare Endpoints slice; preserve stdhttp alias

Routes now derive from a single declarative slice. The stdhttp alias on
net/http is preserved (the handler predates pkg/api so the file already
disambiguated the package name); api.Register is parameterized on the
stdlib mux/handler types so it works transparently with the alias.

Only POST /v1/events carries an EventName (dlq-event-received) — the
seven admin routes (replay/resolve/reset/batch + read endpoints) are
direct calls outside the CQRS gateway split."
```

---

### Task 7: Generate dlqueue artifacts and strip values-local

**Files:**

- Create: `services/dlqueue/api/openapi.yaml`
- Create: `services/dlqueue/api/catalog-info.yaml`
- Create: `deploy/dlqueue/values-generated.yaml`
- Modify: `deploy/dlqueue/values-local.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: dlqueue OK`. Creates `services/dlqueue/api/openapi.yaml` (eight operations), `services/dlqueue/api/catalog-info.yaml`, and `deploy/dlqueue/values-generated.yaml`.

- [ ] **Step 2: Inspect the generated values**

Run:

```bash
cat deploy/dlqueue/values-generated.yaml
```

Expected:

```yaml
# DO NOT EDIT — generated by tools/specgen from
#   services/dlqueue/internal/adapter/inbound/http/endpoints.go
# Run `make generate-specs` to refresh.
cqrs:
  endpoints:
    dlq-event-received:
      method: POST
      endpoint: /v1/events
```

- [ ] **Step 3: Strip the now-duplicated subkeys from values-local.yaml**

In `deploy/dlqueue/values-local.yaml`, the existing `cqrs.endpoints.dlq-event-received` block has four subkeys (`port`, `method`, `endpoint`, `triggers`). Drop `method` and `endpoint`:

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
    dlq-event-received:
      port: 12004
      triggers:
        - name: ingest-dlq-event
          url: self
          payload:
            - passthrough
```

- [ ] **Step 4: Verify lint and helm template**

Run:

```bash
make lint-specs
helm template dlqueue charts/bookinfo-service \
  -f deploy/dlqueue/values-generated.yaml \
  -f deploy/dlqueue/values-local.yaml \
  --namespace bookinfo > /tmp/dlqueue-render.yaml
echo "render exit: $?"
grep -B1 -A2 'method: POST' /tmp/dlqueue-render.yaml | head -10
```

Expected: spectral `0 errors`; `render exit: 0`; the merged values still produce `method: POST` and `endpoint: /v1/events` from the generated file deep-merged with the local `port: 12004` and `triggers: …`.

- [ ] **Step 5: Commit**

```bash
git add services/dlqueue/api deploy/dlqueue/values-generated.yaml deploy/dlqueue/values-local.yaml
git commit -s -m "feat(dlqueue): generated openapi.yaml + catalog-info + values-generated

values-generated.yaml owns cqrs.endpoints.dlq-event-received.method and
.endpoint. The local file keeps port and triggers (deploy-time
concerns). Helm dual-f deep-merges both at install."
```

---

## Phase 4 — `ingestion` full migration

`ingestion` is the first migration that exercises both halves on a non-`ratings` service: it has two HTTP endpoints (admin trigger + status read) and one Kafka event (`book-added`). The kafka producer follows the same refactor pattern from Phase 1. Note the unexported payload struct here is `bookEvent` (not `submittedBody`); it becomes `BookEvent`.

### Task 8: Migrate ingestion HTTP

**Files:**

- Create: `services/ingestion/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/ingestion/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create `endpoints.go`**

Create `services/ingestion/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/ingestion/api/openapi.yaml.
//
// ingestion has no CQRS endpoint — the trigger and status routes are
// direct admin calls; events are published to Kafka by the producer
// loop, not by an HTTP handler.
var Endpoints = []api.Endpoint{
	{
		Method:   "POST",
		Path:     "/v1/ingestion/trigger",
		Summary:  "Trigger a one-shot scrape (optionally overriding queries)",
		Request:  TriggerRequest{},
		Response: ScrapeResultResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 409, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/ingestion/status",
		Summary:  "Get current scraper state and last result",
		Response: StatusResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
```

- [ ] **Step 2: Refactor `RegisterRoutes`**

In `services/ingestion/internal/adapter/inbound/http/handler.go`, add `pkg/api` to the imports:

```go
import (
	"encoding/json"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/port"
)
```

Replace the body of `RegisterRoutes`:

```go
// RegisterRoutes registers the ingestion routes on the given mux by
// looping over the Endpoints slice in endpoints.go — single source of
// truth for runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"POST /v1/ingestion/trigger": h.triggerScrape,
		"GET /v1/ingestion/status":   h.getStatus,
	})
}
```

- [ ] **Step 3: Run handler tests**

Run:

```bash
go test ./services/ingestion/internal/adapter/inbound/http/... -race -count=1 -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add services/ingestion/internal/adapter/inbound/http/endpoints.go services/ingestion/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(ingestion): declare Endpoints slice; loop in RegisterRoutes

Routes now derive from a single declarative slice. ingestion has no
CQRS endpoint (the trigger/status calls bypass the gateway split — they
are direct admin/observability routes), so neither POST carries an
EventName."
```

---

### Task 9: Refactor ingestion producer to descriptor-driven `Publish`

**Files:**

- Create: `services/ingestion/internal/adapter/outbound/kafka/exposed.go`
- Modify: `services/ingestion/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Export `bookEvent` → `BookEvent`**

In `services/ingestion/internal/adapter/outbound/kafka/producer.go`, rename `bookEvent` (lines 32–43) to `BookEvent` and update its single use in `PublishBookAdded` (line 89: `evt := bookEvent{` → `evt := BookEvent{`). Add a doc comment:

```go
// BookEvent is the marshaled Kafka record value for a book-added
// CloudEvent. Its JSON shape matches details' AddDetailRequest, since
// the details Sensor uses passthrough payload. Exported so the
// events.Descriptor in exposed.go can reference it as a JSONSchema
// source for tools/specgen.
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
```

- [ ] **Step 2: Create `exposed.go`**

Create `services/ingestion/internal/adapter/outbound/kafka/exposed.go`:

```go
package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/ingestion/api/asyncapi.yaml and the
// events.exposed block in deploy/ingestion/values-generated.yaml.
//
// ExposureKey "raw-books-details" matches the existing chart key in
// deploy/ingestion/values-local.yaml — the EventSource bound to the
// raw_books_details Kafka topic. The CE type is a logical content type
// (book-added), but the Helm grouping key is the topic-derived name.
var Exposed = []events.Descriptor{
	{
		Name:        "book-added",
		ExposureKey: "raw-books-details",
		CEType:      "com.bookinfo.ingestion.book-added",
		CESource:    "ingestion",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     BookEvent{},
		Description: "Emitted for every Book scraped from Open Library.",
	},
}
```

- [ ] **Step 3: Replace the producer's hardcoded consts with `Publish`**

In `services/ingestion/internal/adapter/outbound/kafka/producer.go`, drop the `ceType`/`ceSource`/`ceVersion` constants from the `const (` block (lines 22–24) so only the topic-creation constants remain:

```go
const (
	defaultPartitions        = 3
	defaultReplicationFactor = 1
)
```

Add `pkg/events` to the imports block. Add the same generic `Publish` method introduced in Phase 1, Task 2 Step 2 (inserted just below `NewProducerWithClient`):

```go
// Publish marshals payload to JSON, builds a CloudEvents-binary Kafka
// record using the descriptor's CEType/CESource/Version, and produces it
// to the producer's configured topic. recordKey is the partition key
// (typically the natural-key field of payload, e.g. ISBN); when empty,
// idempotencyKey is used as a fallback.
func (p *Producer) Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", d.Name, err)
	}

	keyBytes := []byte(recordKey)
	if len(keyBytes) == 0 {
		keyBytes = []byte(idempotencyKey)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   keyBytes,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(d.Version)},
			{Key: "ce_type", Value: []byte(d.CEType)},
			{Key: "ce_source", Value: []byte(d.CESource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(idempotencyKey)},
			{Key: "content-type", Value: []byte(d.ContentType)},
		},
	}

	ctx, span := telemetry.StartProducerSpan(ctx, p.topic, idempotencyKey)
	defer span.End()

	telemetry.InjectTraceContext(ctx, record)

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published event",
		"topic", p.topic,
		"ce_type", d.CEType,
		"idempotency_key", idempotencyKey,
	)
	return nil
}
```

Reduce `PublishBookAdded` to a typed wrapper. The previous body computed an `idempotencyKey := fmt.Sprintf("ingestion-isbn-%s", book.ISBN)` inside the payload — keep that same logic and pass the same value as the wrapper's idempotency-key argument so the `ce_subject` header is unchanged from the pre-refactor record:

```go
// PublishBookAdded sends a book-added CloudEvent to Kafka. Thin typed
// wrapper around Publish; the descriptor is the single source of truth
// for CE headers (exposed.go).
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

	return p.Publish(ctx, Exposed[0], evt, book.ISBN, idempotencyKey)
}
```

- [ ] **Step 4: Run producer tests**

Run:

```bash
go test ./services/ingestion/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: every existing test continues to PASS. The CE header values are byte-identical to the pre-refactor record for any given input book.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/kafka/exposed.go services/ingestion/internal/adapter/outbound/kafka/producer.go
git commit -s -m "refactor(ingestion/kafka): descriptor-driven Publish

Drops the hardcoded ce* const block and replaces it with Exposed[0] in
exposed.go. PublishBookAdded becomes a typed wrapper around the new
generic Publish that reads its CE headers from the descriptor. The
idempotencyKey derivation (ingestion-isbn-<ISBN>) is preserved so
ce_subject remains byte-identical for downstream consumers.

ExposureKey 'raw-books-details' aligns with the existing Helm chart key
for the EventSource bound to the raw_books_details topic."
```

---

### Task 10: Generate ingestion artifacts and strip values-local

**Files:**

- Create: `services/ingestion/api/openapi.yaml`
- Create: `services/ingestion/api/asyncapi.yaml`
- Create: `services/ingestion/api/catalog-info.yaml`
- Create: `deploy/ingestion/values-generated.yaml`
- Modify: `deploy/ingestion/values-local.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: ingestion OK`. Creates the three artifacts under `services/ingestion/api/` and `deploy/ingestion/values-generated.yaml`.

- [ ] **Step 2: Inspect outputs**

Run:

```bash
cat deploy/ingestion/values-generated.yaml
cat services/ingestion/api/asyncapi.yaml | head -40
```

Expected: `values-generated.yaml` declares only `events.exposed.raw-books-details.{contentType,eventTypes}` (no `cqrs.endpoints` block, since neither HTTP route has an `EventName`). `asyncapi.yaml` has one channel `raw-books-details` with one message `book-added`.

- [ ] **Step 3: Strip the now-duplicated `contentType` from values-local.yaml**

In `deploy/ingestion/values-local.yaml`, the existing `events.exposed.raw-books-details` block has three subkeys (`topic`, `eventBusName`, `contentType`). Drop `contentType`:

```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    raw-books-details:
      topic: raw_books_details
      eventBusName: kafka
```

- [ ] **Step 4: Verify lint and helm template**

Run:

```bash
make lint-specs
helm template ingestion charts/bookinfo-service \
  -f deploy/ingestion/values-generated.yaml \
  -f deploy/ingestion/values-local.yaml \
  --namespace bookinfo > /tmp/ingestion-render.yaml
echo "render exit: $?"
grep -A3 'eventTypes:' /tmp/ingestion-render.yaml | head -10
```

Expected: spectral `0 errors`; `render exit: 0`; rendered `eventTypes: [com.bookinfo.ingestion.book-added]`.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/api deploy/ingestion/values-generated.yaml deploy/ingestion/values-local.yaml
git commit -s -m "feat(ingestion): generated openapi.yaml + asyncapi.yaml + catalog-info

The HTTP side (admin trigger + status) is documented in openapi.yaml
without any cqrs.endpoints entries (no EventName — these routes bypass
the CQRS gateway split). The Kafka side adds one channel, one message.

Strips the now-duplicated contentType from
deploy/ingestion/values-local.yaml; topic and eventBusName remain."
```

---

## Phase 5 — `details` full migration

`details` is the first migration where the same service migrates both halves AND continues to consume events. The `events.consumed.raw-books-details` block in `deploy/details/values-local.yaml` stays unchanged — `tools/specgen` only owns the *exposed* side. The unexported payload struct here is `bookAddedBody` (not `bookEvent` like ingestion); it becomes `BookAddedPayload`.

### Task 11: Migrate details HTTP

**Files:**

- Create: `services/details/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/details/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create `endpoints.go`**

Create `services/details/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/details/api/openapi.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/details",
		Summary:  "List all book details",
		Response: []DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/details/{id}",
		Summary:  "Get a single book detail by ID",
		Response: DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/details",
		Summary:   "Add a new book detail",
		EventName: "book-added",
		Request:   AddDetailRequest{},
		Response:  DetailResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
}
```

- [ ] **Step 2: Refactor `RegisterRoutes`**

In `services/details/internal/adapter/inbound/http/handler.go`, add `pkg/api` to the imports:

```go
import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
)
```

Replace `RegisterRoutes`:

```go
// RegisterRoutes registers the details routes on the given mux by
// looping over the Endpoints slice in endpoints.go — single source of
// truth for runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"GET /v1/details":       h.listDetails,
		"GET /v1/details/{id}":  h.getDetail,
		"POST /v1/details":      h.addDetail,
	})
}
```

- [ ] **Step 3: Run handler tests**

Run:

```bash
go test ./services/details/internal/adapter/inbound/http/... -race -count=1 -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add services/details/internal/adapter/inbound/http/endpoints.go services/details/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(details): declare Endpoints slice; loop in RegisterRoutes

Routes now derive from a single declarative slice. POST /v1/details
carries EventName=book-added (matches the existing
cqrs.endpoints.book-added block in values-local.yaml). The two GET
routes have no EventName."
```

---

### Task 12: Refactor details producer to descriptor-driven `Publish`

**Files:**

- Create: `services/details/internal/adapter/outbound/kafka/exposed.go`
- Modify: `services/details/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Export `bookAddedBody` → `BookAddedPayload`**

In `services/details/internal/adapter/outbound/kafka/producer.go`, rename `bookAddedBody` (lines 31–43) to `BookAddedPayload` and update its sole use in `PublishBookAdded` (line 87: `body := bookAddedBody{` → `body := BookAddedPayload{`):

```go
// BookAddedPayload is the marshaled Kafka record value for a book-added
// CloudEvent. Exported because the events.Descriptor in exposed.go
// references it as a JSONSchema source for tools/specgen.
type BookAddedPayload struct {
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
```

- [ ] **Step 2: Create `exposed.go`**

Create `services/details/internal/adapter/outbound/kafka/exposed.go`:

```go
package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/details/api/asyncapi.yaml and the
// events.exposed block in deploy/details/values-generated.yaml.
//
// ExposureKey "events" matches the existing chart key in
// deploy/details/values-local.yaml — one EventSource publishing all
// details domain events from the bookinfo_details_events topic.
var Exposed = []events.Descriptor{
	{
		Name:        "book-added",
		ExposureKey: "events",
		CEType:      "com.bookinfo.details.book-added",
		CESource:    "details",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     BookAddedPayload{},
		Description: "Emitted after a successful AddDetail call.",
	},
}
```

- [ ] **Step 3: Replace consts with `Publish` and reduce `PublishBookAdded`**

In `services/details/internal/adapter/outbound/kafka/producer.go`, drop the `ceTypeBookAdded` / `ceSource` / `ceVersion` constants (lines 22–24); add `pkg/events` to the imports; add the same generic `Publish` method introduced in Phase 1 Task 2 Step 2 (verbatim — same signature).

Reduce `PublishBookAdded` to a typed wrapper. The previous record key was `evt.IdempotencyKey`; preserve that as the `recordKey` argument:

```go
// PublishBookAdded sends a book-added CloudEvent to Kafka. Thin typed
// wrapper around Publish; the descriptor is the single source of truth
// for CE headers (exposed.go).
func (p *Producer) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	body := BookAddedPayload{
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
	return p.Publish(ctx, Exposed[0], body, evt.IdempotencyKey, evt.IdempotencyKey)
}
```

(`recordKey` and `idempotencyKey` are deliberately the same here — pre-refactor the producer used `evt.IdempotencyKey` for both `Key` and `ce_subject`.)

- [ ] **Step 4: Run producer tests**

Run:

```bash
go test ./services/details/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/details/internal/adapter/outbound/kafka/exposed.go services/details/internal/adapter/outbound/kafka/producer.go
git commit -s -m "refactor(details/kafka): descriptor-driven Publish

Drops the hardcoded ce* const block and adds Exposed in exposed.go.
PublishBookAdded becomes a typed wrapper that picks Exposed[0] (details
publishes a single CE type today; future events should be picked by
Name via a small helper). Record key and ce_subject preserved as
evt.IdempotencyKey to keep byte-identical CE records."
```

---

### Task 13: Generate details artifacts and strip values-local

**Files:**

- Create: `services/details/api/openapi.yaml`
- Create: `services/details/api/asyncapi.yaml`
- Create: `services/details/api/catalog-info.yaml`
- Create: `deploy/details/values-generated.yaml`
- Modify: `deploy/details/values-local.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: details OK`.

- [ ] **Step 2: Inspect generated values**

Run:

```bash
cat deploy/details/values-generated.yaml
```

Expected:

```yaml
# DO NOT EDIT — generated by tools/specgen from
#   services/details/internal/adapter/inbound/http/endpoints.go
#   services/details/internal/adapter/outbound/kafka/exposed.go
# Run `make generate-specs` to refresh.
cqrs:
  endpoints:
    book-added:
      method: POST
      endpoint: /v1/details
events:
  exposed:
    events:
      contentType: application/json
      eventTypes:
        - com.bookinfo.details.book-added
```

- [ ] **Step 3: Strip generated subkeys from values-local.yaml**

In `deploy/details/values-local.yaml`:
- Drop `cqrs.endpoints.book-added.method` and `.endpoint` (keep `port`, `triggers`).
- Drop `events.exposed.events.contentType` and `.eventTypes` (keep `topic`, `eventBusName`).
- The `events.consumed.raw-books-details` block stays unchanged — `tools/specgen` only owns the exposed side.

After:

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
      triggers:
        - name: create-detail
          url: self
          payload:
            - passthrough

# (sensor / gateway blocks unchanged)

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_details_events
      eventBusName: kafka
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

- [ ] **Step 4: Verify lint and helm template**

Run:

```bash
make lint-specs
helm template details charts/bookinfo-service \
  -f deploy/details/values-generated.yaml \
  -f deploy/details/values-local.yaml \
  --namespace bookinfo > /tmp/details-render.yaml
echo "render exit: $?"
grep -B1 -A3 'method: POST' /tmp/details-render.yaml | head -10
grep -A3 'eventTypes:' /tmp/details-render.yaml | head -5
```

Expected: spectral `0 errors`; `render exit: 0`; rendered manifests show both the `cqrs.endpoints.book-added.method=POST/endpoint=/v1/details` (from generated) and `eventTypes: [com.bookinfo.details.book-added]` (from generated), plus the local `port: 12000` and `triggers` (from local).

- [ ] **Step 5: Commit**

```bash
git add services/details/api deploy/details/values-generated.yaml deploy/details/values-local.yaml
git commit -s -m "feat(details): generated openapi.yaml + asyncapi.yaml + catalog-info

values-generated.yaml owns cqrs.endpoints.book-added.{method,endpoint}
and events.exposed.events.{contentType,eventTypes}.
deploy/details/values-local.yaml keeps deploy-time concerns (port,
triggers, topic, eventBusName) and the events.consumed.raw-books-details
block (consumed events stay in local — only exposed is generated)."
```

---

## Phase 6 — `reviews` full migration

`reviews` is the first migration with **two** events sharing one `ExposureKey`, so the `eventTypes` array in the generated values-yaml will be a union of both CETypes. It also has three HTTP endpoints, two of which carry `EventName` (review-submitted, review-deleted). Both producer-side payload structs (`submittedBody`, `deletedBody`) need to be exported.

### Task 14: Migrate reviews HTTP

**Files:**

- Create: `services/reviews/internal/adapter/inbound/http/endpoints.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler.go`

- [ ] **Step 1: Create `endpoints.go`**

Create `services/reviews/internal/adapter/inbound/http/endpoints.go`:

```go
package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// DeleteReviewRequest is the JSON body for POST /v1/reviews/delete.
// Hoisted out of the handler's anonymous struct so tools/specgen can
// resolve it as a JSONSchema.
type DeleteReviewRequest struct {
	ReviewID string `json:"review_id"`
}

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/reviews/api/openapi.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/reviews/{id}",
		Summary:  "Get reviews for a product (paginated)",
		Response: ProductReviewsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/reviews",
		Summary:   "Submit a new review",
		EventName: "review-submitted",
		Request:   SubmitReviewRequest{},
		Response:  ReviewResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/reviews/delete",
		Summary:   "Delete a review by ID (Sensor-routed command)",
		EventName: "review-deleted",
		Request:   DeleteReviewRequest{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
```

(The `Response` field is omitted on the delete route because the handler returns `204 No Content`. The OpenAPI builder treats nil `Response` as no body, which matches the wire behavior.)

- [ ] **Step 2: Refactor `handler.go` to use the hoisted DTO and `api.Register`**

In `services/reviews/internal/adapter/inbound/http/handler.go`, add `pkg/api` to the imports:

```go
import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)
```

Replace the `RegisterRoutes` body:

```go
// RegisterRoutes registers the reviews routes on the given mux by
// looping over the Endpoints slice in endpoints.go — single source of
// truth for runtime routing and OpenAPI generation.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	api.Register(mux, Endpoints, map[string]http.HandlerFunc{
		"GET /v1/reviews/{id}":      h.getProductReviews,
		"POST /v1/reviews":          h.submitReview,
		"POST /v1/reviews/delete":   h.deleteReview,
	})
}
```

In `deleteReview`, replace the anonymous struct decode with the named DTO:

```go
func (h *Handler) deleteReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req DeleteReviewRequest
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

- [ ] **Step 3: Run handler tests**

Run:

```bash
go test ./services/reviews/internal/adapter/inbound/http/... -race -count=1 -v
```

Expected: PASS — the anonymous struct's wire shape is preserved (one `review_id` JSON field), so existing tests continue to pass.

- [ ] **Step 4: Commit**

```bash
git add services/reviews/internal/adapter/inbound/http/endpoints.go services/reviews/internal/adapter/inbound/http/handler.go
git commit -s -m "refactor(reviews): declare Endpoints slice; hoist DeleteReviewRequest

Routes now derive from a single declarative slice. The previously
anonymous delete request struct is hoisted to a named DeleteReviewRequest
type so tools/specgen can resolve it as a JSONSchema. Wire shape is
unchanged — existing handler tests still pass."
```

---

### Task 15: Refactor reviews producer to descriptor-driven `Publish` (two descriptors)

**Files:**

- Create: `services/reviews/internal/adapter/outbound/kafka/exposed.go`
- Modify: `services/reviews/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Export `submittedBody` and `deletedBody`**

In `services/reviews/internal/adapter/outbound/kafka/producer.go`:

```go
// ReviewSubmittedPayload is the marshaled Kafka record value for a
// review-submitted CloudEvent.
type ReviewSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ReviewDeletedPayload is the marshaled Kafka record value for a
// review-deleted CloudEvent.
type ReviewDeletedPayload struct {
	ReviewID       string `json:"review_id"`
	ProductID      string `json:"product_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}
```

Update `PublishReviewSubmitted` and `PublishReviewDeleted` to use the renamed types (they already refer to `submittedBody` and `deletedBody` literals — the rename is mechanical).

- [ ] **Step 2: Create `exposed.go` with two descriptors sharing one ExposureKey**

Create `services/reviews/internal/adapter/outbound/kafka/exposed.go`:

```go
package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/reviews/api/asyncapi.yaml and the
// events.exposed.events block in deploy/reviews/values-generated.yaml.
//
// Both descriptors share ExposureKey "events" — one EventSource
// publishes both CE types from the bookinfo_reviews_events topic. The
// generator emits a union list of CETypes under events.exposed.events.eventTypes.
var Exposed = []events.Descriptor{
	{
		Name:        "review-submitted",
		ExposureKey: "events",
		CEType:      "com.bookinfo.reviews.review-submitted",
		CESource:    "reviews",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ReviewSubmittedPayload{},
		Description: "Emitted after a successful SubmitReview call.",
	},
	{
		Name:        "review-deleted",
		ExposureKey: "events",
		CEType:      "com.bookinfo.reviews.review-deleted",
		CESource:    "reviews",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ReviewDeletedPayload{},
		Description: "Emitted after a successful DeleteReview call.",
	},
}
```

- [ ] **Step 3: Replace consts with `Publish` and add a small descriptor lookup**

In `services/reviews/internal/adapter/outbound/kafka/producer.go`, drop the `ceTypeReviewSubmitted` / `ceTypeReviewDeleted` / `ceSource` / `ceVersion` constants. Add `pkg/events` to imports.

Reviews already has a private `produce(ctx, ceType, key, partitionHint, body)` helper today — replace it with the same generic `Publish` introduced in Phase 1 (verbatim signature). Both `PublishReviewSubmitted` and `PublishReviewDeleted` then become tiny wrappers that pick the descriptor by `Name` (because `Exposed[i]` order is fragile when there are multiple descriptors):

```go
// descriptorFor returns the descriptor whose Name matches; panics if
// Exposed is missing the named descriptor (drift between exposed.go
// and the producer surfaces immediately).
func descriptorFor(name string) events.Descriptor {
	for _, d := range Exposed {
		if d.Name == name {
			return d
		}
	}
	panic(fmt.Sprintf("kafka: no descriptor for event %q", name))
}

// PublishReviewSubmitted sends a review-submitted CloudEvent to Kafka.
func (p *Producer) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	body := ReviewSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Text:           evt.Text,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, descriptorFor("review-submitted"), body, evt.ProductID, evt.IdempotencyKey)
}

// PublishReviewDeleted sends a review-deleted CloudEvent to Kafka.
func (p *Producer) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	body := ReviewDeletedPayload{
		ReviewID:       evt.ReviewID,
		ProductID:      evt.ProductID,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, descriptorFor("review-deleted"), body, evt.ProductID, evt.IdempotencyKey)
}
```

Add the same generic `Publish` method as in Phase 1 Task 2 Step 2. Delete the old private `produce` method — `Publish` supersedes it.

- [ ] **Step 4: Run producer tests**

Run:

```bash
go test ./services/reviews/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: PASS — record key (`evt.ProductID` falling back to `evt.IdempotencyKey`) and headers are byte-identical to the pre-refactor records.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/outbound/kafka/exposed.go services/reviews/internal/adapter/outbound/kafka/producer.go
git commit -s -m "refactor(reviews/kafka): descriptor-driven Publish + descriptorFor lookup

Drops the hardcoded ce* const block and replaces the private produce
helper with a generic Publish method. Both PublishReviewSubmitted and
PublishReviewDeleted pick their descriptor by Name via descriptorFor —
not by Exposed[i] index — because reviews has two descriptors sharing
ExposureKey 'events'. Future event additions are append-safe.

ReviewSubmittedPayload and ReviewDeletedPayload are exported so
tools/specgen can resolve them as JSONSchema sources."
```

---

### Task 16: Generate reviews artifacts and strip values-local

**Files:**

- Create: `services/reviews/api/openapi.yaml`
- Create: `services/reviews/api/asyncapi.yaml`
- Create: `services/reviews/api/catalog-info.yaml`
- Create: `deploy/reviews/values-generated.yaml`
- Modify: `deploy/reviews/values-local.yaml`

- [ ] **Step 1: Run the generator**

Run:

```bash
make generate-specs
```

Expected: prints `specgen: reviews OK`. All six services should now report OK; productpage is filtered out by `DiscoverServices` because it has no `endpoints.go`.

- [ ] **Step 2: Inspect generated values**

Run:

```bash
cat deploy/reviews/values-generated.yaml
```

Expected:

```yaml
# DO NOT EDIT — generated by tools/specgen from
#   services/reviews/internal/adapter/inbound/http/endpoints.go
#   services/reviews/internal/adapter/outbound/kafka/exposed.go
# Run `make generate-specs` to refresh.
cqrs:
  endpoints:
    review-submitted:
      method: POST
      endpoint: /v1/reviews
    review-deleted:
      method: POST
      endpoint: /v1/reviews/delete
events:
  exposed:
    events:
      contentType: application/json
      eventTypes:
        - com.bookinfo.reviews.review-submitted
        - com.bookinfo.reviews.review-deleted
```

- [ ] **Step 3: Strip generated subkeys from values-local.yaml**

In `deploy/reviews/values-local.yaml`:
- Drop `cqrs.endpoints.review-submitted.method` and `.endpoint`.
- Drop `cqrs.endpoints.review-deleted.method` and `.endpoint`.
- Drop `events.exposed.events.contentType` and `.eventTypes`.

After:

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
    review-submitted:
      port: 12001
      triggers:
        - name: create-review
          url: self
          payload:
            - passthrough
    review-deleted:
      port: 12003
      triggers:
        - name: delete-review-write
          url: self
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id

# (sensor / gateway blocks unchanged)

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    events:
      topic: bookinfo_reviews_events
      eventBusName: kafka
```

- [ ] **Step 4: Verify lint and helm template**

Run:

```bash
make lint-specs
helm template reviews charts/bookinfo-service \
  -f deploy/reviews/values-generated.yaml \
  -f deploy/reviews/values-local.yaml \
  --namespace bookinfo > /tmp/reviews-render.yaml
echo "render exit: $?"
grep -B1 -A4 'review-submitted' /tmp/reviews-render.yaml | head -20
grep -B1 -A4 'review-deleted' /tmp/reviews-render.yaml | head -20
grep -A3 'eventTypes:' /tmp/reviews-render.yaml | head -10
```

Expected: spectral `0 errors`; `render exit: 0`; both endpoints have full `method/endpoint/port/triggers` assembled from the deep-merge; `eventTypes` lists both CETypes.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/api deploy/reviews/values-generated.yaml deploy/reviews/values-local.yaml
git commit -s -m "feat(reviews): generated openapi.yaml + asyncapi.yaml + catalog-info

values-generated.yaml owns both cqrs.endpoints.{review-submitted,review-deleted}.{method,endpoint}
and the union events.exposed.events.eventTypes list (two CETypes
sharing one ExposureKey). values-local.yaml keeps deploy concerns
(port, triggers, topic, eventBusName)."
```

---

## Phase 7 — `productpage` exception documentation

`productpage` is intentionally skipped from the generated catalog. This phase amends the design spec with an explicit exception section and adds no code changes.

### Task 17: Document the productpage exception in the spec

**Files:**

- Modify: `docs/superpowers/specs/2026-04-25-api-spec-generation-design.md`

- [ ] **Step 1: Append the exception section**

At the end of `docs/superpowers/specs/2026-04-25-api-spec-generation-design.md`, append (after the existing "Risks & open questions" section):

```markdown
## Skipped: productpage

`productpage` is the only service in this monorepo that does not
participate in the generated API catalog. The decision is intentional:

- **It is a BFF (HTML + HTMX), not a service-to-service API surface.**
  The handlers under `services/productpage/internal/handler/` return
  `text/html` for the user-facing pages and HTMX partials. The single
  REST endpoint `GET /v1/products/{id}` is consumed by productpage's own
  frontend (HTMX poll), not by other services in the system.
- **Its layout does not match the discovery contract.** `tools/specgen`
  walks `services/<svc>/internal/adapter/inbound/http/endpoints.go` and
  `services/<svc>/internal/adapter/outbound/kafka/exposed.go`.
  productpage uses a flat `internal/handler/` layout (no hex-arch
  inbound/outbound split) and has no kafka producer, so neither slice
  has a natural home there without restructuring the package.
- **The Backstage event-catalog scaffolding template targets
  cross-team integration use cases.** A team browsing the catalog and
  clicking "configure to my service" wants to wire a *consumer* to one
  of the events exposed elsewhere in the system. productpage exposes
  none of the events the template cares about (no kafka producer); its
  HTML responses are not actionable in that flow.
- **Practical rollback path.** Anyone needing to surface productpage's
  REST endpoint in Backstage can either (a) extend the specgen walker
  to discover an alternative `endpoints.go` location and map
  `text/html` operations to OpenAPI 3.1's `responses.<status>.content
  ['text/html']` form, or (b) refactor productpage to the canonical
  hex-arch layout and migrate it like the others. Neither is in scope
  for the initial rollout.

`productpage` therefore has no `services/productpage/api/` directory
and no `deploy/productpage/values-generated.yaml`. The Makefile's dual-
`-f` Helm install (`make k8s-deploy`) already short-circuits on missing
`values-generated.yaml`, so productpage's install line falls back to
the single `-f deploy/productpage/values-local.yaml` form transparently.
The CI `specs-drift` job naturally ignores productpage because the
runner's `DiscoverServices` only enumerates services that have one of
the declarative slices.
```

- [ ] **Step 2: Verify markdown lints (no broken links, valid headers)**

Run:

```bash
python3 -c "
import re, sys
with open('docs/superpowers/specs/2026-04-25-api-spec-generation-design.md') as f:
    content = f.read()
assert '## Skipped: productpage' in content, 'section not appended'
assert content.count('# API Spec Generation') == 1, 'top heading duplicated'
print('ok')
"
```

Expected: `ok`.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-04-25-api-spec-generation-design.md
git commit -s -m "docs(specs/api-spec-generation): append productpage exception

productpage is a BFF (HTML + HTMX) with a non-hex-arch layout and no
service-to-service REST surface; it is intentionally skipped from the
generated catalog. The exception is now documented in the spec
alongside the existing Goals / Architecture / Migration sections.

No code change — make generate-specs already filters out services
without endpoints.go / exposed.go, so productpage is naturally
excluded from the runner's output."
```

---

## Phase 8 — Bulk regenerate, verify drift & lint, deploy verification

This phase regenerates everything once more (catching any cross-service ordering drift), runs every gate, and verifies the dual-`-f` Helm install works end-to-end against a live k3d cluster.

### Task 18: Bulk regenerate and assert no drift

**Files:** none modified (any drift detected here triggers a fix-and-commit cycle).

- [ ] **Step 1: Run the full generator**

Run:

```bash
make generate-specs
```

Expected output:

```
specgen: details OK
specgen: dlqueue OK
specgen: ingestion OK
specgen: notification OK
specgen: ratings OK
specgen: reviews OK
```

(Order may differ — `DiscoverServices` returns alphabetical from `os.ReadDir`. There must be no `SKIPPED` lines.)

- [ ] **Step 2: Verify zero drift**

Run:

```bash
git diff --exit-code
echo "drift exit: $?"
```

Expected: `drift exit: 0`. If non-zero, the drift must be fixed: regenerate, inspect the diff, fold any necessary committed YAML into the relevant phase's commit (`git commit --amend` is allowed *only on the most recent commit if no push has happened yet*; otherwise add a `chore: fix specs drift` follow-up commit).

- [ ] **Step 3: Run spectral across every spec**

Run:

```bash
make lint-specs
```

Expected: spectral reports `0 errors` for every `services/*/api/openapi.yaml` and `services/*/api/asyncapi.yaml`. Warnings about missing `operationId`/`tags`/`description` are acceptable per the foundation policy.

- [ ] **Step 4: Run oasdiff against origin/main**

Run:

```bash
make diff-specs
```

Expected: every new spec is reported as `NEW (not on origin/main, skipping)`. The ratings spec should report no breaking changes (its `info.version` is `1.0.0` and the routes are unchanged from PR #55). If `oasdiff` reports any breaking change for ratings, investigate — it should be impossible.

- [ ] **Step 5: Run the full Go test suite**

Run:

```bash
go test ./... -race -count=1
```

Expected: PASS for every package. The producer refactors in Phases 1, 4, 5, 6 should not change any test outcome — they preserve byte-identical CE records.

- [ ] **Step 6: Run golangci-lint**

Run:

```bash
make lint
```

Expected: `0 issues`.

- [ ] **Step 7 (optional, no commit if everything is clean)**

If Steps 1–6 produce no diffs and no failures, skip to Task 19. Otherwise, commit any accumulated drift fixes:

```bash
git add -p   # carefully review every hunk
git commit -s -m "chore(specs): drift fixes after bulk regenerate"
```

---

### Task 19: End-to-end verification on local k3d

**Files:** none modified — verification only.

- [ ] **Step 1: Bring up (or refresh) the local cluster**

Run:

```bash
make k8s-status >/dev/null 2>&1 || make run-k8s
```

Expected: `make run-k8s` runs to completion (k3d cluster, platform, observability, apps); pod logs are visible. `make k8s-status` lists all 11 expected deployments (`productpage`, `details`, `details-write`, `reviews`, `reviews-write`, `ratings`, `ratings-write`, `notification`, `dlqueue`, `dlqueue-write`, `ingestion`).

- [ ] **Step 2: Force a rebuild + redeploy of every service**

Run:

```bash
make k8s-rebuild
```

Expected: every service gets rebuilt; each `helm upgrade --install` deep-merges `deploy/<svc>/values-generated.yaml` with `deploy/<svc>/values-local.yaml`; every Deployment reaches `Available`. No CrashLoopBackOff; no Helm template render failures.

- [ ] **Step 3: Smoke each backend service through the gateway**

Run:

```bash
# ratings: GET, POST (CQRS roundtrip), GET
curl -fsS http://localhost:8080/v1/ratings/product-1 | jq .
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"product-1","reviewer":"alice","stars":5}' \
  http://localhost:8080/v1/ratings | jq .
sleep 2
curl -fsS http://localhost:8080/v1/ratings/product-1 | jq .

# reviews: paginated GET, POST, POST delete
curl -fsS 'http://localhost:8080/v1/reviews/product-1?page=1&page_size=10' | jq .
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"product-1","reviewer":"alice","text":"good"}' \
  http://localhost:8080/v1/reviews | jq .
sleep 2
curl -fsS 'http://localhost:8080/v1/reviews/product-1' | jq .

# details: list, GET, POST
curl -fsS http://localhost:8080/v1/details | jq '. | length'
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"title":"X","author":"Y","year":2026,"type":"paperback","pages":1,"publisher":"P","language":"en","isbn_10":"","isbn_13":"9999999999999"}' \
  http://localhost:8080/v1/details | jq .

# notification: list (must accept ?recipient=…)
curl -fsS 'http://localhost:8080/v1/notifications?recipient=system@bookinfo' | jq '.notifications | length'

# dlqueue: list (should be empty in a healthy cluster)
curl -fsS http://localhost:8080/v1/events | jq '.total_items'

# ingestion: status (no scrape forced — just confirm reachability)
curl -fsS http://localhost:8080/v1/ingestion/status | jq .state
```

Expected: every call returns 2xx; the ratings/reviews/details POSTs propagate via Sensor → write Deployment → DB; the second GET for ratings returns the new entry with `count: 1`; reviews second GET shows the new review; details POST returns 201 with the new ID.

- [ ] **Step 4: Tail Tempo for the event chain on a fresh details add**

Run:

```bash
# In one terminal: open Grafana
open http://localhost:3000  # macOS; otherwise xdg-open

# In another, trigger:
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"title":"Trace Probe","author":"E2E","year":2026,"type":"paperback","pages":1,"publisher":"P","language":"en","isbn_10":"","isbn_13":"9999999999998"}' \
  http://localhost:8080/v1/details | jq .id
```

Expected: in Tempo, a single trace tying `details (gateway POST)` → `EventSource` → `Sensor (book-added trigger)` → `details-write (POST /v1/details)` → `details-write (Kafka producer span emitting com.bookinfo.details.book-added)` → `notification-write (POST /v1/notifications, attribute filter ce-type=com.bookinfo.details.book-added)`. Span attributes for the producer span come from the `events.Descriptor` in `exposed.go`.

- [ ] **Step 5: Re-run the drift gate after the cluster traffic**

Run:

```bash
make generate-specs
git diff --exit-code
echo "drift exit: $?"
```

Expected: `drift exit: 0`. Cluster traffic must not produce spec drift.

- [ ] **Step 6: No commit (verification only)**

If everything above passes, the migration is verified end-to-end. No further commits.

---

## Plan self-review

**Spec coverage:**

| Spec section / non-`ratings` migration                     | Plan task |
|------------------------------------------------------------|-----------|
| ratings AsyncAPI (descriptor, exposed.go, generated specs) | 1–3       |
| notification HTTP migration                                | 4–5       |
| dlqueue HTTP migration (with stdhttp alias)                | 6–7       |
| ingestion HTTP + AsyncAPI                                  | 8–10      |
| details HTTP + AsyncAPI                                    | 11–13     |
| reviews HTTP + AsyncAPI (multi-descriptor)                 | 14–16     |
| productpage exception                                      | 17        |
| Bulk regenerate + drift / lint / breaking gates            | 18        |
| End-to-end k3d verification                                | 19        |

All migration scopes from the user-supplied table are covered.

**Type / signature consistency check:**

- `Producer.Publish(ctx, d events.Descriptor, payload any, recordKey, idempotencyKey string) error` — uniform across ratings, ingestion, details, reviews. The five-argument signature is deliberate: the partition key (per-event natural key, e.g. ISBN, ProductID) and the CE subject (always the idempotency key) are conceptually distinct and must both be plumbed through.
- `Exposed []events.Descriptor` — same variable name, same package level, in every kafka producer package.
- `Endpoints []api.Endpoint` and `const APIVersion = "1.0.0"` — same in every http adapter package.
- `descriptorFor(name string) events.Descriptor` — only introduced for reviews (multi-descriptor producer); ratings/ingestion/details continue to use `Exposed[0]` with a comment marking it as safe-only-while-single-descriptor.
- The runner's `DiscoverServices` already filters productpage out (it has neither slice file), so no runner change is needed.

**Placeholder scan:** No `TBD`, `TODO`, "fill in details", or "mirror Phase N". Every step shows the exact code to write or modify; every command is exact and includes the expected output. The `BookEvent` rename in ingestion explicitly preserves the `ingestion-isbn-<ISBN>` idempotency-key derivation from the original code so CE records remain byte-identical for downstream consumers — that's the kind of detail the foundation plan flagged as drift-risk and that this plan honors.

**Survey-derived corrections to the user prompt:**

- The user prompt suggested `ce-type` (dash form) for binary headers. The actual code uses `ce_type` (underscore form). This plan keeps the underscore form unchanged everywhere — changing it would be a wire-level breaking change for downstream consumers.
- The user prompt's per-service table called out details having "presumably" a producer publishing to `bookinfo_details_events`. Confirmed by survey: yes, in `services/details/internal/adapter/outbound/kafka/producer.go`, ce-type `com.bookinfo.details.book-added`, payload struct `bookAddedBody` (renamed to `BookAddedPayload`).
- The user prompt described reviews' delete route as `POST /v1/reviews/delete`. Confirmed: that's the exact path. The hand-written CQRS payload override (`body.review_id` → `review_id`) lives in `values-local.yaml` and stays there as a triggers-payload concern — the generator only touches `method`/`endpoint`.
- The user prompt suggested ingestion's ExposureKey "usually matches the existing chart key". Confirmed: ingestion's existing chart key is `raw-books-details`, so `ExposureKey: "raw-books-details"` matches. ratings and details use `events`. reviews uses `events` (shared by both descriptors). dlqueue and notification have no `events.exposed` block at all.
- The user prompt noted dlqueue's `stdhttp` alias must be preserved. Confirmed by survey: line 8 of `services/dlqueue/internal/adapter/inbound/http/handler.go` does `stdhttp "net/http"`. The refactor in Task 6 keeps the alias and uses `stdhttp.HandlerFunc` / `*stdhttp.ServeMux` types, which are interoperable with `pkg/api`'s generic `Register` because they are the same underlying stdlib types.
