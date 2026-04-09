# Optimize Sarama Transactional Producer Latency for Multi-Dependency Triggers

## Problem

Multi-dependency triggers (those using `&&` in the dependency expression, e.g. `dep-a && dep-b`) require the full Kafka transactional pipeline: event topic -> trigger topic -> action topic. Each hop involves a Kafka transaction (`BeginTxn` -> `Produce` -> `AddOffsetsToTxn` -> `CommitTxn`).

Instrumented measurements show `CommitTxn` takes ~1.5-2s per transaction due to sarama's `CONCURRENT_TRANSACTIONS` retry mechanism:

```
Produce ack #1 (partition 0):  T+507ms   → publishTxnPartitions retries
Produce ack #2 (partition 2):  T+1,009ms → publishTxnPartitions retries
CommitTxn returns:             T+2,026ms → publishOffsetsToTxn + endTxn retries
```

Each `AddPartitionsToTxn` and `EndTxn` call triggers `CONCURRENT_TRANSACTIONS` errors from the Kafka broker when the previous transaction's markers haven't been fully written. Sarama retries these with **100ms backoff** (default `Producer.Transaction.Retry.Backoff`), accumulating ~500ms per partition registration and ~1,000ms for the commit phase.

### Why This Happens

The sarama transactional producer (v1.43.0) has three retry points, all using the same 100ms backoff:

1. **`publishTxnPartitions()`** — called before each produce batch, sends `AddPartitionsToTxn` to register new partitions with the transaction coordinator. On `CONCURRENT_TRANSACTIONS`, retries with reduced backoff (20ms for the first partition, 100ms for subsequent).

2. **`publishOffsetsToTxn()`** — called during `CommitTxn`, sends `AddOffsetsToTxn` + `TxnOffsetCommit`. Retries with 100ms backoff.

3. **`endTxn()`** — called during `CommitTxn`, sends `EndTxn`. Retries with 100ms backoff on `CONCURRENT_TRANSACTIONS`.

Source: `transaction_manager.go` lines 761-892 (publishTxnPartitions), 271-475 (publishOffsetsToTxn), 619-691 (endTxn).

### Evidence

The reference GKE environment (3 brokers, Kafka 4.1.1) achieves 0.43ms eventsource-to-sensor latency. A single-node k3d environment shows 1.5-2s per transaction. The broker completes individual requests in <50ms (`EventPerformanceMonitor` shows 10ms avg). The 1.5s is entirely client-side retry backoff accumulation.

## Design

A layered approach addressing both the sarama configuration and the argo-events transaction architecture.

### Layer 1: Tune Sarama Transaction Retry Configuration

**File: `pkg/eventbus/kafka/base/kafka.go`**

Expose configurable transaction retry settings through the sarama `Config` object:

```go
// Transaction retry tuning for low-latency real-time processing.
// The Kafka broker typically resolves CONCURRENT_TRANSACTIONS within
// a few milliseconds. The default 100ms backoff is designed for
// cross-datacenter clusters but is excessive for single-region deployments.
config.Producer.Transaction.Retry.Backoff = 10 * time.Millisecond
config.Producer.Transaction.Retry.Max = 100  // more retries at lower backoff
```

**Rationale:** Reducing backoff from 100ms to 10ms means each retry burns 10ms instead of 100ms. A transaction that previously required 5 retries at 100ms (500ms) now takes 5 retries at 10ms (50ms). The `Max` is increased to 100 to compensate for more retries needed if the coordinator is genuinely busy.

**Alternative — Exponential backoff:**

```go
config.Producer.Transaction.Retry.BackoffFunc = func(retries, maxRetries int) time.Duration {
    backoff := time.Duration(1<<uint(retries)) * time.Millisecond  // 1ms, 2ms, 4ms, 8ms...
    if backoff > 100*time.Millisecond {
        return 100 * time.Millisecond
    }
    return backoff
}
```

This starts at 1ms and doubles each retry, capping at 100ms. For most cases the coordinator resolves within the first few retries, keeping total latency under 20ms.

**Expected impact:** Multi-dep transaction from ~1.5s to ~50-150ms per hop.

### Layer 2: Batch Partition Registration

**File: `pkg/eventbus/kafka/sensor/kafka_transaction.go`**

Currently, the argo-events code sends 2 messages (one per trigger) to the action topic. If these messages target different partitions, sarama's `publishTxnPartitions()` is called once per partition, serialized by `MaxOpenRequests=1`. Each call may retry independently.

**Optimization:** Route all messages for the same transaction to the **same partition** using a consistent key, reducing `AddPartitionsToTxn` calls from N to 1:

