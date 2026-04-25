# Kafka EventSource Consume Span Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Insert a `eventsource.consume` CONSUMER span between the upstream producer span and the existing `eventsource.publish` PRODUCER span on the Argo Events Kafka EventSource side only. After this change, Tempo shows the kafka source's read-from-topic operation as a distinct span ahead of the publish-to-eventbus span.

**Architecture:** Single-file edit to `pkg/eventsources/sources/kafka/start.go` in the argo-events fork. Both `processOne` closures (consumerGroupConsumer and partitionConsumer) extract upstream traceparent from `msg.Headers`, start a CONSUMER span, then re-inject the consume span's traceparent back into the headers map. The existing `WithKafkaHeaders` + `SpanFromCloudEvent` chain in `eventing.go` then makes the `eventsource.publish` PRODUCER span a child of the new CONSUMER. No edits to shared code; webhook and other source types untouched.

**Tech Stack:** Go 1.22, sarama (kafka client), `go.opentelemetry.io/otel`, argo-events fork's `tracing` package.

**Spec reference:** `docs/superpowers/specs/2026-04-25-kafka-eventsource-consume-span-design.md`

**Repo:** `/Users/kaio.fellipe/Documents/git/others/argo-events` (NOT the bookinfo repo)
**Branch:** `feat/cloudevents-compliance-otel-tracing` (PR 3961)
**Remote:** `fork` = `git@github.com:kaio6fellipe/argo-events.git`

---

## File Structure

Single file modified in argo-events fork. No new files. No test file edits (unit-test mocking sarama messages is heavyweight; cluster Tempo verification is the acceptance gate).

```text
pkg/eventsources/sources/kafka/start.go    # +consume span in both processOne closures
```

After this plan completes, the bookinfo cluster also gets a smoke verification (no source code change there) — the cluster pod restart and Tempo inspection are part of acceptance.

---

## Hard Constraints (apply to every commit in this plan)

These come from the design spec; failure to follow them means the work is rejected:

1. **Append-only on `feat/cloudevents-compliance-otel-tracing`.** The branch currently has 21 commits ahead of master. The new commit is the 22nd. The previous 21 commits MUST remain bit-identical after the push (verify with diff against a baseline snapshot).
2. **Sign-off every commit** via `git commit -s`. DCO required by argoproj/argo-events upstream.
3. **NO `Co-Authored-By: Claude ... <noreply@anthropic.com>` trailer.** NO `Co-Authored-By` of any kind on argo-events fork commits. (The bookinfo-side spec/plan commits in `event-driven-bookinfo` repo can keep the Claude trailer; this constraint applies to argo-events ONLY.)
4. **Fast-forward push only.** No `--force` / `--force-with-lease`.
5. **No edits to `eventing.go`** or other shared code. The Kafka source is the only file that changes.

---

## Task 1: Snapshot baseline + switch branch

**Files:** none modified

- [ ] **Step 1: Open the argo-events repo and verify clean state**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git status
```

Expected: clean working tree.

- [ ] **Step 2: Switch to the PR-3961 branch and sync**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git checkout feat/cloudevents-compliance-otel-tracing
git fetch fork feat/cloudevents-compliance-otel-tracing
git status
[ "$(git rev-parse HEAD)" = "$(git rev-parse fork/feat/cloudevents-compliance-otel-tracing)" ] && echo "LOCAL EQ FORK" || echo "DIVERGED"
```

Expected: `LOCAL EQ FORK`. If `DIVERGED`, run `git pull --ff-only fork feat/cloudevents-compliance-otel-tracing` and re-check.

- [ ] **Step 3: Snapshot the existing 21 commits to a baseline file**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git log --oneline master..feat/cloudevents-compliance-otel-tracing > /tmp/argo-events-pr3961-baseline-r2.txt
wc -l /tmp/argo-events-pr3961-baseline-r2.txt
```

Expected: `21 /tmp/...`. The top line is `9d2ed451 feat(eventsource/kafka): propagate W3C trace context from record headers` (the prior PR-3961 round's last commit).

- [ ] **Step 4: Verify `tracing.StartConsumerSpan` helper is present**

```bash
grep -n "func StartConsumerSpan" pkg/shared/tracing/tracing.go
```

Expected: a single line near `:140` showing `func StartConsumerSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span)`.

If absent, abort — the spec assumes this helper exists. (It was added by an earlier PR-3961 commit.)

- [ ] **Step 5: Verify imports already present in `start.go`**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
grep -E "go.opentelemetry.io/otel|/eventsourcecommon|/shared/tracing" pkg/eventsources/sources/kafka/start.go | head -10
```

