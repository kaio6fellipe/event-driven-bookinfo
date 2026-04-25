# Kafka Producer Span Instrumentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap each Kafka `ProduceSync` call in a child OTel span (SpanKindProducer, OTel messaging semconv attributes) so Tempo shows the publish operation as a peer of the DB spans under the API server span.

**Architecture:** Single helper `telemetry.StartProducerSpan(ctx, topic, key)` added to existing `pkg/telemetry/kafka.go`. Each producer (details, reviews, ratings, ingestion) wraps its `ProduceSync` call in a 4-line block: start span (defer end) → InjectTraceContext (already there) → produce → record error on failure. No new module deps; raw OTel attribute strings.

**Tech Stack:** Go 1.22, `go.opentelemetry.io/otel` (already pulled at v1.43.0), `github.com/twmb/franz-go/pkg/kgo`.

**Spec reference:** `docs/superpowers/specs/2026-04-25-kafka-producer-span-design.md`

---

## File Structure

Existing files modified — no new files.

```text
pkg/telemetry/kafka.go              # +helper StartProducerSpan
pkg/telemetry/kafka_test.go         # +unit test for helper

services/details/internal/adapter/outbound/kafka/producer.go        # 4-line wrap in PublishBookAdded
services/reviews/internal/adapter/outbound/kafka/producer.go        # 4-line wrap in produce helper
services/ratings/internal/adapter/outbound/kafka/producer.go        # 4-line wrap in PublishRatingSubmitted
services/ingestion/internal/adapter/outbound/kafka/producer.go      # 4-line wrap in PublishBookAdded
```

Producer test files are NOT modified — the existing `Test...InjectsTraceparent` tests already verify traceparent presence and continue to pass with the wrap in place. Adding extra assertions about parent-span identity is out of scope (covered by the helper unit test plus end-to-end Tempo inspection).

---

## Task 1: Add `StartProducerSpan` helper to `pkg/telemetry/kafka.go`

**Files:**

- Modify: `pkg/telemetry/kafka.go`
- Modify: `pkg/telemetry/kafka_test.go`

- [ ] **Step 1: Write the failing test**

Open `pkg/telemetry/kafka_test.go`. Add to imports (alongside existing imports):

```go
"go.opentelemetry.io/otel/trace"
```

Append at the end of the file (preserve existing tests):

```go
func TestStartProducerSpan_AttachesSpanToContext(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	defer parent.End()

	ctx, span := telemetry.StartProducerSpan(parentCtx, "my_topic", "my_key")
	defer span.End()

	// The returned ctx must carry the new span (not the parent).
	got := trace.SpanFromContext(ctx)
	if got.SpanContext().SpanID() == parent.SpanContext().SpanID() {
		t.Fatalf("ctx still references parent span %q; expected new producer span",
			parent.SpanContext().SpanID().String())
	}
	if !got.SpanContext().IsValid() {
		t.Fatal("returned ctx has no valid span context")
	}
	// Trace ID is inherited from parent.
	if got.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("trace_id = %s, want inherited %s",
			got.SpanContext().TraceID(), parent.SpanContext().TraceID())
	}
}

func TestStartProducerSpan_NoParent_StillReturnsValidSpan(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	ctx, span := telemetry.StartProducerSpan(context.Background(), "my_topic", "my_key")
	defer span.End()

	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Fatal("expected valid span context even without a parent span")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./pkg/telemetry/... -run TestStartProducerSpan -v
```

Expected: compile error `undefined: telemetry.StartProducerSpan`.

- [ ] **Step 3: Implement the helper**

Open `pkg/telemetry/kafka.go`. Append to the imports block (preserve existing imports — `context`, `kgo`, `otel`, `propagation` are already there):

```go
"go.opentelemetry.io/otel/attribute"
"go.opentelemetry.io/otel/trace"
```

Append the helper at the END of the file (after `InjectTraceContext`):

