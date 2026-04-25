# End-to-End Tracing Across Kafka Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Producer (Go) injects W3C `traceparent` into Kafka record headers; argo-events fork's Kafka source extracts those headers into CloudEvent extensions so the existing `tracing.SpanFromCloudEvent` chain joins the upstream trace. Result: one connected Tempo trace from gateway through to notification.

**Architecture:** Two-repo change. (1) `event-driven-bookinfo`: shared `pkg/telemetry/kafka.go` helper called once per producer. (2) `argo-events` fork: new `WithKafkaHeaders` Option (mirrors existing `WithHTTPHeaders`) plus a one-line call from each kafka source dispatch site. Cherry-pick the fork commit from `feat/cloudevents-compliance-otel-tracing` (PR 3961) onto the consumed branch `feat/combined-prs-3961-3983` so the image rebuild picks it up.

**Tech Stack:** Go 1.22, `github.com/twmb/franz-go`, OpenTelemetry SDK (W3C TraceContext propagator), Argo Events 1.9+, `github.com/cloudevents/sdk-go/v2`.

**Spec reference:** `docs/superpowers/specs/2026-04-25-e2e-tracing-kafka-design.md`

---

## File Structure

### Repo 1 — `event-driven-bookinfo` (this repo)

**Created:**

```
pkg/telemetry/kafka.go               # kafkaHeaderCarrier + InjectTraceContext
pkg/telemetry/kafka_test.go          # unit test for the helper
```

**Modified (one line each):**

```
services/details/internal/adapter/outbound/kafka/producer.go
services/details/internal/adapter/outbound/kafka/producer_test.go
services/reviews/internal/adapter/outbound/kafka/producer.go
services/reviews/internal/adapter/outbound/kafka/producer_test.go
services/ratings/internal/adapter/outbound/kafka/producer.go
services/ratings/internal/adapter/outbound/kafka/producer_test.go
services/ingestion/internal/adapter/outbound/kafka/producer.go
services/ingestion/internal/adapter/outbound/kafka/producer_test.go
```

### Repo 2 — `argo-events` fork (`/Users/kaio.fellipe/Documents/git/others/argo-events`)

**Modified on `feat/cloudevents-compliance-otel-tracing`:**

```
pkg/eventsources/common/common.go            # add WithKafkaHeaders Option
pkg/eventsources/common/common_test.go       # extend existing test file
pkg/eventsources/sources/kafka/start.go      # call dispatch with WithKafkaHeaders
```

Then cherry-picked verbatim onto `feat/combined-prs-3961-3983`.

---

## Phase 1 — Producer side (event-driven-bookinfo)

### Task 1: Shared Kafka trace-injection helper

**Files:**

- Create: `pkg/telemetry/kafka.go`
- Create: `pkg/telemetry/kafka_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/telemetry/kafka_test.go`:

```go
package telemetry_test

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestInjectTraceContext_WithActiveSpan_AddsTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	rec := &kgo.Record{Topic: "t"}
	telemetry.InjectTraceContext(ctx, rec)

	headers := map[string]string{}
	for _, h := range rec.Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}

	want := span.SpanContext().TraceID().String()
	got := headers["traceparent"]
	if len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent trace_id = %q, want trace_id %q embedded", got, want)
	}
}

func TestInjectTraceContext_NoActiveSpan_NoTraceparent(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	rec := &kgo.Record{Topic: "t"}
	telemetry.InjectTraceContext(context.Background(), rec)

	for _, h := range rec.Headers {
		if h.Key == "traceparent" {
			t.Fatalf("expected no traceparent header, got %q", string(h.Value))
		}
	}
}

func TestInjectTraceContext_PreservesExistingHeaders(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctx, span := otel.Tracer("test").Start(context.Background(), "p")
	defer span.End()

	rec := &kgo.Record{
		Topic: "t",
		Headers: []kgo.RecordHeader{
			{Key: "ce_type", Value: []byte("com.example.x")},
		},
	}
	telemetry.InjectTraceContext(ctx, rec)

	var hasCE, hasTP bool
	for _, h := range rec.Headers {
		if h.Key == "ce_type" {
			hasCE = true
		}
		if h.Key == "traceparent" {
			hasTP = true
		}
	}
	if !hasCE {
		t.Error("ce_type header was lost")
	}
	if !hasTP {
		t.Error("traceparent was not added")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/telemetry/... -v`

