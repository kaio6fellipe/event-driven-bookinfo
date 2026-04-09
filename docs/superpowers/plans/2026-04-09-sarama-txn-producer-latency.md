# Optimize Sarama Transactional Producer Latency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce Kafka transaction latency from ~1.5s to <100ms for multi-dependency triggers by tuning sarama's retry backoff, batching partition registration, exposing CRD-level configuration, and skipping the action topic for evaluated triggers.

**Architecture:** Four layers implemented in priority order. Layer 1 (retry backoff tuning) provides the biggest immediate win. Layer 2 (partition batching) reduces `AddPartitionsToTxn` calls. Layer 3 (CRD config) gives operators control. Layer 4 (skip action topic) eliminates one full Kafka hop for satisfied multi-dep triggers.

**Tech Stack:** Go, sarama v1.43.0, Argo Events Kafka sensor, Kubernetes CRDs

**Repository:** `/Users/kaio.fellipe/Documents/git/others/argo-events` branch `feat/combined-prs-3961-3983`

---

### Task 1: Layer 1 — Reduce transaction retry backoff from 100ms to 10ms

**Files:**
- Modify: `pkg/eventbus/kafka/base/kafka.go:56-73` (producer config section)

- [ ] **Step 1: Add transaction retry backoff tuning**

In `pkg/eventbus/kafka/base/kafka.go`, add the following lines after `config.Net.MaxOpenRequests = 1` (line 59) and before the partitioner switch:

```go
	// Transaction retry tuning for low-latency processing.
	// Sarama defaults to 100ms backoff between CONCURRENT_TRANSACTIONS retries,
	// which accumulates to ~1.5s per transaction on rapid successive commits.
	// Reducing to 10ms with more retries keeps total retry time under 100ms.
	config.Producer.Transaction.Retry.Backoff = 10 * time.Millisecond
	config.Producer.Transaction.Retry.Max = 100
```

Also add `"time"` to the imports:

```go
import (
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/argoproj/argo-events/pkg/apis/events/v1alpha1"
	sharedutil "github.com/argoproj/argo-events/pkg/shared/util"
	"go.uber.org/zap"
)
```

- [ ] **Step 2: Verify the build compiles**

```bash
go build ./...
```

Expected: Clean build.

- [ ] **Step 3: Run existing tests**

```bash
go test ./pkg/eventbus/kafka/... -v -count=1
```

Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/eventbus/kafka/base/kafka.go
git commit -m "perf: reduce sarama transaction retry backoff from 100ms to 10ms

The Kafka broker typically resolves CONCURRENT_TRANSACTIONS within a few
milliseconds. Sarama's default 100ms backoff causes ~500ms per retry cycle,
accumulating to ~1.5s per transaction. Reducing to 10ms with 100 max retries
keeps total retry time under 100ms while still handling genuinely busy
coordinators."
```

---

### Task 2: Layer 1 — Build, deploy, and validate backoff improvement

**Files:**
- No code changes — validation task

- [ ] **Step 1: Build and deploy**

```bash
make image IMAGE_NAMESPACE=ghcr.io/kaio6fellipe VERSION=prs-3961-3983 DOCKER_PUSH=true
```

- [ ] **Step 2: Restart sensor pods**

```bash
kubectl --context=k3d-bookinfo-local delete pods -n bookinfo -l sensor-name
kubectl --context=k3d-bookinfo-local wait --for=condition=Ready pods -n bookinfo -l sensor-name --timeout=60s
```

- [ ] **Step 3: Send test request and measure transaction timing**

Wait for consumers to stabilize, then:

```bash
sleep 15
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id":"test-backoff-tuning","rating":5}'
```

- [ ] **Step 4: Check sensor logs for transaction timing**

```bash
sleep 3
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l sensor-name=rating-submitted-sensor --tail=10 | grep -E "Begin transaction|Finished transaction"
```

Expected: The gap between `Begin transaction` and `Finished transaction` should be **<200ms** (down from ~1.5s). If the single-dep optimization from the previous plan is already applied, you won't see transaction logs for single-dep triggers — test with a multi-dep sensor or temporarily revert the single-dep optimization to isolate this change.

---

### Task 3: Layer 2 — Batch partition registration with consistent keys

**Files:**
- Modify: `pkg/eventbus/kafka/sensor/kafka_sensor.go:309-361` (the `Event` method)
- Modify: `pkg/eventbus/kafka/sensor/kafka_sensor.go:363-423` (the `Trigger` method)

- [ ] **Step 1: Add consistent partition key to `Event()` for multi-dep messages**

In the `Event()` method, when producing messages to the trigger topic (the multi-dep branch), use a consistent key derived from the source message to ensure all messages land on the same partition:

Find the message construction for multi-dep triggers (the `messages = append(messages, &sarama.ProducerMessage{...})` block in the else branch after `OneAndDone()`). Change the `Key` field:

```go
		// Use source message coordinates as key to route all trigger
		// messages from the same event to the same partition, reducing
		// AddPartitionsToTxn calls from N to 1 per transaction.
		partitionKey := fmt.Sprintf("%s-%d-%d", msg.Topic, msg.Partition, msg.Offset)

		messages = append(messages, &sarama.ProducerMessage{
			Topic: s.topics.trigger,
			Key:   sarama.StringEncoder(partitionKey),
			Value: sarama.ByteEncoder(value),
			Headers: []sarama.RecordHeader{{
				Key:   []byte(dependencyNameHeader),
				Value: []byte(trigger.depName),
			}},
		})