Expected: shows imports for `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/attribute`, `go.opentelemetry.io/otel/propagation`, `eventsourcecommon`, and `tracing`. If `go.opentelemetry.io/otel/codes` is missing, Task 2 will add it.

---

## Task 2: Add consume span in `consumerGroupConsumer.processOne`

**Files:**
- Modify: `/Users/kaio.fellipe/Documents/git/others/argo-events/pkg/eventsources/sources/kafka/start.go`

This task edits the FIRST `processOne` closure (around line 263). Task 3 edits the second (partitionConsumer, around line 449) with the same pattern.

- [ ] **Step 1: Locate the first dispatch site and surrounding code**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
grep -n "dispatch(eventBody" pkg/eventsources/sources/kafka/start.go
```

Expected: two lines, first around 291 (consumerGroupConsumer) and second around 482 (partitionConsumer). Note both line numbers; use the first one for this task.

- [ ] **Step 2: Read the consumerGroupConsumer block from headers map to dispatch**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
sed -n '260,295p' pkg/eventsources/sources/kafka/start.go
```

Expected: shows the `headers := make(map[string]string)` block, the for-loop populating it from `msg.Headers`, the `eventData.Headers = headers` assignment, the json marshal, and the `dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers))` call.

- [ ] **Step 3: Add `codes` import if missing**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
grep -n "go.opentelemetry.io/otel/codes" pkg/eventsources/sources/kafka/start.go
```

If the grep returns nothing, edit the imports block to add (alongside the other otel imports):

```go
"go.opentelemetry.io/otel/codes"
```

If the grep returns a line, skip this step.

- [ ] **Step 4: Replace the dispatch block in consumerGroupConsumer**

In the first `processOne` closure inside `consumerGroupConsumer`, replace this exact block:

```go
		headers := make(map[string]string)

		for _, recordHeader := range msg.Headers {
			headers[string(recordHeader.Key)] = string(recordHeader.Value)
		}

		eventData.Headers = headers

		if kafkaEventSource.JSONBody {
			if el.SchemaRegistry != nil {
				value, err := toJson(el.SchemaRegistry, msg)
				if err != nil {
					return fmt.Errorf("failed to retrieve json value using the schema registry, %w", err)
				}
				eventData.Body = (*json.RawMessage)(&value)
			} else {
				eventData.Body = (*json.RawMessage)(&msg.Value)
			}
		} else {
			eventData.Body = msg.Value
		}
		eventBody, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("failed to marshal the event data, rejecting the event, %w", err)
		}

		kafkaID := genUniqueID(el.GetEventSourceName(), el.GetEventName(), kafkaEventSource.URL, msg.Topic, msg.Partition, msg.Offset)

		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
			return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
		}