Expected: compile failure `undefined: telemetry.InjectTraceContext` (since the source file doesn't exist yet).

- [ ] **Step 3: Implement the helper**

Create `pkg/telemetry/kafka.go`:

```go
package telemetry

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// kafkaHeaderCarrier adapts a *kgo.Record's Headers slice to TextMapCarrier
// for W3C trace context propagation.
type kafkaHeaderCarrier struct {
	record *kgo.Record
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range c.record.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	for i := range c.record.Headers {
		if c.record.Headers[i].Key == key {
			c.record.Headers[i].Value = []byte(value)
			return
		}
	}
	c.record.Headers = append(c.record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.record.Headers))
	for _, h := range c.record.Headers {
		keys = append(keys, h.Key)
	}
	return keys
}

// InjectTraceContext writes W3C traceparent (and tracestate, when set) from
// ctx into the Kafka record headers via the registered TextMapPropagator.
// No-op when ctx carries no active span.
func InjectTraceContext(ctx context.Context, record *kgo.Record) {
	otel.GetTextMapPropagator().Inject(ctx, &kafkaHeaderCarrier{record: record})
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/telemetry/... -race -count=1 -v`

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry/kafka.go pkg/telemetry/kafka_test.go
git commit -m "feat(pkg/telemetry): add Kafka record traceparent injector

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Wire injector into details producer

**Files:**

- Modify: `services/details/internal/adapter/outbound/kafka/producer.go`
- Modify: `services/details/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the failing test**

Open `services/details/internal/adapter/outbound/kafka/producer_test.go`. Add at the bottom (preserve existing tests):

```go
func TestPublishBookAdded_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_details_events")

	if err := p.PublishBookAdded(ctx, domain.BookAddedEvent{
		Title: "T", Author: "A", Year: 2024, Type: "paperback",
		IdempotencyKey: "k1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	headers := map[string]string{}
	for _, h := range fc.records[0].Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}
	want := span.SpanContext().TraceID().String()
	if got := headers["traceparent"]; len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent = %q, want embedded trace_id %q", got, want)
	}
}
```

Add the imports needed at the top of the test file (alongside existing imports):

```go
import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./services/details/internal/adapter/outbound/kafka/... -run TestPublishBookAdded_InjectsTraceparent -v`

Expected: FAIL with `expected traceparent header, got none` (producer doesn't inject yet).

- [ ] **Step 3: Add the inject call to the producer**

Open `services/details/internal/adapter/outbound/kafka/producer.go`. Add the import (alongside existing imports):

```go
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
```

Locate the section in `PublishBookAdded` that builds `record` then calls `p.client.ProduceSync`. Insert one line between record construction and the `ProduceSync` call:

```go
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

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./services/details/internal/adapter/outbound/kafka/... -race -count=1 -v`

Expected: all tests PASS, including `TestPublishBookAdded_InjectsTraceparent`.

- [ ] **Step 5: Commit**

```bash
git add services/details/internal/adapter/outbound/kafka/
git commit -m "feat(details): inject W3C traceparent into Kafka record headers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Wire injector into reviews producer

**Files:**