```go
// StartProducerSpan starts a child span for a Kafka publish operation following
// OTel messaging semantic conventions. The caller must End() the span (use defer).
// Span name is "<topic> publish"; SpanKind is Producer.
func StartProducerSpan(ctx context.Context, topic, key string) (context.Context, trace.Span) {
	return otel.Tracer("kafka-producer").Start(ctx,
		topic+" publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", topic),
			attribute.String("messaging.operation.type", "publish"),
			attribute.String("messaging.kafka.message.key", key),
		),
	)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
go test ./pkg/telemetry/... -race -count=1 -v
```

Expected: all tests PASS, including the two new `TestStartProducerSpan_*` cases plus pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry/kafka.go pkg/telemetry/kafka_test.go
git commit -m "$(cat <<'EOF'
feat(pkg/telemetry): add StartProducerSpan helper for Kafka publish

Returns a context + trace.Span with SpanKindProducer and OTel messaging
semconv attributes (messaging.system=kafka, destination.name=<topic>,
operation.type=publish, kafka.message.key=<key>). Span name follows the
"<destination> <operation>" convention. Producers wrap their ProduceSync
call with this helper so Tempo shows the publish op as a child of the
API server span.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Wrap details producer

**Files:**

- Modify: `services/details/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Read the current PublishBookAdded function**

Run:

```bash
grep -n "PublishBookAdded\|ProduceSync\|telemetry.InjectTraceContext\|FirstErr" services/details/internal/adapter/outbound/kafka/producer.go
```

Note the line numbers for: function signature, `InjectTraceContext` call, `ProduceSync` call, and `FirstErr` error check.

- [ ] **Step 2: Add `codes` import if not present**

Open `services/details/internal/adapter/outbound/kafka/producer.go`. Check if `"go.opentelemetry.io/otel/codes"` is already in the imports block. If not, add it (alongside the existing OTel imports).

- [ ] **Step 3: Wrap the ProduceSync call**

Locate this existing block in `PublishBookAdded`:

```go
telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

Replace it with:

```go
ctx, span := telemetry.StartProducerSpan(ctx, p.topic, evt.IdempotencyKey)
defer span.End()

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

- [ ] **Step 4: Build and run tests**

Run:

```bash
go test ./services/details/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: all tests PASS, including pre-existing `TestPublishBookAdded`, `TestPublishBookAdded_ProduceError`, `TestPublishBookAdded_InjectsTraceparent`.

The error-path test (`TestPublishBookAdded_ProduceError`) exercises `span.RecordError` + `SetStatus`; it should still pass — the error is wrapped and returned exactly as before.

- [ ] **Step 5: Commit**

```bash
git add services/details/internal/adapter/outbound/kafka/producer.go
git commit -m "$(cat <<'EOF'
feat(details): wrap Kafka publish in producer span

Calls telemetry.StartProducerSpan before InjectTraceContext + ProduceSync
so Tempo shows the publish operation as a peer of the DB spans under
the API server span. Errors recorded on the span via RecordError +
SetStatus.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wrap reviews producer

**Files:**

- Modify: `services/reviews/internal/adapter/outbound/kafka/producer.go`

Reviews has TWO publish methods (`PublishReviewSubmitted`, `PublishReviewDeleted`) that delegate to a private `produce(ctx, ceType, key, partitionHint, body)` helper. Wrapping the helper covers both events.

- [ ] **Step 1: Read the current produce helper**

Run:

```bash
grep -n "func (p \*Producer) produce\|ProduceSync\|telemetry.InjectTraceContext\|FirstErr" services/reviews/internal/adapter/outbound/kafka/producer.go
```

Note the line numbers for: `produce` function signature, `InjectTraceContext` call, `ProduceSync` call.

- [ ] **Step 2: Add `codes` import if not present**

Open `services/reviews/internal/adapter/outbound/kafka/producer.go`. Add `"go.opentelemetry.io/otel/codes"` to the imports if not already there.

- [ ] **Step 3: Wrap the ProduceSync call inside `produce`**

Locate this existing block in `produce`:

```go
telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