```

with:

```go
		headers := make(map[string]string)

		for _, recordHeader := range msg.Headers {
			headers[string(recordHeader.Key)] = string(recordHeader.Value)
		}

		// Extract upstream W3C trace context from the Kafka record headers
		parentCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))

		// Start an eventsource.consume CONSUMER span as the parent for the
		// downstream eventsource.publish PRODUCER span emitted in eventing.go.
		spanCtx, consumeSpan := tracing.StartConsumerSpan(parentCtx, otel.Tracer("argo-events-eventsource"), "eventsource.consume",
			attribute.String("eventsource.name", el.GetEventSourceName()),
			attribute.String("eventsource.type", "kafka"),
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", msg.Topic),
			attribute.String("messaging.operation.type", "receive"),
			attribute.String("messaging.kafka.message.key", string(msg.Key)),
			attribute.Int("messaging.kafka.message.partition", int(msg.Partition)),
			attribute.Int64("messaging.kafka.message.offset", msg.Offset),
		)
		defer consumeSpan.End()

		// Re-inject the CONSUMER span's trace context into the headers map so
		// WithKafkaHeaders writes the CONSUMER traceparent as the CloudEvent
		// extension. eventing.go's SpanFromCloudEvent then makes the
		// eventsource.publish PRODUCER span a child of CONSUMER.
		otel.GetTextMapPropagator().Inject(spanCtx, propagation.MapCarrier(headers))

		eventData.Headers = headers

		if kafkaEventSource.JSONBody {
			if el.SchemaRegistry != nil {
				value, err := toJson(el.SchemaRegistry, msg)
				if err != nil {
					return fmt.Errorf("failed to retrieve json value using the schema registry, %w", err)
				}
				eventData.Body = (*json.RawMessage)(&value)
			} else {
				eventData.Body = (*json.RawMessage)(&msg.Value)
			}
		} else {
			eventData.Body = msg.Value
		}
		eventBody, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("failed to marshal the event data, rejecting the event, %w", err)
		}

		kafkaID := genUniqueID(el.GetEventSourceName(), el.GetEventName(), kafkaEventSource.URL, msg.Topic, msg.Partition, msg.Offset)

		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
			consumeSpan.RecordError(err)
			consumeSpan.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
		}
```

The diff inserts:
1. Extract upstream traceparent
2. Start CONSUMER span (with `defer span.End()`)
3. Re-inject CONSUMER traceparent
4. Add `RecordError`/`SetStatus` on dispatch error

`eventData.Headers = headers` MOVES from before-inject to after-inject so the dispatched event body carries the CONSUMER's traceparent.

- [ ] **Step 5: Build to confirm compile**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
GOFLAGS=-mod=mod go build ./pkg/eventsources/sources/kafka/...
```

Expected: no output, exit 0. If build fails, fix imports/syntax before continuing.

- [ ] **Step 6: Run gofmt + vet**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
gofmt -w pkg/eventsources/sources/kafka/start.go
GOFLAGS=-mod=mod go vet ./pkg/eventsources/sources/kafka/...
```

Expected: no formatter changes (or trivial whitespace), no vet errors.

- [ ] **Step 7: Verify second dispatch site is still present and untouched**

```bash
grep -n "dispatch(eventBody" pkg/eventsources/sources/kafka/start.go
```

Expected: TWO matches. The first (in consumerGroupConsumer) now has the CONSUMER wrap; the second (in partitionConsumer) is still the bare form. Task 3 will update the second.

> NOTE: do NOT commit yet — Task 3 edits the same file. One commit covers both edits.

---

## Task 3: Add consume span in `partitionConsumer.processOne`

**Files:**
- Modify: `/Users/kaio.fellipe/Documents/git/others/argo-events/pkg/eventsources/sources/kafka/start.go`

This is the second dispatch site, around line 482 (post-Task-2 line numbers may shift slightly). Same edit as Task 2.

- [ ] **Step 1: Locate the second dispatch site**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
grep -n "dispatch(eventBody" pkg/eventsources/sources/kafka/start.go
```

Expected: two matches; the second is the one to update.

- [ ] **Step 2: Read the partitionConsumer block from headers map to dispatch**

Look at the `partitionConsumer` function (`func (el *EventListener) partitionConsumer(...)`). The structure of its `processOne` closure mirrors consumerGroupConsumer's:

```go
		headers := make(map[string]string)

		for _, recordHeader := range message.Headers {
			headers[string(recordHeader.Key)] = string(recordHeader.Value)
		}

		eventData.Headers = headers

		if kafkaEventSource.JSONBody {
			if el.SchemaRegistry != nil {
				value, err := toJson(el.SchemaRegistry, message)
				if err != nil {
					return fmt.Errorf("failed to retrieve json value using the schema registry, %w", err)
				}
				eventData.Body = (*json.RawMessage)(&value)
			} else {
				eventData.Body = (*json.RawMessage)(&message.Value)
			}
		} else {
			eventData.Body = message.Value
		}
		eventBody, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("failed to marshal the event data, rejecting the event, %w", err)
		}

		kafkaID := genUniqueID(el.GetEventSourceName(), el.GetEventName(), kafkaEventSource.URL, message.Topic, message.Partition, message.Offset)

		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
			return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
		}
```

