# Restore Direct Action Invocation for Single-Dependency Triggers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the `24f782af` optimization that bypasses the Kafka action topic for single-dependency triggers, reducing event-to-action latency from ~1.5s to <15ms.

**Architecture:** In the `Event()` handler of `kafka_sensor.go`, detect single-dependency triggers via `OneAndDone()` and invoke the action callback directly instead of producing to the action topic. This produces 0 Kafka messages for single-dep triggers, causing `KafkaTransaction.Commit` to hit the `len(messages)==0` early return (simple offset commit, no Kafka transaction).

**Tech Stack:** Go, sarama, Argo Events Kafka sensor

**Repository:** `/Users/kaio.fellipe/Documents/git/others/argo-events` branch `feat/combined-prs-3961-3983`

---

### Task 1: Restore direct action invocation in `Event()` handler

**Files:**
- Modify: `pkg/eventbus/kafka/sensor/kafka_sensor.go:309-361` (the `Event` method)

- [ ] **Step 1: Modify `Event()` to invoke action directly for single-dep triggers**

Replace the current `Event()` method at line 309-361 with:

```go
func (s *KafkaSensor) Event(msg *sarama.ConsumerMessage) ([]*sarama.ProducerMessage, int64, func()) {
	var event *cloudevents.Event
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		s.Logger.Errorw("Failed to deserialize cloudevent, skipping", zap.Error(err))
		return nil, msg.Offset + 1, nil
	}

	messages := []*sarama.ProducerMessage{}
	var fns []func()

	for _, trigger := range s.triggers.List(event) {
		event, err := trigger.Transform(trigger.depName, event)
		if err != nil {
			s.Logger.Errorw("Failed to transform cloudevent, skipping", zap.Error(err))
			continue
		}

		if !trigger.Filter(trigger.depName, event) {
			s.Logger.Debug("Filter condition satisfied, skipping")
			continue
		}

		if trigger.OneAndDone() {
			// Single-dependency: invoke action directly, skip action topic.
			// This avoids a full Kafka transaction (BeginTxn + Produce +
			// AddOffsetsToTxn + CommitTxn) which can add seconds of latency
			// due to CONCURRENT_TRANSACTIONS retries in sarama.
			f := trigger.Action([]*cloudevents.Event{event}, trigger.depName)
			if f != nil {
				fns = append(fns, f)
			}
			continue
		}

		// Multi-dependency: route through trigger topic for dependency aggregation
		value, err := json.Marshal(event)
		if err != nil {
			s.Logger.Errorw("Failed to serialize cloudevent, skipping", zap.Error(err))
			continue
		}

		messages = append(messages, &sarama.ProducerMessage{
			Topic: s.topics.trigger,
			Key:   sarama.StringEncoder(trigger.Name()),
			Value: sarama.ByteEncoder(value),
			Headers: []sarama.RecordHeader{{
				Key:   []byte(dependencyNameHeader),
				Value: []byte(trigger.depName),
			}},
		})
	}

	// Compose all direct-action callbacks into a single function
	var fn func()
	if len(fns) > 0 {
		fn = func() {
			for _, f := range fns {
				f()
			}
		}
	}

	return messages, msg.Offset + 1, fn
}
```

Key changes from the current code:
1. Added `var fns []func()` to collect action callbacks
2. `trigger.OneAndDone()` branch calls `trigger.Action()` directly and appends to `fns`, then `continue` (no message produced)
3. Multi-dep triggers produce to `s.topics.trigger` (not `s.topics.action`)
4. Returns composed `fn` callback instead of `nil`

- [ ] **Step 2: Verify the build compiles**

Run from the argo-events repo root:

```bash
go build ./...
```

Expected: Clean build, no errors.

- [ ] **Step 3: Run existing unit tests**

```bash
go test ./pkg/eventbus/kafka/sensor/... -v -count=1
```

Expected: All existing tests pass. The `Event()` function may not have direct unit tests — verify by checking test output.

- [ ] **Step 4: Commit**

```bash
git add pkg/eventbus/kafka/sensor/kafka_sensor.go
git commit -m "perf: restore direct action invocation for single-dep triggers

For single-dependency triggers (OneAndDone), invoke the action callback
directly instead of routing through the Kafka action topic. This avoids
a full Kafka transaction (BeginTxn + Produce + AddOffsetsToTxn + CommitTxn)
per message, eliminating ~1.5s of CONCURRENT_TRANSACTIONS retry latency.

The optimization was originally in 24f782af but was removed during the
consumerBatchMaxWait refactor. Multi-dependency triggers still route
through the trigger topic for dependency aggregation."
```

---

### Task 2: Build, deploy, and validate latency improvement

**Files:**
- No code changes — this is a validation task

- [ ] **Step 1: Build the updated image**

From the argo-events repo root:

```bash
make image IMAGE_NAMESPACE=ghcr.io/kaio6fellipe VERSION=prs-3961-3983 DOCKER_PUSH=true
```

This builds the arm64 image, pushes to ghcr.io, and imports into the k3d cluster.

- [ ] **Step 2: Restart sensor pods to pick up new image**

```bash
kubectl --context=k3d-bookinfo-local delete pods -n bookinfo -l sensor-name
kubectl --context=k3d-bookinfo-local wait --for=condition=Ready pods -n bookinfo -l sensor-name --timeout=60s
```

- [ ] **Step 3: Wait for consumers to stabilize and send test request**

```bash
sleep 15
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id":"test-single-dep-fix","rating":5}'
```

Expected: `success` with HTTP 200.

- [ ] **Step 4: Check sensor logs for timing**

```bash
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l sensor-name=rating-submitted-sensor --tail=10 | grep -E "Received|Begin transaction|Finished|Making a http"
```

Expected:
- `Received message` on `argo-events` topic
- **No** `Begin transaction` / `Finished transaction` logs (no Kafka transaction)
- `Making a http request` within ~5-15ms of `Received message`

If you still see `Begin transaction`, the `OneAndDone()` check is not being hit — verify the sensor has the updated image by checking the version log line at startup.

- [ ] **Step 5: Check the Tempo trace**

```bash
sleep 8
TEMPO_UID=$(curl -s -u "$GRAFANA_USER:$GRAFANA_PASS" http://localhost:3000/api/datasources | python3 -c "import sys,json; ds=[d for d in json.load(sys.stdin) if d['type']=='tempo']; print(ds[0]['uid'])")
curl -s -u "$GRAFANA_USER:$GRAFANA_PASS" "http://localhost:3000/api/datasources/proxy/uid/$TEMPO_UID/api/search?limit=3&start=$(($(date +%s) - 60))&end=$(date +%s)" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for t in data.get('traces', []):
    print(f\"TraceID: {t['traceID']}  duration: {t.get('durationMs',0)}ms\")
"
```

Fetch the most recent trace and verify the gap between `eventsource.publish` end and `sensor.trigger` start is <50ms.