- Modify: `services/reviews/internal/adapter/outbound/kafka/producer.go`
- Modify: `services/reviews/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the failing test**

Open `services/reviews/internal/adapter/outbound/kafka/producer_test.go`. Add at the bottom:

```go
func TestPublishReviewSubmitted_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_reviews_events")

	if err := p.PublishReviewSubmitted(ctx, domain.ReviewSubmittedEvent{
		ID: "r1", ProductID: "p", Reviewer: "u", Text: "ok", IdempotencyKey: "k1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	headers := map[string]string{}
	for _, h := range fc.records[0].Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}
	want := span.SpanContext().TraceID().String()
	if got := headers["traceparent"]; len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent = %q, want embedded trace_id %q", got, want)
	}
}
```

Add imports if not already present:

```go
import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./services/reviews/internal/adapter/outbound/kafka/... -run TestPublishReviewSubmitted_InjectsTraceparent -v`

Expected: FAIL with `expected traceparent header, got none`.

- [ ] **Step 3: Add the inject call to the reviews producer**

Open `services/reviews/internal/adapter/outbound/kafka/producer.go`. Add import:

```go
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
```

In the private `produce(ctx, ceType, key, partitionHint, body)` helper, insert one line between record construction and the `ProduceSync` call:

```go
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

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./services/reviews/internal/adapter/outbound/kafka/... -race -count=1 -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/outbound/kafka/
git commit -m "feat(reviews): inject W3C traceparent into Kafka record headers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Wire injector into ratings producer

**Files:**

- Modify: `services/ratings/internal/adapter/outbound/kafka/producer.go`
- Modify: `services/ratings/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the failing test**

Open `services/ratings/internal/adapter/outbound/kafka/producer_test.go`. Add at the bottom:

```go
func TestPublishRatingSubmitted_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "bookinfo_ratings_events")

	if err := p.PublishRatingSubmitted(ctx, domain.RatingSubmittedEvent{
		ID: "rt1", ProductID: "p", Reviewer: "u", Stars: 5, IdempotencyKey: "k1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	headers := map[string]string{}
	for _, h := range fc.records[0].Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}
	want := span.SpanContext().TraceID().String()
	if got := headers["traceparent"]; len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent = %q, want embedded trace_id %q", got, want)
	}
}
```

Add imports if not already present:

```go
import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./services/ratings/internal/adapter/outbound/kafka/... -run TestPublishRatingSubmitted_InjectsTraceparent -v`

Expected: FAIL.

- [ ] **Step 3: Add the inject call to the ratings producer**

Open `services/ratings/internal/adapter/outbound/kafka/producer.go`. Add import:

```go
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
```

In `PublishRatingSubmitted`, insert one line between record construction and the `ProduceSync` call:

```go
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

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./services/ratings/internal/adapter/outbound/kafka/... -race -count=1 -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ratings/internal/adapter/outbound/kafka/
git commit -m "feat(ratings): inject W3C traceparent into Kafka record headers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Wire injector into ingestion producer

**Files:**

- Modify: `services/ingestion/internal/adapter/outbound/kafka/producer.go`
- Modify: `services/ingestion/internal/adapter/outbound/kafka/producer_test.go`

- [ ] **Step 1: Write the failing test**

Open `services/ingestion/internal/adapter/outbound/kafka/producer_test.go`. Add at the bottom:

```go
func TestPublishBookAdded_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})
	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	fc := &fakeClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "raw_books_details")

	book := domain.Book{
		Title:       "T",
		Authors:     []string{"A"},
		ISBN:        "9780000000099",
		PublishYear: 2024,
	}
	if err := p.PublishBookAdded(ctx, book); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	headers := map[string]string{}
	for _, h := range fc.records[0].Headers {
		headers[h.Key] = string(h.Value)
	}
	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header, got none")
	}
	want := span.SpanContext().TraceID().String()
	if got := headers["traceparent"]; len(got) < 35 || got[3:35] != want {
		t.Errorf("traceparent = %q, want embedded trace_id %q", got, want)
	}
}
```

Add imports if not already present:

```go
import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `go test ./services/ingestion/internal/adapter/outbound/kafka/... -run TestPublishBookAdded_InjectsTraceparent -v`

Expected: FAIL with `expected traceparent header, got none`.

- [ ] **Step 3: Add the inject call to the ingestion producer**

Open `services/ingestion/internal/adapter/outbound/kafka/producer.go`. Add import:

```go
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
```

In `PublishBookAdded`, insert one line between record construction and the `ProduceSync` call:

```go
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

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./services/ingestion/internal/adapter/outbound/kafka/... -race -count=1 -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/kafka/
git commit -m "feat(ingestion): inject W3C traceparent into Kafka record headers

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Producer-side full lint + test sweep

- [ ] **Step 1: Lint**

Run: `make lint`

Expected: `0 issues.`