> NOTE: variable name is `message` here (vs `msg` in consumerGroupConsumer). Use `message` consistently in the new code.

- [ ] **Step 3: Replace the partitionConsumer block**

Replace it with:

```go
		headers := make(map[string]string)

		for _, recordHeader := range message.Headers {
			headers[string(recordHeader.Key)] = string(recordHeader.Value)
		}

		// Extract upstream W3C trace context from the Kafka record headers
		parentCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))

		// Start an eventsource.consume CONSUMER span as the parent for the
		// downstream eventsource.publish PRODUCER span emitted in eventing.go.
		spanCtx, consumeSpan := tracing.StartConsumerSpan(parentCtx, otel.Tracer("argo-events-eventsource"), "eventsource.consume",
			attribute.String("eventsource.name", el.GetEventSourceName()),
			attribute.String("eventsource.type", "kafka"),
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", message.Topic),
			attribute.String("messaging.operation.type", "receive"),
			attribute.String("messaging.kafka.message.key", string(message.Key)),
			attribute.Int("messaging.kafka.message.partition", int(message.Partition)),
			attribute.Int64("messaging.kafka.message.offset", message.Offset),
		)
		defer consumeSpan.End()

		// Re-inject the CONSUMER span's trace context into the headers map so
		// WithKafkaHeaders writes the CONSUMER traceparent as the CloudEvent
		// extension. eventing.go's SpanFromCloudEvent then makes the
		// eventsource.publish PRODUCER span a child of CONSUMER.
		otel.GetTextMapPropagator().Inject(spanCtx, propagation.MapCarrier(headers))

		eventData.Headers = headers

		if kafkaEventSource.JSONBody {
			if el.SchemaRegistry != nil {
				value, err := toJson(el.SchemaRegistry, message)
				if err != nil {
					return fmt.Errorf("failed to retrieve json value using the schema registry, %w", err)
				}
				eventData.Body = (*json.RawMessage)(&value)
			} else {
				eventData.Body = (*json.RawMessage)(&message.Value)
			}
		} else {
			eventData.Body = message.Value
		}
		eventBody, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("failed to marshal the event data, rejecting the event, %w", err)
		}

		kafkaID := genUniqueID(el.GetEventSourceName(), el.GetEventName(), kafkaEventSource.URL, message.Topic, message.Partition, message.Offset)

		if err = dispatch(eventBody, eventsourcecommon.WithID(kafkaID), eventsourcecommon.WithKafkaHeaders(headers)); err != nil {
			consumeSpan.RecordError(err)
			consumeSpan.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("failed to dispatch a Kafka event, %w", err)
		}
```

> NOTE: identical pattern to Task 2, only with `message.*` instead of `msg.*`. The function-local variable `ctx` may not exist at this scope in `partitionConsumer.processOne` — verify by inspecting the enclosing function signature. The closure captures whatever ctx is in scope; both processOne closures should have access to a `ctx`. If `ctx` is unavailable, use `context.Background()` and report DONE_WITH_CONCERNS so the controller can fix.