Replace it with:

```go
ctx, span := telemetry.StartProducerSpan(ctx, p.topic, key)
defer span.End()

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

> NOTE: the local variable `key` already exists in `produce` (it's the function's third parameter — the idempotency key). Reuse it as the span's message key attribute. Both publish methods pass their `evt.IdempotencyKey` as this argument.

- [ ] **Step 4: Build and run tests**

Run:

```bash
go test ./services/reviews/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: all tests PASS, including pre-existing `TestPublishReviewSubmitted`, `TestPublishReviewDeleted`, `TestPublishReviewSubmitted_InjectsTraceparent`.

- [ ] **Step 5: Commit**

```bash
git add services/reviews/internal/adapter/outbound/kafka/producer.go
git commit -m "$(cat <<'EOF'
feat(reviews): wrap Kafka publish in producer span

Wraps the shared produce helper so both PublishReviewSubmitted and
PublishReviewDeleted emit a SpanKindProducer span under the API server
span. Errors recorded on the span via RecordError + SetStatus.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wrap ratings producer

**Files:**

- Modify: `services/ratings/internal/adapter/outbound/kafka/producer.go`

- [ ] **Step 1: Read the current PublishRatingSubmitted function**

Run:

```bash
grep -n "PublishRatingSubmitted\|ProduceSync\|telemetry.InjectTraceContext\|FirstErr" services/ratings/internal/adapter/outbound/kafka/producer.go
```

- [ ] **Step 2: Add `codes` import if not present**

Open `services/ratings/internal/adapter/outbound/kafka/producer.go`. Add `"go.opentelemetry.io/otel/codes"` to imports if not present.

- [ ] **Step 3: Wrap the ProduceSync call**

Locate this existing block in `PublishRatingSubmitted`:

```go
telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

Replace it with:

```go
ctx, span := telemetry.StartProducerSpan(ctx, p.topic, evt.IdempotencyKey)
defer span.End()

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

- [ ] **Step 4: Build and run tests**

Run:

```bash
go test ./services/ratings/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: all tests PASS, including `TestPublishRatingSubmitted`, `TestPublishRatingSubmitted_InjectsTraceparent`.

- [ ] **Step 5: Commit**

```bash
git add services/ratings/internal/adapter/outbound/kafka/producer.go
git commit -m "$(cat <<'EOF'
feat(ratings): wrap Kafka publish in producer span

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Wrap ingestion producer

**Files:**

- Modify: `services/ingestion/internal/adapter/outbound/kafka/producer.go`

Ingestion uses `book.ISBN` as the Kafka record key, not an idempotency key. Pass that as the span's message key attribute.

- [ ] **Step 1: Read the current PublishBookAdded function**

Run:

```bash
grep -n "PublishBookAdded\|ProduceSync\|telemetry.InjectTraceContext\|FirstErr" services/ingestion/internal/adapter/outbound/kafka/producer.go
```

- [ ] **Step 2: Add `codes` import if not present**

Open `services/ingestion/internal/adapter/outbound/kafka/producer.go`. Add `"go.opentelemetry.io/otel/codes"` to imports if not present.

- [ ] **Step 3: Wrap the ProduceSync call**

Locate this existing block in `PublishBookAdded`:

```go
telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

Replace it with:

```go
ctx, span := telemetry.StartProducerSpan(ctx, p.topic, book.ISBN)
defer span.End()

telemetry.InjectTraceContext(ctx, record)

results := p.client.ProduceSync(ctx, record)
if err := results.FirstErr(); err != nil {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return fmt.Errorf("producing to Kafka: %w", err)
}
```

> NOTE: `book.ISBN` is the function's input field used elsewhere in the same method as the Kafka record key (`Key: []byte(book.ISBN)`). Reuse it.

- [ ] **Step 4: Build and run tests**

Run:

```bash
go test ./services/ingestion/internal/adapter/outbound/kafka/... -race -count=1 -v
```

Expected: all tests PASS, including the pre-existing `TestPublishBookAdded` table tests, `TestPublishBookAdded_ProduceError`, and `TestPublishBookAdded_InjectsTraceparent`.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/internal/adapter/outbound/kafka/producer.go
git commit -m "$(cat <<'EOF'
feat(ingestion): wrap Kafka publish in producer span

Uses book.ISBN as messaging.kafka.message.key (matches the value also
used as the Kafka record key).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Full lint + test sweep

- [ ] **Step 1: Run lint**

Run:

```bash
make lint
```

Expected: `0 issues.`

- [ ] **Step 2: Run all tests**

Run:

```bash
make test
```

Expected: every package OK; no FAIL lines.

- [ ] **Step 3: If lint failed, fix and re-run**

If `make lint` reports anything (commonly: gofmt or unused imports if `codes` was added speculatively where it wasn't needed), fix and recommit:

```bash
gofmt -w pkg/telemetry/ services/details/internal/adapter/outbound/kafka/ services/reviews/internal/adapter/outbound/kafka/ services/ratings/internal/adapter/outbound/kafka/ services/ingestion/internal/adapter/outbound/kafka/
make lint
git add -A
git commit -m "chore: gofmt sweep after producer span wrap

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

If no lint issues, no commit needed.

---

## Task 7: Rebuild bookinfo services in cluster

- [ ] **Step 1: Confirm cluster up**

Run:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get pods --no-headers | awk '{print $3}' | sort | uniq -c
```

Expected: only `Running`. If pods are missing, the cluster may have been torn down — abort this task and re-run `make run-k8s` first.

- [ ] **Step 2: Rebuild and redeploy bookinfo Go services**

Run:

```bash
make k8s-rebuild
```

Expected: images rebuilt, imported into k3d, deployments rolled. Takes ~1-2 min.

- [ ] **Step 3: Wait for rollout**

Run:

```bash
sleep 15
kubectl --context=k3d-bookinfo-local -n bookinfo get pods --no-headers | awk '{print $3}' | sort | uniq -c
```

Expected: only `Running`.

---

## Task 8: Cluster smoke test — capture trace IDs for each event type

After this task completes, you will have 4 trace IDs to inspect manually in Grafana Tempo at <http://localhost:3000>.

- [ ] **Step 1: book-added**

Run:

```bash
curl -sS -X POST http://localhost:8080/v1/details \
  -H 'Content-Type: application/json' \
  -d '{"title":"span-test-book","author":"a","year":2026,"type":"paperback","pages":1,"isbn_13":"9780000000333"}'
echo
sleep 5
TRACE_BOOK=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/details-write --since=20s 2>&1 \
  | grep '"title":"span-test-book"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "book-added trace_id: $TRACE_BOOK"
```

Expected: a 32-char hex string.

- [ ] **Step 2: review-submitted**

```bash
curl -sS -X POST http://localhost:8080/v1/reviews \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"span-test-prod","reviewer":"u","text":"span audit"}'
echo
sleep 5
TRACE_REVIEW=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write --since=20s 2>&1 \
  | grep '"product_id":"span-test-prod"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "review-submitted trace_id: $TRACE_REVIEW"
```

- [ ] **Step 3: rating-submitted**

```bash
curl -sS -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"span-test-rating","reviewer":"u","stars":4}'
echo
sleep 5
TRACE_RATING=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/ratings-write --since=20s 2>&1 \
  | grep '"product_id":"span-test-rating"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "rating-submitted trace_id: $TRACE_RATING"
```

- [ ] **Step 4: review-deleted**

```bash
RID=$(kubectl --context=k3d-bookinfo-local -n bookinfo exec statefulset/reviews-postgresql -- \
  bash -c "PGPASSWORD=bookinfo psql -U bookinfo -d bookinfo_reviews -t -c \"SELECT id FROM reviews WHERE product_id='span-test-prod' LIMIT 1;\"" 2>&1 | xargs)
echo "Deleting review id: $RID"

curl -sS -X POST http://localhost:8080/v1/reviews/delete \
  -H 'Content-Type: application/json' \
  -d "{\"review_id\":\"$RID\"}"
echo
sleep 5
TRACE_DELETE=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write --since=20s 2>&1 \
  | grep "$RID" \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "review-deleted trace_id: $TRACE_DELETE"
```

- [ ] **Step 5: Print summary table for the user**

```bash
echo "=================================================="
echo "Trace IDs to inspect at http://localhost:3000 (Grafana → Explore → Tempo):"
echo "=================================================="
echo "book-added        : $TRACE_BOOK"
echo "review-submitted  : $TRACE_REVIEW"
echo "rating-submitted  : $TRACE_RATING"
echo "review-deleted    : $TRACE_DELETE"
echo
echo "Expected for each: a span tree with"
echo "  <service>-write-api POST /v1/<resource>      [SERVER]"
echo "  ├── pool.acquire / INSERT / etc              [INTERNAL via otelpgx]"
echo "  └── <topic> publish                          [PRODUCER, NEW]"
echo "      └── eventsource.receive                  [SERVER, argo-events fork]"
echo "          └── eventsource.publish              [PRODUCER]"
echo "              └── eventbus consume             [CONSUMER]"
echo "                  └── trigger HTTP             [CLIENT]"
echo "                      └── notification-api     [SERVER]"
```

---

## Task 9: Push to PR #54

- [ ] **Step 1: Verify branch and clean state**

```bash
git branch --show-current
git status
```

Expected: on `feat/event-driven-notifications`, clean working tree.

- [ ] **Step 2: Push**

```bash
git push
```

Expected: branch updated; PR #54 picks up the new commits.

- [ ] **Step 3: Watch CI**

```bash
gh pr checks 54 --watch --interval 30 --fail-fast
```

Expected: all checks green.

---

## Self-Review

**Spec coverage:**

- Decision 1 (hand-rolled, not kotel) → Tasks 1-5 use the manual approach ✔
- Decision 2 (raw attribute strings) → Task 1 uses `attribute.String("messaging.system", "kafka")` etc. ✔
- Decision 3 (span name `<topic> publish`) → Task 1 uses `topic+" publish"` ✔
- Decision 4 (all four producers in scope) → Tasks 2-5 cover details/reviews/ratings/ingestion ✔
- Decision 5 (RecordError + SetStatus on failure) → Tasks 2-5 each include the error-recording branch ✔
- Component: helper in `pkg/telemetry/kafka.go` → Task 1 ✔
- Component: per-producer 4-line wrap → Tasks 2-5 ✔
- Error-handling: ProduceSync error path records on span → Tasks 2-5 ✔
- Testing: helper unit test → Task 1 (two test cases) ✔
- Testing: existing producer tests continue passing → Tasks 2-5 verify with full test runs ✔
- Testing: cluster smoke test capturing 4 trace IDs → Task 8 ✔
- Acceptance criteria 1-5 → Task 1 (helper test), Tasks 2-5 (producer tests), Task 8 (cluster Tempo verification) ✔

**Placeholder scan:** no "TBD", "TODO", "implement later". The "If `WithContext` exists" type of conditional check from prior plans is not present here — Task 1 specifies exactly what to import and write.

**Type consistency:**

- `StartProducerSpan(ctx context.Context, topic, key string) (context.Context, trace.Span)` is the same signature in Task 1 (definition) and Tasks 2-5 (call sites).
- Each producer call site passes the appropriate per-service key: `evt.IdempotencyKey` for details/reviews/ratings, `book.ISBN` for ingestion. The `key` parameter is type-consistent (`string`).
- Error recording uses `span.RecordError(err)` and `span.SetStatus(codes.Error, err.Error())` consistently across Tasks 2-5.
- `defer span.End()` is consistent across all wraps.