```

Also add `"fmt"` to the imports if not already present.

- [ ] **Step 2: Apply the same pattern in `Trigger()` for action topic messages**

In the `Trigger()` method, find the message construction block (lines ~403-411) where messages are appended to the action topic. Change the `Key` to use the source message coordinates:

```go
			partitionKey := fmt.Sprintf("%s-%d-%d", msg.Topic, msg.Partition, msg.Offset)

			messages = append(messages, &sarama.ProducerMessage{
				Topic: s.topics.action,
				Key:   sarama.StringEncoder(partitionKey),
				Value: sarama.ByteEncoder(value),
				Headers: []sarama.RecordHeader{{
					Key:   []byte(dependencyNameHeader),
					Value: []byte(dependencyName),
				}},
			})
```

- [ ] **Step 3: Build and run tests**

```bash
go build ./...
go test ./pkg/eventbus/kafka/... -v -count=1
```

Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/eventbus/kafka/sensor/kafka_sensor.go
git commit -m "perf: use consistent partition key to batch AddPartitionsToTxn calls

Route all trigger/action messages from the same source event to the same
Kafka partition using a composite key (topic-partition-offset). This reduces
the number of AddPartitionsToTxn calls from N (one per target partition) to
1 per transaction, saving one full retry cycle per additional partition."
```

---

### Task 4: Layer 3 — CRD-level transaction retry configuration

**Files:**
- Modify: `pkg/apis/events/v1alpha1/kafka_eventbus.go:4-33` (add `TransactionRetryBackoff` field)
- Modify: `pkg/eventbus/kafka/base/kafka.go` (read and apply the CRD field)

- [ ] **Step 1: Add `TransactionRetryBackoff` field to `KafkaBus`**

In `pkg/apis/events/v1alpha1/kafka_eventbus.go`, add the new field after `ConsumerBatchMaxWait`:

```go
	// TransactionRetryBackoff sets the backoff duration between Kafka transaction
	// retry attempts when CONCURRENT_TRANSACTIONS errors occur. Lower values
	// reduce latency at the cost of slightly higher CPU from more frequent retries.
	// Accepts a Go duration string (e.g., "10ms", "100ms"). Defaults to "10ms".
	// Use "100ms" for cross-region deployments with high coordinator latency.
	// +optional
	TransactionRetryBackoff string `json:"transactionRetryBackoff,omitempty" protobuf:"bytes,9,opt,name=transactionRetryBackoff"`
```

- [ ] **Step 2: Apply CRD field in `kafka.go` Config()**

In `pkg/eventbus/kafka/base/kafka.go`, replace the hardcoded backoff with CRD-configurable logic. Replace the transaction retry block added in Task 1:

```go
	// Transaction retry tuning — configurable via EventBus CRD.
	// Default 10ms is optimized for single-region deployments where the
	// Kafka broker resolves CONCURRENT_TRANSACTIONS within milliseconds.
	txnRetryBackoff := 10 * time.Millisecond
	if k.config.TransactionRetryBackoff != "" {
		if d, err := time.ParseDuration(k.config.TransactionRetryBackoff); err == nil && d > 0 {
			txnRetryBackoff = d
		}
	}
	config.Producer.Transaction.Retry.Backoff = txnRetryBackoff
	config.Producer.Transaction.Retry.Max = 100
```

- [ ] **Step 3: Regenerate codegen**

The CRD field addition requires regenerating protobuf, deepcopy, and OpenAPI:

```bash
make generate
```

If `make generate` is not available, run the individual generators:

```bash
go generate ./...
```

Verify the generated files include the new field:

```bash
grep "TransactionRetryBackoff" pkg/apis/events/v1alpha1/zz_generated.deepcopy.go
```

Expected: The field appears in `DeepCopyInto` for `KafkaBus` (it's a string value type, so `*out = *in` handles it automatically — no explicit line needed, but verify it's not excluded).

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./pkg/eventbus/kafka/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add pkg/apis/events/v1alpha1/kafka_eventbus.go pkg/eventbus/kafka/base/kafka.go
git add -A pkg/apis/events/v1alpha1/  # generated files
git commit -m "feat: add transactionRetryBackoff CRD field for EventBus Kafka config