- [ ] **Step 4: Build, vet, gofmt**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
gofmt -w pkg/eventsources/sources/kafka/start.go
GOFLAGS=-mod=mod go build ./pkg/eventsources/sources/kafka/...
GOFLAGS=-mod=mod go vet ./pkg/eventsources/sources/kafka/...
```

Expected: clean. If build fails on `ctx` not being in scope at the partitionConsumer site, see the NOTE in Step 3.

- [ ] **Step 5: Verify both dispatch sites updated**

```bash
grep -B2 "consumeSpan.End" pkg/eventsources/sources/kafka/start.go | head -20
```

Expected: TWO `defer consumeSpan.End()` lines visible (one per processOne closure).

- [ ] **Step 6: Verify the existing 21 commits are still present and unchanged**

```bash
diff <(git log --oneline master..HEAD) /tmp/argo-events-pr3961-baseline-r2.txt && echo "BASELINE INTACT"
```

Expected: `BASELINE INTACT` (no `+`/`-` lines). The working tree has uncommitted edits — those don't appear in the log.

- [ ] **Step 7: Stage and commit (signed off, no Claude trailer)**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git add pkg/eventsources/sources/kafka/start.go
git commit -s -m "feat(eventsource/kafka): add eventsource.consume CONSUMER span before dispatch

Inserts an eventsource.consume CONSUMER span in both processOne
closures (consumerGroupConsumer and partitionConsumer). The span is
parented to the upstream traceparent extracted from msg.Headers, then
its own traceparent is re-injected into the headers map so
WithKafkaHeaders + SpanFromCloudEvent in eventing.go make the existing
eventsource.publish PRODUCER span a child of CONSUMER (instead of
directly under upstream).

Span carries OTel messaging semconv attributes: messaging.system=kafka,
destination.name=<topic>, operation.type=receive, kafka.message.key,
kafka.message.partition, kafka.message.offset. Errors from dispatch
are recorded on the CONSUMER span via RecordError + SetStatus.

Webhook and other source types are not affected — only the kafka
source's processOne closures get the new span."
```

- [ ] **Step 8: Verify commit trailer**

```bash
git log -1 --format=%B
```

Expected output ENDS with exactly:

```
Signed-off-by: <Name> <email>
```

NO `Co-Authored-By` anywhere in the output. If `Co-Authored-By` is present, run `git reset HEAD~1`, recommit without that trailer.

- [ ] **Step 9: Verify branch state**

```bash
git log --oneline master..HEAD | wc -l
```

Expected: `22`.

```bash
diff <(git log --oneline master..HEAD | tail -21) /tmp/argo-events-pr3961-baseline-r2.txt && echo "BASELINE INTACT (top 21 unchanged, new commit on top)"
```

Expected: `BASELINE INTACT (top 21 unchanged, new commit on top)`.

- [ ] **Step 10: Push to fork (fast-forward only)**

```bash
git push fork feat/cloudevents-compliance-otel-tracing
```

Expected: fast-forward push. PR 3961 picks up the new commit.

If rejected with "non-fast-forward", STOP — do not force. Investigate (likely someone else pushed) and report.

---

## Task 4: Cherry-pick onto `feat/combined-prs-3961-3983`

**Files:** none modified directly; cherry-picks the Task-3 commit

- [ ] **Step 1: Capture the new commit SHA**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
NEW_SHA=$(git rev-parse HEAD)
echo "New commit SHA: $NEW_SHA"
git log --format='%s' -1 HEAD
```

Expected: SHA of the Task-3 commit; subject `feat(eventsource/kafka): add eventsource.consume CONSUMER span before dispatch`.

- [ ] **Step 2: Switch to the consumed branch and sync**

```bash
git checkout feat/combined-prs-3961-3983
git fetch fork feat/combined-prs-3961-3983
git pull --ff-only fork feat/combined-prs-3961-3983
```

Expected: `Already up to date` or fast-forward.

- [ ] **Step 3: Cherry-pick with sign-off**

```bash
git cherry-pick -s "$NEW_SHA"
```

Expected: clean cherry-pick. Conflict is unlikely — combined branch's only divergence from `feat/cloudevents-compliance-otel-tracing` is the kafka-latency commits and the codegen merge, none of which touch `pkg/eventsources/sources/kafka/start.go` in the same lines.

If a conflict arises, STOP and report. Do not force.

- [ ] **Step 4: Build to confirm**

```bash
GOFLAGS=-mod=mod go build ./pkg/eventsources/sources/kafka/...
```

Expected: clean.

- [ ] **Step 5: Verify cherry-picked commit is on top**

```bash
git log --oneline -2
```

Expected: top line is the cherry-picked commit on this branch (different SHA from `feat/cloudevents-compliance-otel-tracing` because cherry-pick rewrites SHAs); subject matches.

- [ ] **Step 6: Verify trailer on the cherry-picked commit**

```bash
git log -1 --format=%B
```

Expected: ends with `Signed-off-by: ...`. NO `Co-Authored-By`.

- [ ] **Step 7: Push to fork**

```bash
git push fork feat/combined-prs-3961-3983
```

Expected: fast-forward push.

---

## Task 5: Build + push image to ghcr.io

- [ ] **Step 1: Confirm on combined branch**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
git branch --show-current
git log --oneline -1
```