- [ ] **Step 2: All tests**

Run: `make test`

Expected: every package OK; no FAIL line.

- [ ] **Step 3: No additional commit needed if everything passes**

If lint reports formatting issues, run `gofmt -w` on the affected files and commit:

```bash
git add -A
git commit -m "chore: gofmt sweep after traceparent injection

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2 — Argo Events fork (`feat/cloudevents-compliance-otel-tracing`)

**Constraints (apply to every commit in this phase):**

1. Preserve the 12 existing PR-3961 commits (`7e5371a8` … `30401397`) unchanged. Append-only.
2. Sign off every commit: `git commit -s`. DCO required by argoproj/argo-events upstream.
3. **NO** `Co-Authored-By: Claude` trailer on these commits.
4. Push as fast-forward only — no `--force` / `--force-with-lease`.

### Task 7: Verify branch state and switch worktree

- [ ] **Step 1: Switch to the fork repo and confirm branch**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git status
git branch --show-current
```

Expected: clean working tree. If current branch is not `feat/cloudevents-compliance-otel-tracing`, run:

```bash
git checkout feat/cloudevents-compliance-otel-tracing
```

- [ ] **Step 2: Snapshot the existing 12 commits to verify untouched later**

```bash
git log --oneline master..feat/cloudevents-compliance-otel-tracing > /tmp/argo-events-pr3961-baseline.txt
wc -l /tmp/argo-events-pr3961-baseline.txt
```

Expected: file lists 12 SHAs (top-most `30401397`, oldest `7e5371a8`). Will compare after pushing.

- [ ] **Step 3: Pull from origin (defensive)**

```bash
git pull --ff-only origin feat/cloudevents-compliance-otel-tracing
```

Expected: `Already up to date.` or fast-forward without merge.

---

### Task 8: Add `WithKafkaHeaders` Option to `eventsourcecommon`

**Files:**

- Modify: `pkg/eventsources/common/common.go`
- Modify: `pkg/eventsources/common/common_test.go`

- [ ] **Step 1: Write the failing test**

Open `pkg/eventsources/common/common_test.go`. Add the following test (preserve existing tests; add to the same package):

```go
func TestWithKafkaHeaders(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		wantTP      string
		wantTS      string
	}{
		{
			name:    "traceparent and tracestate present",
			headers: map[string]string{
				"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
				"tracestate":  "rojo=00f067aa0ba902b7",
			},
			wantTP: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantTS: "rojo=00f067aa0ba902b7",
		},
		{
			name:    "only traceparent",
			headers: map[string]string{
				"traceparent": "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
			},
			wantTP: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
			wantTS: "",
		},
		{
			name:    "no trace headers — no extension set",
			headers: map[string]string{"x-other": "v"},
			wantTP:  "",
			wantTS:  "",
		},
		{
			name:    "nil headers — safe no-op",
			headers: nil,
			wantTP:  "",
			wantTS:  "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			e := cloudevents.NewEvent()
			if err := common.WithKafkaHeaders(tt.headers)(&e); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, _ := e.Extensions()["traceparent"].(string); got != tt.wantTP {
				t.Errorf("traceparent extension = %q, want %q", got, tt.wantTP)
			}
			if got, _ := e.Extensions()["tracestate"].(string); got != tt.wantTS {
				t.Errorf("tracestate extension = %q, want %q", got, tt.wantTS)
			}
		})
	}
}
```

If `cloudevents` is not yet imported in this test file, add to imports:

```go
import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
)
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./pkg/eventsources/common/ -run TestWithKafkaHeaders -v
```

Expected: compile failure `undefined: common.WithKafkaHeaders`.

- [ ] **Step 3: Implement `WithKafkaHeaders`**

Open `pkg/eventsources/common/common.go`. Add this function at the end of the file (after `WithHTTPHeaders`, mirroring its shape):