```go
// In kafka_sensor.go, event topic handler:
// Use a consistent partition key for all messages in the same event processing cycle
partitionKey := fmt.Sprintf("%s-%d", msg.Topic, msg.Partition)
for _, trigger := range triggers {
    messages = append(messages, &sarama.ProducerMessage{
        Topic: topic,
        Key:   sarama.StringEncoder(partitionKey),  // same key → same partition
        Value: sarama.ByteEncoder(value),
    })
}
```

**Expected impact:** Reduces `AddPartitionsToTxn` calls from 2+ to 1, saving one full retry cycle (~500ms with default backoff, ~50ms with tuned backoff).

### Layer 3: CRD-Level Transaction Retry Configuration

**File: `pkg/apis/events/v1alpha1/kafka_eventbus.go`**

Add optional fields to `KafkaBus` for operator-level transaction tuning:

```go
// TransactionRetryBackoff sets the backoff duration between transaction
// retry attempts. Lower values reduce latency when CONCURRENT_TRANSACTIONS
// errors occur, at the cost of slightly higher CPU usage from more frequent
// retries. Default: "10ms". Use "100ms" for cross-region deployments.
// +optional
TransactionRetryBackoff string `json:"transactionRetryBackoff,omitempty" protobuf:"bytes,9,opt,name=transactionRetryBackoff"`
```

**File: `pkg/eventbus/kafka/base/kafka.go`**

Parse and apply the CRD field:

```go
if k.config.TransactionRetryBackoff != "" {
    d, err := time.ParseDuration(k.config.TransactionRetryBackoff)
    if err == nil && d > 0 {
        config.Producer.Transaction.Retry.Backoff = d
    }
}
```

This allows operators to tune transaction retry behavior per EventBus without rebuilding the image.

### Layer 4: Skip Intermediate Topics for Evaluated Triggers

For multi-dep triggers that have already been evaluated (all dependencies satisfied), the current flow produces to the action topic and then consumes from it in a separate `ConsumeClaim` goroutine. This adds a full transaction round-trip.

**Optimization:** When the trigger topic handler evaluates dependencies and all are satisfied, invoke the action directly (same pattern as Layer 1 of the single-dep spec) instead of routing through the action topic:

```go
// In trigger topic handler:
events, err := trigger.Update(event, msg.Partition, msg.Offset, msg.Timestamp, depName)
if len(events) > 0 {
    // Dependencies satisfied — invoke action directly
    f := trigger.Action(events, depName)
    if f != nil {
        fns = append(fns, f)
    }
    // No message produced to action topic
    continue
}
```

This eliminates the action topic hop entirely for multi-dep triggers, leaving only the event → trigger topic transaction (which still benefits from Layers 1-3).

**Trade-off:** The action topic currently serves as a durable record of actions to execute. Skipping it means action execution is not independently retryable from the action topic. However, the trigger topic transaction still provides at-least-once delivery — if the sensor crashes after the trigger topic commit but before action execution, the trigger topic message will be reconsumed and re-evaluated.

## Implementation Priority

| Layer | Impact | Effort | Risk |
|-------|--------|--------|------|
| 1 — Tune retry backoff | High (3-10x improvement) | Low (1 line change) | Low |
| 2 — Batch partition registration | Medium (saves 1 retry cycle) | Low (key change) | Low |
| 3 — CRD configuration | Medium (operator flexibility) | Medium (CRD + codegen) | Low |
| 4 — Skip action topic | High (eliminates 1 full hop) | Medium (handler refactor) | Medium (changes action delivery semantics) |

**Recommended order:** Layer 1 first (immediate, low-risk). Validate improvement. Then Layer 4 if further reduction is needed. Layers 2 and 3 as follow-ups.

## Testing

1. **Layer 1:** Measure transaction timing with `Retry.Backoff=10ms` vs 100ms on single-node and multi-node Kafka
2. **Layer 2:** Verify all action messages land on the same partition; measure `AddPartitionsToTxn` call count
3. **Layer 3:** Apply `transactionRetryBackoff: "10ms"` to EventBus CRD; verify sensor reads and applies it
4. **Layer 4:** Multi-dep trigger with 2+ dependencies; verify action fires after all deps satisfied; verify crash recovery re-evaluates from trigger topic

## Expected End State

| Scenario | Before | After (Layer 1+4) |
|----------|--------|-------------------|
| Single-dep trigger | ~1,500ms | <15ms (covered by single-dep spec) |
| Multi-dep trigger (2 deps) | ~3,000ms (2 hops × 1.5s) | <100ms (1 hop × 50ms, direct action) |
| Multi-dep trigger (worst case) | ~4,500ms (3 hops × 1.5s) | <150ms (1 hop × tuned backoff + direct action) |