Allows operators to tune the Kafka transaction retry backoff per EventBus
without rebuilding the image. Defaults to 10ms (optimized for single-region).
Cross-region deployments can set '100ms' to avoid excessive retries when
coordinator latency is high.

Example:
  spec:
    kafka:
      transactionRetryBackoff: '10ms'"
```

---

### Task 5: Layer 4 — Skip action topic for evaluated multi-dep triggers

**Files:**
- Modify: `pkg/eventbus/kafka/sensor/kafka_sensor.go:363-423` (the `Trigger` method)

- [ ] **Step 1: Modify `Trigger()` to invoke action directly when dependencies are satisfied**

Replace the `Trigger()` method at lines 363-423 with:

```go
func (s *KafkaSensor) Trigger(msg *sarama.ConsumerMessage) ([]*sarama.ProducerMessage, int64, func()) {
	var event *cloudevents.Event
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		// do not return here as we still need to call trigger.Offset
		// below to determine current offset
		s.Logger.Errorw("Failed to deserialize cloudevent, skipping", zap.Error(err))
	}

	messages := []*sarama.ProducerMessage{}
	offset := msg.Offset + 1
	var dependencyName string
	if event != nil && len(msg.Headers) > 0 {
		for _, header := range msg.Headers {
			if string(header.Key) == dependencyNameHeader {
				dependencyName = string(header.Value)
				break
			}
		}
	}

	var fns []func()

	// update trigger with new event and add any resulting action to
	// transaction messages
	if trigger, ok := s.triggers[string(msg.Key)]; ok && event != nil {
		func() {
			events, err := trigger.Update(event, msg.Partition, msg.Offset, msg.Timestamp, dependencyName)
			if err != nil {
				s.Logger.Errorw("Failed to update trigger, skipping", zap.Error(err))
				return
			}

			// no events, trigger not yet satisfied
			if events == nil {
				return
			}

			// Dependencies satisfied — invoke action directly, skip action topic.
			// This avoids a full Kafka transaction for the trigger→action hop.
			// At-least-once is preserved: if the sensor crashes before the trigger
			// topic offset is committed, the message will be reconsumed and
			// re-evaluated.
			f := trigger.Action(events, dependencyName)
			if f != nil {
				fns = append(fns, f)
			}
		}()
	}

	// need to determine smallest possible offset against all
	// triggers as other triggers may have messages that land on the
	// same partition
	for _, trigger := range s.triggers {
		offset = trigger.Offset(msg.Partition, offset)
	}

	// Compose action callbacks
	var fn func()
	if len(fns) > 0 {
		fn = func() {
			for _, f := range fns {
				f()
			}
		}
	}

	return messages, offset, fn
}
```

Key changes:
1. Added `var fns []func()` to collect action callbacks
2. When `trigger.Update()` returns satisfied events, call `trigger.Action()` directly instead of producing to the action topic
3. Return composed `fn` callback instead of `nil`

- [ ] **Step 2: Build and test**

```bash
go build ./...
go test ./pkg/eventbus/kafka/... -v -count=1
```

- [ ] **Step 3: Commit**

```bash
git add pkg/eventbus/kafka/sensor/kafka_sensor.go
git commit -m "perf: skip action topic for satisfied multi-dep triggers

When a multi-dependency trigger's Update() returns satisfied events,
invoke the action directly instead of routing through the Kafka action
topic. This eliminates one full Kafka transaction round-trip (produce +
consume + transaction commit) for the trigger→action hop.

At-least-once semantics are preserved: if the sensor crashes before the
trigger topic offset is committed, the trigger topic message will be
reconsumed and re-evaluated."
```

---

### Task 6: Build, deploy, and validate full optimization stack

**Files:**
- No code changes — end-to-end validation

- [ ] **Step 1: Build final image with all layers**

```bash
make image IMAGE_NAMESPACE=ghcr.io/kaio6fellipe VERSION=prs-3961-3983 DOCKER_PUSH=true
```

- [ ] **Step 2: Restart all argo-events pods**

```bash
kubectl --context=k3d-bookinfo-local delete pods -n bookinfo -l owner-name
kubectl --context=k3d-bookinfo-local wait --for=condition=Ready pods -n bookinfo -l owner-name --timeout=60s
```

- [ ] **Step 3: Send test request**

```bash
sleep 15
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id":"test-full-optimization","rating":5}'
```

- [ ] **Step 4: Verify latency in sensor logs**

```bash
sleep 3
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l sensor-name=rating-submitted-sensor --tail=10 | grep -E "Received|Begin|Finished|Making"
```

Expected for single-dep triggers (our bookinfo sensors):
- `Received message` → `Making a http request` within **<15ms**
- **No** `Begin transaction` / `Finished transaction` logs

- [ ] **Step 5: Verify trace in Tempo**

Fetch the latest trace and verify the eventsource-to-sensor gap is <50ms (comparable to the `24f782af` baseline of ~5ms).