```go
// WithKafkaHeaders extracts W3C trace context headers (traceparent/tracestate)
// from a Kafka record's headers map and sets them as CloudEvent extensions.
// This enables trace propagation from Kafka producers (e.g. franz-go via
// otel.GetTextMapPropagator().Inject) into the eventbus dispatch chain
// where SpanFromCloudEvent will pick them up as the parent span.
func WithKafkaHeaders(headers map[string]string) Option {
	return func(e *event.Event) error {
		if tp, ok := headers["traceparent"]; ok && tp != "" {
			e.SetExtension("traceparent", tp)
		}
		if ts, ok := headers["tracestate"]; ok && ts != "" {
			e.SetExtension("tracestate", ts)
		}
		return nil
	}
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./pkg/eventsources/common/ -run TestWithKafkaHeaders -race -count=1 -v
```

Expected: all 4 sub-tests PASS.

- [ ] **Step 5: Run full common package test suite**

```bash
go test ./pkg/eventsources/common/ -race -count=1
```

Expected: all PASS.

- [ ] **Step 6: Commit (signed off, no Claude trailer)**

```bash
git add pkg/eventsources/common/common.go pkg/eventsources/common/common_test.go
git commit -s -m "feat(eventsource): add WithKafkaHeaders option to propagate W3C trace context

Mirrors the existing WithHTTPHeaders option. When a Kafka record carries
W3C trace context (traceparent/tracestate) in its headers, this option
copies them into the CloudEvent extensions so the existing
tracing.SpanFromCloudEvent extractor in eventing.go can pick them up as
the parent span for the eventsource.publish PRODUCER span. Closes the
producer-to-eventsource trace gap when used by the kafka source's
dispatch call."
```

Verify the trailer is exactly `Signed-off-by: ...`:

```bash
git log -1 --format=%B | tail -3
```

Expected: ends with `Signed-off-by: <Name> <email>` and no `Co-Authored-By` line.

---

### Task 9: Use `WithKafkaHeaders` in the Kafka source dispatch

**Files:**

- Modify: `pkg/eventsources/sources/kafka/start.go`

- [ ] **Step 1: Locate the two dispatch sites**

```bash
grep -n "dispatch(eventBody" pkg/eventsources/sources/kafka/start.go
```

Expected output (paths and line numbers may vary slightly — current head shows two matches):

```
291:		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID)); err != nil {
482:		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID)); err != nil {
```

These are the consumerGroupConsumer path (~line 291) and partitionConsumer path (~line 482).

- [ ] **Step 2: Update the consumerGroupConsumer dispatch site**

Open `pkg/eventsources/sources/kafka/start.go`. Find the block around line 291. The lines just before look like:

```go
headers := make(map[string]string)
for _, recordHeader := range msg.Headers {
	headers[string(recordHeader.Key)] = string(recordHeader.Value)
}
eventData.Headers = headers

// ... (json marshaling) ...

if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID)); err != nil {
	return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
}
```

Modify the `dispatch` call to also pass `WithKafkaHeaders(headers)`:

```go
if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
	return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
}
```

- [ ] **Step 3: Update the partitionConsumer dispatch site**

Find the block around line 482. The same `headers` map is built locally (different variable scope but same name). Apply the identical change:

```go
if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
	return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
}
```

- [ ] **Step 4: Build and run package tests**

```bash
go build ./pkg/eventsources/sources/kafka/...
go test ./pkg/eventsources/sources/kafka/... -race -count=1
```

Expected: build OK, tests PASS (existing tests unaffected).

- [ ] **Step 5: Run lint and codegen check**

If the project has Make targets for lint and codegen, run them:

```bash
make lint || true
make codegen || true
```

If unavailable, run gofmt:

```bash
gofmt -w pkg/eventsources/sources/kafka/start.go pkg/eventsources/common/common.go
```

If `make codegen` produces changes, include them in the same commit (matches the existing pattern where codegen-only commits sit alongside their feature changes — see `b039f558 chore: regenerate codegen after combining PRs #3961 + #3983`).

- [ ] **Step 6: Verify the 12 baseline commits are still present and unchanged**

```bash
diff <(git log --oneline master..feat/cloudevents-compliance-otel-tracing | tail -12) <(tail -12 /tmp/argo-events-pr3961-baseline.txt)
```

Expected: no output (12 baseline SHAs identical).

