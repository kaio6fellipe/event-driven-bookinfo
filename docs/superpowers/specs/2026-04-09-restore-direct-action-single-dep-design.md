# Restore Direct Action Invocation for Single-Dependency Triggers

## Problem

PR #3983 (`feat: configurable consumer batch timeout for Kafka EventBus sensors`) introduced `consumerBatchMaxWait` to eliminate batch wait latency. However, the refactor also removed an optimization from commit `24f782af` that bypassed the Kafka action topic for single-dependency triggers.

### Evidence

Instrumented transaction timing on a single-node k3d Kafka broker:

| Step | Duration |
|------|----------|
| `BeginTxn` | 0.08ms |
| `Input 2 msgs` | 0.26ms |
| `AddOffsetsToTxn` | 0.07ms |
| **`CommitTxn`** | **2,026ms** |

The `CommitTxn` breakdown shows ~500ms per `AddPartitionsToTxn` call (one per target partition) plus ~1,000ms for `publishOffsetsToTxn` + `endTxn`. Each step triggers sarama's `CONCURRENT_TRANSACTIONS` retry loop at 100ms backoff.

With the `24f782af` optimization, the same flow takes **5ms total** because `transaction.Commit` receives 0 producer messages and hits the early return path (`session.MarkOffset + session.Commit`), avoiding the Kafka transactional producer entirely.

### Root Cause

In `24f782af`, single-dependency triggers invoke the action callback directly:

```go
// kafka_sensor.go — event topic handler
if trigger.OneAndDone() {
    f := trigger.Action([]*cloudevents.Event{event}, trigger.depName)
    if f != nil {
        fns = append(fns, f)
    }
    continue  // no messages produced → transaction.Commit early return
}
```

In `b039f558`, this was replaced with routing through the action topic:

```go
// kafka_sensor.go — event topic handler
if trigger.OneAndDone() {
    data = []*cloudevents.Event{event}
    topic = s.topics.action  // produces to Kafka → full transaction
} else {
    data = event
    topic = s.topics.trigger
}
// ...produces message, returns nil callback
return messages, msg.Offset + 1, nil
```

## Design

Restore the direct action invocation for single-dependency triggers in the `consumeClaimRealtime` path. The optimization applies only when `BatchMaxWait == 0` (real-time mode), preserving the existing batched behavior unchanged.

### Changes

**File: `pkg/eventbus/kafka/sensor/kafka_sensor.go`**

In the event topic handler function (the closure returned by `KafkaSensor.Initialize()` that processes messages from the `argo-events` topic), restore the `OneAndDone()` check:

```go
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
// ...
messages = append(messages, &sarama.ProducerMessage{
    Topic: s.topics.trigger,
    // ...
})
```

Compose the collected `fns` into a single callback returned alongside messages:

```go
var fn func()
if len(fns) > 0 {
    fn = func() {
        for _, f := range fns {
            f()
        }
    }
}
return messages, msg.Offset + 1, fn
```

### Behavior

| Trigger Type | `BatchMaxWait=0` (real-time) | `BatchMaxWait>0` (batched) |
|---|---|---|
| Single-dep | Direct action invocation (no Kafka transaction) | Unchanged (action topic) |
| Multi-dep | Route through trigger topic (Kafka transaction) | Unchanged (trigger topic) |

### `atLeastOnce` Semantics

With direct action invocation, the action executes **before** the offset is committed. If the sensor crashes after executing the action but before committing, the message will be reconsumed and the action will fire again — preserving at-least-once semantics. This is the same behavior as the `atLeastOnce=true` flag on individual triggers.

### Testing

1. **Single-dep trigger**: Verify action fires within <50ms of event receipt (no transaction logs)
2. **Multi-dep trigger**: Verify messages still route through trigger topic with full transaction
3. **Crash recovery**: Kill sensor between action execution and offset commit; verify action fires again on restart
4. **Trace propagation**: Verify distributed trace spans link correctly from eventsource → sensor.trigger → target service

### Expected Latency

| Metric | Before (b039f558) | After |
|--------|-------------------|-------|
| Single-dep event → action | ~1,500-2,000ms | **<15ms** |
| Multi-dep event → trigger topic | ~1,500-2,000ms | ~1,500-2,000ms (unchanged) |

### Scope

- **In scope**: Restore direct action invocation in `kafka_sensor.go`
- **Out of scope**: Sarama transactional producer optimization (covered in a separate spec)
- **Not changed**: `consumeClaimBatched`, `consumeClaimRealtime`, `KafkaTransaction.Commit`, `parseBatchMaxWait`, CRD fields
