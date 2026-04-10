# Argo Events PRODUCER/CONSUMER Span Instrumentation

## Problem

Argo Events PR #3961 added OpenTelemetry tracing with two spans (`eventsource.publish` and `sensor.trigger`), both using `SPAN_KIND_INTERNAL`. This creates two gaps:

1. **Wrong span kinds:** `sensor.trigger` is an outbound HTTP call (should be CLIENT), `eventsource.publish` publishes to the EventBus (should be PRODUCER)
2. **Missing inbound span:** There is no span representing the EventSource receiving an event from its external source (HTTP webhook, message subscription, etc.). The correct span kind for this depends on the EventSource type.
3. **Missing messaging spans:** There are no PRODUCER or CONSUMER spans for the EventBus communication. The EventSource publishes to the EventBus and the Sensor consumes from it, but neither operation has a span with the correct messaging span kind or OTel messaging semantic conventions.

Without PRODUCER/CONSUMER spans, the `eventsource -> [EventBus] -> sensor` edge is invisible in Tempo's service graph, even with `enable_messaging_system_latency_histogram` enabled.

## Scope

Code changes to the Argo Events fork (`ghcr.io/kaio6fellipe/argo-events`, branch based on PRs #3961 + #3983). The changes must be **backend-agnostic** — covering all three supported EventBus types and being extensible to the 32+ EventSource types and 11 trigger types.

### EventBus Types

| Backend | Config field | `messaging.system` value | Notes |
|---|---|---|---|
| Kafka | `.kafka` | `kafka` | External broker, active-active sensor HA |
| JetStream | `.jetStream` | `nats` | Argo-managed, leader election only |
| NATS Streaming (Stan) | `.nats` | `nats` | Deprecated, leader election only |

### Current Trace Flow (PR #3961)

```
Gateway (CLIENT)
  -> webhook EventSource receives HTTP
    -> eventsource.publish (INTERNAL) — starts span from CloudEvent traceparent
      -> InjectTraceIntoCloudEvent() — writes traceparent into CloudEvent extensions
        -> EventBus publish (Kafka/JetStream/Stan) — no span
          -> EventBus consume — no span
            -> sensor extractTraceFromEvents() — extracts traceparent
              -> sensor.trigger (INTERNAL) — child of eventsource.publish
                -> Inject() — writes traceparent into outbound HTTP headers
                  -> Write Service (SERVER)
```

Trace context propagation uses **CloudEvent extensions** (`traceparent`/`tracestate` keys), not EventBus message headers. This is backend-agnostic by design.

## Design

### Target Trace Flow (Webhook EventSource Example)

```text
Gateway (CLIENT)
  -> eventsource.receive (SERVER) — new span, pairs with gateway CLIENT
    -> eventsource.publish (PRODUCER) — changed from INTERNAL, with messaging attributes
      -> [EventBus message with traceparent in CloudEvent extensions]
        -> sensor.consume (CONSUMER) — new span, pairs with eventsource.publish PRODUCER
          -> sensor.trigger (CLIENT) — changed from INTERNAL, pairs with write service SERVER
            -> Write Service (SERVER)
```

### Target Trace Flow (Subscriber EventSource Example — e.g., Kafka, SQS, Pub/Sub)

```text
External System (Kafka topic, SQS queue, Pub/Sub subscription, etc.)
  -> eventsource.receive (CONSUMER) — new span, kind depends on source type
    -> eventsource.publish (PRODUCER) — publishes to EventBus with messaging attributes
      -> [EventBus message with traceparent in CloudEvent extensions]
        -> sensor.consume (CONSUMER) — new span, pairs with eventsource.publish PRODUCER
          -> sensor.trigger (CLIENT) — pairs with write service SERVER
            -> Write Service (SERVER)
```

### EventSource Inbound Classification (All 32 Types)

The `eventsource.receive` span kind depends on how the EventSource receives events from its external source. All 32 EventSource types fall into one of four categories:

**HTTP webhook receivers (external system pushes to EventSource HTTP server) -> `eventsource.receive` as SERVER:**

| EventSource type | External system behavior |
| --- | --- |
| Webhook | Generic HTTP POST receiver |
| GitHub | GitHub sends webhook POST |
| GitLab | GitLab sends webhook POST |
| Bitbucket (Cloud) | Bitbucket sends webhook POST |
| Bitbucket Server | Bitbucket Server sends webhook POST |
| Slack | Slack sends event payload POST |
| Stripe | Stripe sends webhook POST |
| NetApp StorageGrid | StorageGrid sends notification POST |
| AWS SNS | SNS pushes notification to HTTP endpoint |

**gRPC server receivers -> `eventsource.receive` as SERVER:**

| EventSource type | External system behavior |
| --- | --- |
| Generic (gRPC) | External client sends gRPC call |

**Message/event subscribers (EventSource subscribes to external system) -> `eventsource.receive` as CONSUMER:**

| EventSource type | Subscription mechanism |
| --- | --- |
| Kafka | Subscribes to Kafka topic |
| AMQP | Subscribes to AMQP queue/exchange (RabbitMQ, etc.) |
| NATS | Subscribes to NATS subject |
| NSQ | Subscribes to NSQ topic |
| MQTT | Subscribes to MQTT topic |
| GCP Pub/Sub | Subscribes to Pub/Sub subscription |
| Redis | Subscribes to Redis Pub/Sub channel |
| Redis Streams | Reads from Redis Stream (XREAD/XREADGROUP) |
| AWS SQS | Polls SQS queue |
| Azure Events Hub | Subscribes to Azure Event Hub partition |
| Azure Queue Storage | Polls Azure Queue Storage |
| Azure Service Bus | Subscribes to Azure Service Bus topic/queue |
| Pulsar | Subscribes to Pulsar topic |
| Emitter | Subscribes to Emitter channel |
| Alibaba Cloud MNS | Polls MNS queue |
| Minio | Subscribes to bucket notifications via Minio client SDK |

**Pollers/watchers (EventSource connects outbound to watch for changes) -> `eventsource.receive` as CLIENT:**

| EventSource type | Connection pattern |
| --- | --- |
| Gerrit | Outbound SSH connection running `gerrit stream-events` |
| SFTP | Polls SFTP server for new/changed files |
| HDFS | Watches HDFS via client connection |
| Kubernetes Resource | Watches k8s API server via watch API |

**Local/scheduled (no external network source) -> no `eventsource.receive` span:**

| EventSource type | Event generation pattern |
| --- | --- |
| Calendar | Generates events on cron/interval schedule |
| File | Watches local filesystem via fsnotify |

### Span Changes Summary

| Span | Status | Kind | Key attributes |
| --- | --- | --- | --- |
| `eventsource.receive` | **New** | **Varies by source type:** SERVER (webhook/gRPC), CONSUMER (subscribers), CLIENT (pollers), omitted (local) | `eventsource.name`, `eventsource.type`, plus type-specific attributes (e.g., `http.method` for SERVER, `messaging.system` for CONSUMER) |
| `eventsource.publish` | **Modified** | PRODUCER (was INTERNAL) | `messaging.system`, `messaging.destination.name`, `messaging.operation.type: send`, `messaging.operation.name: send` |
| `sensor.consume` | **New** | CONSUMER | `messaging.system`, `messaging.destination.name`, `messaging.operation.type: process`, `messaging.operation.name: process`, `messaging.consumer.group.name` |
| `sensor.trigger` | **Modified** | CLIENT (was INTERNAL) | `server.address`, `http.request.method` (existing trigger attributes preserved) |

### Messaging Attributes by EventBus Type

The `messaging.system` and related attributes must be set based on the EventBus backend in use:

| Attribute | Kafka | JetStream | Stan (deprecated) |
|---|---|---|---|
| `messaging.system` | `kafka` | `nats` | `nats` |
| `messaging.destination.name` | Kafka topic name | JetStream subject | NATS channel |
| `messaging.consumer.group.name` | Consumer group ID | Durable consumer name | Queue group |
| `server.address` | Broker bootstrap address | NATS server URL | NATS server URL |

### Implementation Approach

#### 1. Tracing Helper Additions (`pkg/shared/tracing/`)

Extend the existing `tracing` package with:

- **`StartServerSpan(ctx, tracer, name, attrs...) (context.Context, trace.Span)`** — creates a SERVER span, used by HTTP webhook EventSource receive
- **`StartProducerSpan(ctx, tracer, name, attrs...) (context.Context, trace.Span)`** — creates a PRODUCER span with messaging attributes
- **`StartConsumerSpan(ctx, tracer, name, attrs...) (context.Context, trace.Span)`** — creates a CONSUMER span with messaging attributes
- **`StartClientSpan(ctx, tracer, name, attrs...) (context.Context, trace.Span)`** — creates a CLIENT span, used by sensor HTTP trigger and poller EventSources
- **`MessagingAttributes(busType, destination, consumerGroup, serverAddr string) []attribute.KeyValue`** — returns the standard OTel messaging semantic convention attributes based on EventBus type or external source type
- **`SourceTypeSpanKind(sourceType string) trace.SpanKind`** — maps an EventSource type string to the correct inbound span kind (SERVER for webhook types, CONSUMER for subscriber types, CLIENT for poller types, INTERNAL for local types). Based on the classification table. Used by source handlers to determine which `Start*Span` helper to call.

This keeps the span kind and attribute logic centralized and testable, with each EventBus backend and source type only needing to pass its type identifier and connection details.

#### 2. EventSource Changes (`pkg/eventsources/`)

**`eventsource.receive` (new span, kind varies by source type):**

Each EventSource type must create an `eventsource.receive` span with the appropriate kind based on its inbound pattern. The span is created in the individual source handler (e.g., `sources/webhook/start.go`, `sources/kafka/start.go`) before the event enters the common `eventing.go` publish loop.

The `SpanKindForSourceType(sourceType string) trace.SpanKind` helper function maps EventSource types to the correct span kind using the classification table above. Each source handler calls the appropriate `Start*Span` helper:

- **Webhook/gRPC sources** (`sources/webhook/`, `sources/generic/`): `StartServerSpan(ctx, tracer, "eventsource.receive", ...)` — creates the `gateway -> eventsource` or `client -> eventsource` edge
- **Subscriber sources** (`sources/kafka/`, `sources/amqp/`, `sources/nats/`, etc.): `StartConsumerSpan(ctx, tracer, "eventsource.receive", ...)` with external messaging attributes (`messaging.system` matching the external source, e.g., `kafka` for a Kafka EventSource, distinct from the EventBus `messaging.system`)
- **Poller sources** (`sources/sftp/`, `sources/hdfs/`, `sources/gerrit/`, `sources/resource/`): `StartClientSpan(ctx, tracer, "eventsource.receive", ...)` — outbound connection to external system
- **Local sources** (`sources/calendar/`, `sources/file/`): No `eventsource.receive` span — the event is locally generated, no external system to pair with

Since PR #3961 already passes `ctx` from each source handler into the common publish loop, the `eventsource.receive` span context naturally becomes the parent of `eventsource.publish`.

**`eventsource.publish` (change to PRODUCER):**

In `eventing.go`, the existing `eventsource.publish` span creation changes from:
- `tracer.Start(ctx, "eventsource.publish")` (INTERNAL)
- to `StartProducerSpan(ctx, tracer, "eventsource.publish", MessagingAttributes(...))` (PRODUCER)

The EventBus type and destination details are available from the `EventSourceAdaptor` configuration. The `BusConfig` struct tells which backend is active (`.kafka`, `.jetStream`, or `.nats`), and the topic/subject name is derivable from the event source name.

**Important distinction:** Subscriber EventSources (e.g., Kafka EventSource) have **two** messaging systems in their trace: the external source they subscribe to (e.g., an external Kafka cluster) and the internal EventBus they publish to (e.g., the Argo Events Kafka EventBus). The `eventsource.receive` CONSUMER span has `messaging.system` attributes for the external source, while the `eventsource.publish` PRODUCER span has `messaging.system` attributes for the EventBus. These are separate spans with separate messaging attributes.

#### 3. Sensor Changes (`pkg/sensors/`)

**`sensor.consume` (new CONSUMER span):**

In `listener.go`, after `extractTraceFromEvents()` extracts the traceparent from CloudEvent extensions and before `triggerOne()` is called, create a CONSUMER span. This span's parent is the remote span context from the CloudEvent, and its span ID becomes the parent of `sensor.trigger`. This creates the `eventsource -> sensor` edge in the service graph.

The messaging attributes come from the sensor's EventBus configuration, available through the `SensorContext` or the EventBus driver.

**`sensor.trigger` (change to CLIENT):**

In `listener.go`, the existing `sensor.trigger` span creation changes from:
- `tracer.Start(ctx, "sensor.trigger")` (INTERNAL)
- to `StartClientSpan(ctx, tracer, "sensor.trigger", attrs...)` (CLIENT)

Add `server.address` attribute extracted from the trigger's HTTP URL configuration.

#### 4. Tempo Service Graph Matching

For the PRODUCER/CONSUMER edge to appear in Tempo's service graph:

- **Edge matching rule:** The consumer's `parent_span_id` must equal the producer's `span_id`. This is already satisfied by the CloudEvent traceparent propagation — the `sensor.consume` span's parent context comes from the `eventsource.publish` span's context via the CloudEvent extensions.
- **Tempo config:** `enable_messaging_system_latency_histogram: true` must be set, and `messaging.system` should be added to `dimensions` for useful label cardinality.

### Implementation Prioritization

PR #3961's `eventsource.publish` span is created in the common `eventing.go` publish loop, which all 32 EventSource types share. The PRODUCER span change and the `sensor.consume`/`sensor.trigger` changes apply to all of them automatically.

The `eventsource.receive` span requires per-source-type work since each source handler has different inbound logic. Recommended implementation order:

1. **Phase 1 (core):** Common changes — `eventsource.publish` PRODUCER, `sensor.consume` CONSUMER, `sensor.trigger` CLIENT, tracing helpers. This gives the `eventsource -> [EventBus] -> sensor -> write-service` edges for all source types.
2. **Phase 2 (webhook sources):** `eventsource.receive` SERVER for webhook, GitHub, GitLab, Bitbucket, Slack, Stripe, SNS, StorageGrid, Generic. This adds the `external -> eventsource` edge for HTTP-based sources.
3. **Phase 3 (subscriber sources):** `eventsource.receive` CONSUMER for Kafka, AMQP, NATS, MQTT, SQS, Pub/Sub, Redis, Azure, Pulsar, etc. This adds the `external-system -> eventsource` edge for subscription sources.
4. **Phase 4 (poller/watcher sources):** `eventsource.receive` CLIENT for Gerrit, SFTP, HDFS, Kubernetes Resource.

**For this project (bookinfo):** Only Phase 1 and Phase 2 (webhook only) are needed since all EventSources use the webhook type.

### Considerations for Non-HTTP Sensor Triggers

The `sensor.trigger` CLIENT span change applies to **HTTP triggers** (used in this project). For other trigger types:

- **HTTP trigger**: CLIENT span (outbound HTTP call) — addressed in this design
- **Kafka trigger**: PRODUCER span (publishing to external Kafka topic)
- **Argo Workflow trigger**: CLIENT span (API call to Argo server)
- **AWS Lambda trigger**: CLIENT span (API call to AWS)
- **Kubernetes Object trigger**: CLIENT span (API call to k8s)
- **Log trigger**: INTERNAL span (local I/O, no network call)

**Recommendation:** Start with HTTP trigger CLIENT span. Other trigger types can be addressed incrementally using the same `StartClientSpan`/`StartProducerSpan` helpers.

## Resulting Service Graph Edges

With all changes applied and Tempo configured with `enable_messaging_system_latency_histogram: true`:

| Client/Producer | Server/Consumer | Edge type | Created by |
|---|---|---|---|
| `default-gw` (Gateway) | `*-eventsource` | CLIENT/SERVER | Gateway CLIENT + eventsource.receive SERVER |
| `*-eventsource` | `*-sensor` | PRODUCER/CONSUMER | eventsource.publish PRODUCER + sensor.consume CONSUMER |
| `*-sensor` | `*-write` services | CLIENT/SERVER | sensor.trigger CLIENT + write service SERVER |
| `productpage` | `details-read`, `reviews-read` | CLIENT/SERVER | Existing (unchanged) |
| `reviews-read` | `ratings-read` | CLIENT/SERVER | Existing (unchanged) |

The full write-side flow becomes visible: `gateway -> eventsource -> [EventBus] -> sensor -> write-service`.

## Impact on Alloy Transform Workaround

Once these changes are deployed, the Alloy `otelcol.processor.transform "argo_events"` workaround becomes unnecessary:

- `eventsource.publish` will be PRODUCER (not INTERNAL), so the `set(span.kind.string, "Server")` condition won't match
- `sensor.trigger` will be CLIENT (not INTERNAL), so the `set(span.kind.string, "Client")` condition won't match

The transform can be removed after verifying the Argo Events changes are producing correct service graph edges.

## Verification

1. Deploy the updated Argo Events image to the local k8s cluster
2. Enable `enable_messaging_system_latency_histogram: true` in Tempo config
3. Add `messaging.system` to Tempo service graph `dimensions`
4. Trigger POST requests through the gateway for each event type (details, reviews, ratings)
5. Verify in Tempo trace view:
   - `eventsource.receive` span with kind=SERVER (for webhook sources) or kind=CONSUMER (for subscriber sources)
   - `eventsource.publish` span with kind=PRODUCER and messaging attributes
   - `sensor.consume` span with kind=CONSUMER and messaging attributes
   - `sensor.trigger` span with kind=CLIENT
6. Verify in Grafana service graph:
   - `gateway -> eventsource` edge visible
   - `eventsource -> sensor` edge visible (messaging system edge)
   - `sensor -> write-service` edge visible
7. Query `traces_service_graph_request_messaging_system_seconds` in Prometheus — confirm histogram data exists with `messaging.system` labels