- [ ] **Step 7: Commit (signed off, no Claude trailer)**

```bash
git add pkg/eventsources/sources/kafka/start.go
git commit -s -m "feat(eventsource/kafka): propagate W3C trace context from record headers

Pass WithKafkaHeaders alongside WithID at both dispatch sites
(consumerGroupConsumer and partitionConsumer) so traceparent/tracestate
present on the Kafka record become CloudEvent extensions. The existing
SpanFromCloudEvent in eventing.go then chains the eventsource.publish
PRODUCER span under the upstream producer span, closing the trace gap
between Kafka producers and the eventbus."
```

Verify trailer:

```bash
git log -1 --format=%B
```

Expected: ends with `Signed-off-by: ...`, no `Co-Authored-By` line.

- [ ] **Step 8: Push to origin (fast-forward only)**

```bash
git push origin feat/cloudevents-compliance-otel-tracing
```

If the push is rejected with "non-fast-forward", STOP — do not force. Pull with rebase instead:

```bash
git pull --rebase origin feat/cloudevents-compliance-otel-tracing
```

Then re-run the previous push.

Expected: GitHub PR 3961 picks up the two new commits (Task 8 + Task 9 outputs).

---

### Task 10: Cherry-pick onto the consumed branch and trigger image build

- [ ] **Step 1: Capture the two new commit SHAs**

```bash
git log --oneline -2 feat/cloudevents-compliance-otel-tracing
```

Expected: top two lines are the new `feat(eventsource/kafka)` commit and the `feat(eventsource): add WithKafkaHeaders option` commit. Note both SHAs.

- [ ] **Step 2: Switch to the consumed branch**

```bash
git checkout feat/combined-prs-3961-3983
git pull --ff-only origin feat/combined-prs-3961-3983
```

Expected: branch is up to date with origin.

- [ ] **Step 3: Cherry-pick the two new commits in order**

Replace `<SHA_OPTION>` and `<SHA_KAFKA>` with the actual SHAs captured above (option commit first, then kafka source commit):

```bash
git cherry-pick -s <SHA_OPTION>
git cherry-pick -s <SHA_KAFKA>
```

The `-s` flag re-signs the cherry-pick (DCO trailer is preserved on the original commits but cherry-pick adds a new author signature line; `-s` keeps it consistent on the consumed branch too).

If a conflict appears, STOP and resolve manually before continuing. The consumed branch's only divergence from `feat/cloudevents-compliance-otel-tracing` is the kafka-latency commits and a codegen merge commit, none of which touch `pkg/eventsources/common/common.go` or `pkg/eventsources/sources/kafka/start.go` — so a clean cherry-pick is expected.

- [ ] **Step 4: Re-run codegen if applicable**

```bash
make codegen || true
```

If codegen produced changes, commit them (preserve the existing project pattern):

```bash
git add -A
git commit -s -m "chore: regenerate codegen after kafka trace propagation cherry-pick"
```

- [ ] **Step 5: Build and test**

```bash
go build ./...
go test ./pkg/eventsources/common/ ./pkg/eventsources/sources/kafka/ -race -count=1
```

Expected: build OK, tests PASS.

- [ ] **Step 6: Push the consumed branch**

```bash
git push origin feat/combined-prs-3961-3983
```

Expected: fast-forward push. Pushing this branch triggers the GitHub Actions workflow that builds and pushes `ghcr.io/kaio6fellipe/argo-events:<tag>`.

- [ ] **Step 7: Confirm the image build completes**

In a browser, open <https://github.com/kaio6fellipe/argo-events/actions> and watch the workflow triggered by this push. Expected: image build green.

Or, from the CLI (if `gh` is authenticated against the fork repo):

```bash
gh -R kaio6fellipe/argo-events run list --branch feat/combined-prs-3961-3983 --limit 1
```

Expected: latest run `completed` / `success`.

---

## Phase 3 — End-to-end validation in `event-driven-bookinfo`

### Task 11: Pull the new image into the running cluster