Expected: branch `feat/combined-prs-3961-3983`; HEAD is the cherry-picked commit from Task 4.

- [ ] **Step 2: Purge stale dist artifacts**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
rm -f dist/argo-events-linux-*
ls dist/
```

Expected: only `kubefied.swagger.json` and `kubernetes.swagger.json` remain (or similar non-binary files); no `argo-events-linux-*` files.

> WHY this matters: Make sees the existing `.gz` files and skips rebuild, packaging stale binaries. Last round this caused the image to ship the old code.

- [ ] **Step 3: Verify ghcr.io login**

```bash
cat ~/.docker/config.json | python3 -c 'import json,sys; print("ghcr.io" in json.load(sys.stdin).get("auths", {}))'
```

Expected: `True`. If `False`, run `docker login ghcr.io -u kaio6fellipe` first (interactive — abort plan execution and ask the user).

- [ ] **Step 4: Build and push the multi-arch image**

```bash
cd /Users/kaio.fellipe/Documents/git/others/argo-events
GOFLAGS=-mod=mod IMAGE_NAMESPACE=ghcr.io/kaio6fellipe DOCKER_PUSH=true VERSION=prs-3961-3983 make image-multi 2>&1 | tee /tmp/argo-events-image-build-r2.log
```

Expected: exit 0. Log ends with the buildx push step. Takes 5-10 min on arm64 Mac.

If the build hits the `inconsistent vendoring` error, the `GOFLAGS=-mod=mod` flag should bypass it. If it still fails, investigate the error message and STOP.

- [ ] **Step 5: Verify the manifest digest changed**

```bash
docker buildx imagetools inspect ghcr.io/kaio6fellipe/argo-events:prs-3961-3983 2>&1 | head -10
```

Expected: a `Digest:` line. Note the digest. Compare against the prior round's `4ae7d54320c22b5f56f2b24886ed6c190903d65f1b9631366d678791e3b8fa38` — it MUST be different.

- [ ] **Step 6: Verify the embedded gitCommit matches**

```bash
docker pull --quiet ghcr.io/kaio6fellipe/argo-events:prs-3961-3983
docker create --name ae-inspect-r2 ghcr.io/kaio6fellipe/argo-events:prs-3961-3983
docker cp ae-inspect-r2:/bin/argo-events /tmp/argo-events-binary-r2
docker rm ae-inspect-r2
echo "=== gitCommit ==="
strings /tmp/argo-events-binary-r2 | grep -oE "gitCommit=[a-f0-9]{40}" | head -1
echo "=== buildDate ==="
strings /tmp/argo-events-binary-r2 | grep -oE "buildDate=[0-9TZ:-]+" | head -1
echo "=== expected gitCommit (combined branch HEAD) ==="
cd /Users/kaio.fellipe/Documents/git/others/argo-events && git rev-parse HEAD
```

Expected: gitCommit matches the combined-branch HEAD. buildDate is today's UTC.

If gitCommit DOESN'T match, the build packaged a stale binary. Re-purge dist/ and rebuild.

---

## Task 6: Pull new image into k3d cluster

- [ ] **Step 1: Confirm bookinfo cluster up**

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get pods --no-headers | awk '{print $3}' | sort | uniq -c
```

Expected: only `Running`. If pods are missing, ask user — cluster may have been torn down.

- [ ] **Step 2: Import the new image into k3d**

```bash
k3d image import ghcr.io/kaio6fellipe/argo-events:prs-3961-3983 -c bookinfo-local
```

Expected: `Successfully imported image(s)`.