- [ ] **Step 1: Switch back to the bookinfo repo**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git status
```

Expected: on `feat/event-driven-notifications` (or whatever branch the producer-side commits landed on), clean working tree.

- [ ] **Step 2: Identify the argo-events image reference**

```bash
grep -rn "ghcr.io/kaio6fellipe/argo-events" deploy/ charts/ 2>&1 | head
```

Note the image reference (likely a `:latest` or pinned tag). If pinned to a specific SHA/tag that hasn't been updated, update it to the tag the new build produced (or to the moving `latest` tag).

- [ ] **Step 3: Rolling restart the argo-events controllers and existing EventSource pods**

```bash
kubectl --context=k3d-bookinfo-local -n platform rollout restart deploy
kubectl --context=k3d-bookinfo-local -n bookinfo delete pod -l eventsource-name
kubectl --context=k3d-bookinfo-local -n bookinfo delete pod -l sensor-name
```

Wait for them to come back:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get pod -l eventsource-name -w
```

Press Ctrl+C once all show `Running 1/1`.

- [ ] **Step 4: Rebuild and redeploy the bookinfo Go services**

```bash
make k8s-rebuild
```

Expected: images rebuilt, imported, deployments rolled.

- [ ] **Step 5: Confirm all pods Running**

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get pods --no-headers | awk '{print $3}' | sort | uniq -c
```

Expected: only `Running` (no Crash/Pending/Container).

---

### Task 12: Smoke test — single connected trace for a review submission

- [ ] **Step 1: POST a review with a unique marker**

```bash
curl -sS -X POST http://localhost:8080/v1/reviews \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"trace-e2e","reviewer":"u","text":"trace audit"}'
echo
sleep 5
```

- [ ] **Step 2: Capture the producing service's trace_id**

```bash
TRACE_REVIEWS=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write --since=30s 2>&1 \
  | grep '"product_id":"trace-e2e"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "reviews-write trace_id: $TRACE_REVIEWS"
```

Expected: a 32-char hex string.

- [ ] **Step 3: Capture the notification service's trace_id**

```bash
TRACE_NOTIF=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/notification --since=30s 2>&1 \
  | grep '"subject":"trace-e2e"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "notification trace_id: $TRACE_NOTIF"
```

Expected: a 32-char hex string.

- [ ] **Step 4: Acceptance — IDs must match**

```bash
[ "$TRACE_REVIEWS" = "$TRACE_NOTIF" ] && echo "MATCH" || echo "MISMATCH: $TRACE_REVIEWS vs $TRACE_NOTIF"
```

Expected: `MATCH`. If `MISMATCH`, the producer→eventsource fix did not take effect. Check that the new image is actually running (`kubectl describe pod -l eventsource-name=reviews-events | grep Image`) and that the producer commit is actually deployed.

---

### Task 13: Tempo trace tree inspection

- [ ] **Step 1: Open Grafana**

In a browser: <http://localhost:3000>. Login `admin / admin` (project default; password may differ in newer deploys — read from secret if needed: `kubectl --context=k3d-bookinfo-local -n observability get secret prometheus-grafana -o jsonpath='{.data.admin-password}' | base64 -d`).

- [ ] **Step 2: Query Tempo for the captured trace**

In Grafana → Explore → select `Tempo` data source → query field, paste:

```
{ traceID="<TRACE_REVIEWS from Task 12 Step 2>" }
```

Submit. Click the resulting trace.

- [ ] **Step 3: Verify the span tree contains every hop**

Expected spans visible in the timeline (order from root to leaf):

1. `envoy-gateway` (server kind, root)
2. `reviews-write-api POST /v1/reviews` (server kind, child of CQRS sensor's HTTP CLIENT)
3. `eventsource.publish` (PRODUCER kind) — emitted from the dispatch in eventing.go on the reviews-events EventSource side
4. eventbus consumer span (CONSUMER kind) — emitted by the notification consumer sensor
5. `notification-consumer-sensor` trigger span (CLIENT kind)
6. `notification-api POST /v1/notifications` (server kind)

Span tree depth >= 6. All under the same root trace_id.

If the eventsource.publish span is missing or appears as a separate trace root, the `WithKafkaHeaders` option is not being applied — re-check Task 9 changes in the running image.

- [ ] **Step 4: Repeat for the other three event types**

Run the same smoke + trace check for:

```bash
# book-added
curl -sS -X POST http://localhost:8080/v1/details -H 'Content-Type: application/json' \
  -d '{"title":"trace-book","author":"a","year":2026,"type":"paperback","pages":1,"isbn_13":"9780000000111"}'

# rating-submitted
curl -sS -X POST http://localhost:8080/v1/ratings -H 'Content-Type: application/json' \
  -d '{"product_id":"trace-rating","reviewer":"u","stars":4}'

# review-deleted (uses the review created in Task 12)
REVIEW_ID=$(kubectl --context=k3d-bookinfo-local -n bookinfo exec statefulset/reviews-postgresql -- \
  bash -c "PGPASSWORD=bookinfo psql -U bookinfo -d bookinfo_reviews -t -c \"SELECT id FROM reviews WHERE product_id='trace-e2e' LIMIT 1;\"" | xargs)
curl -sS -X POST http://localhost:8080/v1/reviews/delete -H 'Content-Type: application/json' \
  -d "{\"review_id\":\"$REVIEW_ID\"}"
```

For each, repeat Task 12 Steps 2-4. Acceptance: `trace_id` matches between producer service and notification service.

---

### Task 14: Final lint sweep + push and PR update

- [ ] **Step 1: Producer-side lint and tests**

```bash
make lint
make test
```

Expected: all clean.

- [ ] **Step 2: Push the bookinfo branch**

```bash
git push
```

Expected: PR (the existing `feat/event-driven-notifications` PR or a new branch — match the working branch) updated.

- [ ] **Step 3: Verify CI passes**

```bash
gh pr checks --watch --interval 30 --fail-fast
```

Expected: all checks green.

---

## Self-Review

**Spec coverage:**

- Decision 1 (two-repo change) → Phase 1 + Phase 2 ✔
- Decision 2 (cherry-pick) → Task 10 ✔
- Decision 3 (mirror webhook pattern) → Task 8 mirrors `WithHTTPHeaders` exactly ✔
- Decision 4 (all 4 producers) → Tasks 2-5 cover details, reviews, ratings, ingestion ✔
- Decision 5 (DCO + no Claude trailer on fork) → Phase 2 constraints + Tasks 8 Step 6, 9 Step 7 ✔
- Producer-side component (`pkg/telemetry/kafka.go`) → Task 1 ✔
- Per-service producer change (one-line addition) → Tasks 2, 3, 4, 5 ✔
- Argo-events fork component (`WithKafkaHeaders` option + Kafka dispatch wiring) → Tasks 8 + 9 ✔
- Image release flow → Task 10 ✔
- Constraints on fork branch (preserve 12 commits, no force-push, sign-off, no Claude trailer) → Phase 2 header + Task 7 Step 2 baseline + Task 9 Step 6 verification ✔
- Data flow target state → Tasks 12, 13 verify ✔
- Error handling cases (no active span, malformed traceparent, panic, producer fails) → covered by `propagation.Inject`'s no-op semantics + existing argo-events recovery; tested in Task 1 (no-active-span case) ✔
- Producer unit test for traceparent presence → Tasks 1, 2, 3, 4, 5 ✔
- Fork unit test → Task 8 ✔
- E2E validation → Tasks 12, 13 ✔
- All 7 acceptance criteria → covered across Tasks 1-13 ✔

**Placeholder scan:** no "TBD", "TODO", "implement later", "similar to". The two cherry-pick SHAs in Task 10 Step 3 are correctly marked as placeholders that the executor reads from Task 10 Step 1 (concrete commands). The image-tag reference in Task 11 Step 2 reflects an actual file lookup, not an unfilled value.

**Type consistency:** `InjectTraceContext(ctx context.Context, record *kgo.Record)` is the same signature in Task 1 (definition), Task 2-5 (call sites), and Task 12 (assertions). `WithKafkaHeaders(headers map[string]string) Option` matches in Task 8 (definition) and Task 9 (call sites). `kafkaHeaderCarrier` is internal to `pkg/telemetry` and not referenced elsewhere — no inconsistency.