- [ ] **Step 3: Restart EventSource and Sensor pods to pick up the new image**

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo delete pod -l eventsource-name
kubectl --context=k3d-bookinfo-local -n bookinfo delete pod -l sensor-name
sleep 12
kubectl --context=k3d-bookinfo-local -n bookinfo get pods -l eventsource-name --no-headers | head
```

Expected: pods restarted, all `1/1 Running`.

- [ ] **Step 4: Verify a pod is on the expected new digest**

```bash
EXPECTED_DIGEST=$(docker buildx imagetools inspect ghcr.io/kaio6fellipe/argo-events:prs-3961-3983 2>&1 | grep "^Digest:" | awk '{print $2}')
ACTUAL_IMAGE_ID=$(kubectl --context=k3d-bookinfo-local -n bookinfo get pod -l eventsource-name=reviews-events -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')
echo "Expected digest: $EXPECTED_DIGEST"
echo "Pod imageID:     $ACTUAL_IMAGE_ID"
```

Expected: pod's `imageID` ends with the manifest's digest sha256.

If they don't match, the pod is still on the old image — re-run Step 2 (`k3d image import`) and Step 3 (delete pods).

---

## Task 7: Cluster smoke test — capture trace IDs to verify consume span

After this task completes, the user inspects each trace ID in Grafana Tempo at `http://localhost:3000`.

- [ ] **Step 1: book-added**

```bash
curl -sS -X POST http://localhost:8080/v1/details \
  -H 'Content-Type: application/json' \
  -d '{"title":"consume-test-book","author":"a","year":2026,"type":"paperback","pages":1,"isbn_13":"9780000000444"}'
echo
sleep 5
TRACE_BOOK=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/details-write --since=20s 2>&1 \
  | grep '"title":"consume-test-book"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "book-added trace_id: $TRACE_BOOK"
```

Expected: 32-char hex trace_id printed.

- [ ] **Step 2: review-submitted**

```bash
curl -sS -X POST http://localhost:8080/v1/reviews \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"consume-test-prod","reviewer":"u","text":"consume audit"}'
echo
sleep 5
TRACE_REVIEW=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write --since=20s 2>&1 \
  | grep '"product_id":"consume-test-prod"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "review-submitted trace_id: $TRACE_REVIEW"
```

- [ ] **Step 3: rating-submitted**

```bash
curl -sS -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"consume-test-rating","reviewer":"u","stars":4}'
echo
sleep 5
TRACE_RATING=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/ratings-write --since=20s 2>&1 \
  | grep '"product_id":"consume-test-rating"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "rating-submitted trace_id: $TRACE_RATING"
```

- [ ] **Step 4: review-deleted**

Insert a fresh review then delete it (so no log-line confusion with prior review-submitted entries):

```bash
curl -sS -X POST http://localhost:8080/v1/reviews \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"consume-test-prod-del","reviewer":"u","text":"to delete"}'
echo
sleep 4
DEL_RID=$(kubectl --context=k3d-bookinfo-local -n bookinfo exec statefulset/reviews-postgresql -- \
  bash -c "PGPASSWORD=bookinfo psql -U bookinfo -d bookinfo_reviews -t -c \"SELECT id FROM reviews WHERE product_id='consume-test-prod-del' LIMIT 1;\"" 2>&1 | xargs)
echo "Deleting review_id: $DEL_RID"

curl -sS -X POST http://localhost:8080/v1/reviews/delete \
  -H 'Content-Type: application/json' \
  -d "{\"review_id\":\"$DEL_RID\"}"
echo
sleep 5
TRACE_DELETE=$(kubectl --context=k3d-bookinfo-local -n bookinfo logs deploy/reviews-write --since=20s 2>&1 \
  | grep '"path":"/v1/reviews/delete"' \
  | grep '"msg":"review deleted"' \
  | head -1 \
  | python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["trace_id"])')
echo "review-deleted trace_id: $TRACE_DELETE"
```

- [ ] **Step 5: Print summary table**

```bash
echo "=================================================="
echo "Trace IDs to inspect at http://localhost:3000 (Grafana → Explore → Tempo):"
echo "=================================================="
echo "book-added        : $TRACE_BOOK"
echo "review-submitted  : $TRACE_REVIEW"
echo "rating-submitted  : $TRACE_RATING"
echo "review-deleted    : $TRACE_DELETE"
echo
echo "Expected span tree per trace:"
echo "  <service>-write-api POST /v1/<resource>      [SERVER]"
echo "  ├── pool.acquire / INSERT / etc              [INTERNAL via otelpgx]"
echo "  └── bookinfo_<service>_events publish        [PRODUCER]"
echo "      └── eventsource.consume                  [CONSUMER]  ← NEW"
echo "          └── eventsource.publish              [PRODUCER]"
echo "              └── eventbus consume             [CONSUMER]"
echo "                  └── trigger HTTP             [CLIENT]"
echo "                      └── notification-api     [SERVER]"
```

The user opens each trace_id in Tempo and confirms the new `eventsource.consume` span sits between `bookinfo_<service>_events publish` and `eventsource.publish`.

---

## Self-Review

**Spec coverage:**

- Decision 1 (span name `eventsource.consume`) → Tasks 2, 3 use exactly this name ✔
- Decision 2 (SpanKindConsumer via `tracing.StartConsumerSpan`) → Tasks 2, 3 ✔
- Decision 3 (Kafka source only) → Tasks 2, 3 modify only `pkg/eventsources/sources/kafka/start.go`; no other source touched ✔
- Decision 4 (re-inject CONSUMER traceparent into headers map) → Tasks 2, 3 include the inject step before `eventData.Headers = headers` ✔
- Decision 5 (append-only on PR 3961) → Task 1 baseline snapshot + Task 3 verification ✔
- Decision 6 (cherry-pick to combined branch + image build) → Tasks 4, 5 ✔
- Decision 7 (sign-off `-s`, no Claude trailer) → Task 3 Step 7 commit + Step 8 verification; Task 4 Step 3 cherry-pick `-s` + Step 6 verification ✔
- Component: file modified (`pkg/eventsources/sources/kafka/start.go`) → Tasks 2, 3 ✔
- Component: replacement pattern with all 8 attributes → Tasks 2, 3 list each `attribute.String` line ✔
- Component: imports needed (`codes`) → Task 2 Step 3 conditional ✔
- Component: branch-state preservation → Task 1 snapshot + Task 3 verification ✔
- Component: image build (purge dist/, GOFLAGS=-mod=mod, build-multi) → Task 5 ✔
- Data flow: target span tree → Task 7 Step 5 prints expected tree ✔
- Error handling: dispatch error records on CONSUMER span → Tasks 2 Step 4 + 3 Step 3 include `RecordError`+`SetStatus` ✔
- Testing: build verification → Tasks 2 Step 5, 3 Step 4, 4 Step 4, 5 Step 4 ✔
- Testing: cluster smoke for all 4 event types → Task 7 Steps 1-4 ✔
- Acceptance criteria 1-6 → Task 1 (baseline), Task 3 (commit + sign-off + verify), Task 4 (cherry-pick), Task 5 (image gitCommit match), Task 6 (pod imageID match), Task 7 (Tempo verification) ✔

**Placeholder scan:** no "TBD", "TODO", "implement later", "similar to". The `ctx` availability check in Task 3 Step 3 is a defensive note with concrete instructions ("verify by inspecting … if unavailable, use `context.Background()` and report DONE_WITH_CONCERNS"), not an unfilled placeholder.

**Type consistency:**

- `tracing.StartConsumerSpan(ctx, tracer, name, attrs...)` is the same signature in Task 2 (call) and Task 3 (call). Helper definition lives in `pkg/shared/tracing/tracing.go:140` (verified in Task 1 Step 4).
- `attribute.String/Int/Int64` calls match across both tasks for all 8 attributes.
- `consumeSpan.End()`, `consumeSpan.RecordError(err)`, `consumeSpan.SetStatus(codes.Error, err.Error())` consistent across Tasks 2 and 3.
- Variable names: `msg` vs `message` is correctly distinguished (consumerGroupConsumer uses `msg`, partitionConsumer uses `message`); each task's code uses the right one.
- `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983` is the same image reference across Tasks 5, 6, and 7 verification commands.
