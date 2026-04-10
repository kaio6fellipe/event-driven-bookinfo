# Argo Events Messaging Spans Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add proper PRODUCER/CONSUMER spans with OTel messaging semantic conventions to Argo Events so sensors and EventSources appear as connected nodes in Tempo's service graph, including the EventBus messaging edge.

**Architecture:** Extend the existing `pkg/shared/tracing` package with span kind helpers and a messaging attributes builder. Modify `eventsource.publish` to PRODUCER, `sensor.trigger` to CLIENT, add new `sensor.consume` CONSUMER span, and add `eventsource.receive` SERVER span for webhook sources. The EventBus link (`eventsource -> sensor`) uses PRODUCER/CONSUMER edge matching in Tempo via CloudEvent traceparent propagation.

**Tech Stack:** Go, OpenTelemetry Go SDK, Argo Events fork (`ghcr.io/kaio6fellipe/argo-events`)

**Spec:** `docs/superpowers/specs/2026-04-10-argo-events-messaging-spans-design.md`

**Repos involved:**
- **Argo Events fork** (`kaio6fellipe/argo-events`) — all Go code changes (Tasks 1-7), branch `feat/cloudevents-compliance-otel-tracing`
- **go-http-server** — Tempo config update (Task 8), image tag stays `prs-3961-3983`

**Git rules for Argo Events fork:**
- All commits must use `git commit -s` (sign-off with `Signed-off-by:` trailer)
- **No** `Co-Authored-By` trailer
- Only push OTel/CloudEvents-related commits to `feat/cloudevents-compliance-otel-tracing` — no Kafka optimization or unrelated changes
- After pushing, merge both PR branches into `feat/combined-prs-3961-3983` on the fork, run codegen, build and push image as `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`

---

## File Map (Argo Events Fork)

| File | Action | Responsibility |
| --- | --- | --- |
| `pkg/shared/tracing/tracing.go` | Modify | Add span kind helpers: `StartServerSpan`, `StartProducerSpan`, `StartConsumerSpan`, `StartClientSpan` |
| `pkg/shared/tracing/messaging.go` | Create | `MessagingAttributes()` builder and `SourceTypeSpanKind()` classifier |
| `pkg/shared/tracing/tracing_test.go` | Create | Tests for span kind helpers |
| `pkg/shared/tracing/messaging_test.go` | Create | Tests for messaging attributes and source type classification |
| `pkg/eventsources/eventing.go` | Modify | Change `eventsource.publish` from INTERNAL to PRODUCER with messaging attributes |
| `pkg/sensors/listener.go` | Modify | Add `sensor.consume` CONSUMER span; change `sensor.trigger` to CLIENT |
| `eventsources/sources/webhook/start.go` | Modify | Add `eventsource.receive` SERVER span for webhook sources |

## File Map (go-http-server)

| File | Action | Responsibility |
| --- | --- | --- |
| `deploy/observability/local/tempo-values.yaml` | Modify | Enable `enable_messaging_system_latency_histogram`, add `messaging.system` to dimensions |

---

## Phase 1: Core Changes (All Source Types Benefit)

### Task 1: Add Span Kind Helper Functions

**Repo:** Argo Events fork
**Files:**
- Modify: `pkg/shared/tracing/tracing.go`

These helpers wrap `tracer.Start()` with the correct `trace.WithSpanKind()` option. Each returns `(context.Context, trace.Span)` matching the standard OTel Go SDK pattern.

- [ ] **Step 1: Write tests for span kind helpers**

Create `pkg/shared/tracing/tracing_test.go`:

```go
package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func setupTestTracer(t *testing.T) (oteltrace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp.Tracer("test"), exporter
}

func TestStartServerSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)

	ctx, span := StartServerSpan(context.Background(), tracer, "eventsource.receive")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].SpanKind != oteltrace.SpanKindServer {
		t.Errorf("expected SpanKindServer, got %v", spans[0].SpanKind)
	}
	if spans[0].Name != "eventsource.receive" {
		t.Errorf("expected name eventsource.receive, got %s", spans[0].Name)
	}
	_ = ctx
}

func TestStartProducerSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)

	ctx, span := StartProducerSpan(context.Background(), tracer, "eventsource.publish")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].SpanKind != oteltrace.SpanKindProducer {
		t.Errorf("expected SpanKindProducer, got %v", spans[0].SpanKind)
	}
	_ = ctx
}

func TestStartConsumerSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)

	ctx, span := StartConsumerSpan(context.Background(), tracer, "sensor.consume")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].SpanKind != oteltrace.SpanKindConsumer {
		t.Errorf("expected SpanKindConsumer, got %v", spans[0].SpanKind)
	}
	_ = ctx
}

func TestStartClientSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)

	ctx, span := StartClientSpan(context.Background(), tracer, "sensor.trigger")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].SpanKind != oteltrace.SpanKindClient {
		t.Errorf("expected SpanKindClient, got %v", spans[0].SpanKind)
	}
	_ = ctx
}

func TestStartSpanWithAttributes(t *testing.T) {
	tracer, exporter := setupTestTracer(t)

	attrs := MessagingAttributes("kafka", "argo-events", "sensor-group", "kafka:9092")
	_, span := StartProducerSpan(context.Background(), tracer, "eventsource.publish", attrs...)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrMap := make(map[string]string)
	for _, a := range spans[0].Attributes {
		attrMap[string(a.Key)] = a.Value.AsString()
	}
	if attrMap["messaging.system"] != "kafka" {
		t.Errorf("expected messaging.system=kafka, got %s", attrMap["messaging.system"])
	}
	if attrMap["messaging.destination.name"] != "argo-events" {
		t.Errorf("expected messaging.destination.name=argo-events, got %s", attrMap["messaging.destination.name"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd pkg/shared/tracing && go test -v -run "TestStart|TestStartSpan"
```

Expected: Compilation errors — `StartServerSpan`, `StartProducerSpan`, `StartConsumerSpan`, `StartClientSpan`, `MessagingAttributes` are undefined.

- [ ] **Step 3: Implement span kind helpers**

Add to `pkg/shared/tracing/tracing.go`:

```go
import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StartServerSpan starts a new span with SpanKindServer.
// Used by HTTP webhook EventSource handlers for inbound requests.
func StartServerSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
	)
}

// StartProducerSpan starts a new span with SpanKindProducer.
// Used by EventSource publish to EventBus.
func StartProducerSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attrs...),
	)
}

// StartConsumerSpan starts a new span with SpanKindConsumer.
// Used by Sensor consume from EventBus and subscriber EventSources.
func StartConsumerSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attrs...),
	)
}

// StartClientSpan starts a new span with SpanKindClient.
// Used by Sensor HTTP trigger and poller EventSources.
func StartClientSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd pkg/shared/tracing && go test -v -run "TestStart"
```

Expected: All `TestStartServerSpan`, `TestStartProducerSpan`, `TestStartConsumerSpan`, `TestStartClientSpan` pass. `TestStartSpanWithAttributes` still fails (MessagingAttributes not yet defined).

- [ ] **Step 5: Commit**

```bash
git add pkg/shared/tracing/tracing.go pkg/shared/tracing/tracing_test.go
git commit -s -m "feat(tracing): add span kind helper functions for SERVER/PRODUCER/CONSUMER/CLIENT"
```

---

### Task 2: Add Messaging Attributes Builder and Source Type Classifier

**Repo:** Argo Events fork
**Files:**
- Create: `pkg/shared/tracing/messaging.go`
- Create: `pkg/shared/tracing/messaging_test.go`

- [ ] **Step 1: Write tests for MessagingAttributes and SourceTypeSpanKind**

Create `pkg/shared/tracing/messaging_test.go`:

```go
package tracing

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestMessagingAttributes(t *testing.T) {
	tests := []struct {
		name          string
		busType       string
		destination   string
		consumerGroup string
		serverAddr    string
		wantSystem    string
		wantDest      string
	}{
		{
			name:          "kafka eventbus",
			busType:       "kafka",
			destination:   "argo-events",
			consumerGroup: "sensor-group",
			serverAddr:    "kafka-bootstrap:9092",
			wantSystem:    "kafka",
			wantDest:      "argo-events",
		},
		{
			name:          "jetstream eventbus",
			busType:       "jetstream",
			destination:   "default",
			consumerGroup: "sensor-durable",
			serverAddr:    "nats:4222",
			wantSystem:    "nats",
			wantDest:      "default",
		},
		{
			name:          "stan eventbus",
			busType:       "stan",
			destination:   "default",
			consumerGroup: "sensor-queue",
			serverAddr:    "nats:4222",
			wantSystem:    "nats",
			wantDest:      "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := MessagingAttributes(tt.busType, tt.destination, tt.consumerGroup, tt.serverAddr)

			attrMap := make(map[attribute.Key]string)
			for _, a := range attrs {
				attrMap[a.Key] = a.Value.AsString()
			}

			if got := attrMap["messaging.system"]; got != tt.wantSystem {
				t.Errorf("messaging.system = %q, want %q", got, tt.wantSystem)
			}
			if got := attrMap["messaging.destination.name"]; got != tt.wantDest {
				t.Errorf("messaging.destination.name = %q, want %q", got, tt.wantDest)
			}
			if _, ok := attrMap["server.address"]; !ok {
				t.Error("missing server.address attribute")
			}
		})
	}
}

func TestMessagingAttributes_EmptyConsumerGroup(t *testing.T) {
	attrs := MessagingAttributes("kafka", "topic", "", "broker:9092")

	for _, a := range attrs {
		if a.Key == "messaging.consumer.group.name" {
			t.Error("should not include messaging.consumer.group.name when empty")
		}
	}
}

func TestSourceTypeSpanKind(t *testing.T) {
	tests := []struct {
		sourceType string
		want       oteltrace.SpanKind
	}{
		// HTTP webhook receivers -> SERVER
		{"webhook", oteltrace.SpanKindServer},
		{"github", oteltrace.SpanKindServer},
		{"gitlab", oteltrace.SpanKindServer},
		{"bitbucket", oteltrace.SpanKindServer},
		{"bitbucketserver", oteltrace.SpanKindServer},
		{"slack", oteltrace.SpanKindServer},
		{"stripe", oteltrace.SpanKindServer},
		{"storagegrid", oteltrace.SpanKindServer},
		{"sns", oteltrace.SpanKindServer},
		{"generic", oteltrace.SpanKindServer},

		// Message subscribers -> CONSUMER
		{"kafka", oteltrace.SpanKindConsumer},
		{"amqp", oteltrace.SpanKindConsumer},
		{"nats", oteltrace.SpanKindConsumer},
		{"nsq", oteltrace.SpanKindConsumer},
		{"mqtt", oteltrace.SpanKindConsumer},
		{"gcppubsub", oteltrace.SpanKindConsumer},
		{"redis", oteltrace.SpanKindConsumer},
		{"redisStream", oteltrace.SpanKindConsumer},
		{"sqs", oteltrace.SpanKindConsumer},
		{"azureEventsHub", oteltrace.SpanKindConsumer},
		{"azureQueueStorage", oteltrace.SpanKindConsumer},
		{"azureServiceBus", oteltrace.SpanKindConsumer},
		{"pulsar", oteltrace.SpanKindConsumer},
		{"emitter", oteltrace.SpanKindConsumer},
		{"minio", oteltrace.SpanKindConsumer},

		// Pollers/watchers -> CLIENT
		{"gerrit", oteltrace.SpanKindClient},
		{"sftp", oteltrace.SpanKindClient},
		{"hdfs", oteltrace.SpanKindClient},
		{"resource", oteltrace.SpanKindClient},

		// Local/scheduled -> INTERNAL
		{"calendar", oteltrace.SpanKindInternal},
		{"file", oteltrace.SpanKindInternal},

		// Unknown -> INTERNAL (safe default)
		{"unknown_source", oteltrace.SpanKindInternal},
	}

	for _, tt := range tests {
		t.Run(tt.sourceType, func(t *testing.T) {
			got := SourceTypeSpanKind(tt.sourceType)
			if got != tt.want {
				t.Errorf("SourceTypeSpanKind(%q) = %v, want %v", tt.sourceType, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd pkg/shared/tracing && go test -v -run "TestMessaging|TestSourceType"
```

Expected: Compilation errors — `MessagingAttributes` and `SourceTypeSpanKind` are undefined.

- [ ] **Step 3: Implement MessagingAttributes and SourceTypeSpanKind**

Create `pkg/shared/tracing/messaging.go`:

```go
package tracing

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// MessagingAttributes returns OTel semantic convention attributes for messaging spans.
// busType is the EventBus or external messaging backend type (e.g., "kafka", "jetstream", "stan").
// destination is the topic/subject/channel name.
// consumerGroup is the consumer group identifier (omitted if empty).
// serverAddr is the broker/server address.
func MessagingAttributes(busType, destination, consumerGroup, serverAddr string) []attribute.KeyValue {
	system := busType
	switch busType {
	case "jetstream", "stan":
		system = "nats"
	}

	attrs := []attribute.KeyValue{
		attribute.String("messaging.system", system),
		attribute.String("messaging.destination.name", destination),
		attribute.String("server.address", serverAddr),
	}

	if consumerGroup != "" {
		attrs = append(attrs, attribute.String("messaging.consumer.group.name", consumerGroup))
	}

	return attrs
}

// SourceTypeSpanKind maps an Argo Events EventSource type to the correct inbound
// span kind based on how it receives events from its external source.
func SourceTypeSpanKind(sourceType string) trace.SpanKind {
	switch sourceType {
	// HTTP webhook receivers -> SERVER
	case "webhook", "github", "gitlab", "bitbucket", "bitbucketserver",
		"slack", "stripe", "storagegrid", "sns", "generic":
		return trace.SpanKindServer

	// Message/event subscribers -> CONSUMER
	case "kafka", "amqp", "nats", "nsq", "mqtt", "gcppubsub",
		"redis", "redisStream", "sqs", "azureEventsHub",
		"azureQueueStorage", "azureServiceBus", "pulsar",
		"emitter", "minio":
		return trace.SpanKindConsumer

	// Pollers/watchers -> CLIENT
	case "gerrit", "sftp", "hdfs", "resource":
		return trace.SpanKindClient

	// Local/scheduled and unknown -> INTERNAL
	default:
		return trace.SpanKindInternal
	}
}
```

- [ ] **Step 4: Run all tracing package tests**

```bash
cd pkg/shared/tracing && go test -v ./...
```

Expected: All tests pass, including `TestStartSpanWithAttributes` from Task 1 (which uses `MessagingAttributes`).

- [ ] **Step 5: Commit**

```bash
git add pkg/shared/tracing/messaging.go pkg/shared/tracing/messaging_test.go
git commit -s -m "feat(tracing): add messaging attributes builder and source type span kind classifier"
```

---

### Task 3: Change `eventsource.publish` to PRODUCER

**Repo:** Argo Events fork
**Files:**
- Modify: `pkg/eventsources/eventing.go`

The `eventsource.publish` span is created in the common publish loop in `eventing.go`. All 32 EventSource types flow through this code path. The change is to replace the bare `tracer.Start(ctx, "eventsource.publish")` call with `StartProducerSpan` and add messaging attributes.

- [ ] **Step 1: Locate the current span creation in `eventing.go`**

Search for `eventsource.publish` in `pkg/eventsources/eventing.go`. The current code looks like:

```go
ctx, span := tracer.Start(ctx, "eventsource.publish",
    trace.WithAttributes(
        attribute.String("eventsource.name", ...),
        attribute.String("eventsource.type", ...),
        attribute.String("event.name", ...),
        attribute.String("event.id", ...),
    ),
)
defer span.End()
```

Note the surrounding context to understand where `BusConfig` is accessible for extracting the EventBus type, destination, and server address.

- [ ] **Step 2: Replace with `StartProducerSpan` and add messaging attributes**

Change the span creation to use the new helper. The EventBus type is available from the EventSource adaptor's bus config. Build the messaging attributes from it:

```go
// Determine EventBus type and destination for messaging attributes
busType, busDestination, busAddr := extractBusInfo(e.busConfig)

msgAttrs := tracing.MessagingAttributes(busType, busDestination, "", busAddr)
spanAttrs := append(msgAttrs,
    attribute.String("eventsource.name", eventSourceName),
    attribute.String("eventsource.type", eventSourceType),
    attribute.String("event.name", eventName),
    attribute.String("event.id", eventID),
    attribute.String("messaging.operation.type", "send"),
    attribute.String("messaging.operation.name", "send"),
)

ctx, span := tracing.StartProducerSpan(ctx, tracer, "eventsource.publish", spanAttrs...)
defer span.End()
```

The `extractBusInfo` helper extracts the bus type, destination topic/subject, and server address from the `BusConfig` struct:

```go
func extractBusInfo(bc *eventbuscommon.BusConfig) (busType, destination, serverAddr string) {
    switch {
    case bc.Kafka != nil:
        return "kafka", bc.Kafka.Topic, bc.Kafka.URL
    case bc.JetStream != nil:
        return "jetstream", bc.JetStream.StreamConfig.Subjects[0], bc.JetStream.URL
    case bc.NATS != nil:
        return "stan", bc.NATS.Subject, bc.NATS.URL
    default:
        return "unknown", "", ""
    }
}
```

Adjust field names based on the actual `BusConfig` struct fields in the fork. Examine `pkg/eventbus/common/interface.go` and related files for the exact struct definition.

- [ ] **Step 3: Run the existing eventsource tests**

```bash
go test -v ./pkg/eventsources/...
```

Expected: All existing tests pass. The span kind change doesn't affect test assertions since PR #3961's tests don't assert on span kind.

- [ ] **Step 4: Commit**

```bash
git add pkg/eventsources/eventing.go
git commit -s -m "feat(eventsource): change eventsource.publish span to PRODUCER with messaging attributes"
```

---

### Task 4: Add `sensor.consume` CONSUMER Span

**Repo:** Argo Events fork
**Files:**
- Modify: `pkg/sensors/listener.go`

Add a new CONSUMER span between trace context extraction and trigger execution. This span's parent comes from the CloudEvent traceparent (pointing to `eventsource.publish`), creating the PRODUCER/CONSUMER edge in Tempo's service graph.

- [ ] **Step 1: Locate the trace extraction and trigger call in `listener.go`**

Search for `extractTraceFromEvents` and `triggerOne` in `pkg/sensors/listener.go`. The current flow is:

```go
ctx = extractTraceFromEvents(ctx, events)
// ... directly into triggerOne() which creates sensor.trigger span
```

- [ ] **Step 2: Insert `sensor.consume` CONSUMER span between extraction and trigger**

After `extractTraceFromEvents` returns the context with the remote span context, create a CONSUMER span. This becomes the parent of `sensor.trigger`:

```go
ctx = extractTraceFromEvents(ctx, events)

// Create CONSUMER span for EventBus message consumption.
// Parent is the remote eventsource.publish PRODUCER span (via CloudEvent traceparent).
// This creates the eventsource -> sensor edge in the service graph.
busType, busDest, busAddr := extractSensorBusInfo(sensorCtx.busConfig)
consumeAttrs := tracing.MessagingAttributes(busType, busDest, consumerGroup, busAddr)
consumeAttrs = append(consumeAttrs,
    attribute.String("sensor.name", sensorName),
    attribute.String("messaging.operation.type", "process"),
    attribute.String("messaging.operation.name", "process"),
)
ctx, consumeSpan := tracing.StartConsumerSpan(ctx, tracer, "sensor.consume", consumeAttrs...)
defer consumeSpan.End()

// triggerOne now creates sensor.trigger as a child of sensor.consume
```

The `extractSensorBusInfo` function mirrors the EventSource-side `extractBusInfo` — it reads the sensor's EventBus config to determine type, destination, consumer group, and server address. Implement it similarly based on the sensor's `BusConfig`.

- [ ] **Step 3: Run existing sensor tests**

```bash
go test -v ./pkg/sensors/...
```

Expected: All existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/sensors/listener.go
git commit -s -m "feat(sensor): add sensor.consume CONSUMER span for EventBus message consumption"
```

---

### Task 5: Change `sensor.trigger` to CLIENT

**Repo:** Argo Events fork
**Files:**
- Modify: `pkg/sensors/listener.go`

- [ ] **Step 1: Locate the current `sensor.trigger` span creation in `listener.go`**

Search for `sensor.trigger` in `pkg/sensors/listener.go`. The current code in `triggerOne()`:

```go
ctx, span := tracer.Start(ctx, "sensor.trigger",
    trace.WithAttributes(
        attribute.String("sensor.name", ...),
        attribute.String("trigger.name", ...),
        attribute.StringSlice("dependencies", ...),
        attribute.StringSlice("event.ids", ...),
    ),
)
defer span.End()
```

- [ ] **Step 2: Replace with `StartClientSpan` and add `server.address`**

Change the span creation to use `StartClientSpan`. For HTTP triggers, extract the target URL from the trigger template to set `server.address`:

```go
spanAttrs := []attribute.KeyValue{
    attribute.String("sensor.name", sensorName),
    attribute.String("trigger.name", triggerName),
    attribute.StringSlice("dependencies", depNames),
    attribute.StringSlice("event.ids", eventIDs),
}

// Add server.address for HTTP triggers
if trigger.Template.HTTP != nil {
    spanAttrs = append(spanAttrs,
        attribute.String("server.address", trigger.Template.HTTP.URL),
        attribute.String("http.request.method", trigger.Template.HTTP.Method),
    )
}

ctx, span := tracing.StartClientSpan(ctx, tracer, "sensor.trigger", spanAttrs...)
defer span.End()
```

- [ ] **Step 3: Run existing sensor tests**

```bash
go test -v ./pkg/sensors/...
```

Expected: All existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/sensors/listener.go
git commit -s -m "feat(sensor): change sensor.trigger span to CLIENT with server.address attribute"
```

---

## Phase 2: Webhook EventSource `eventsource.receive` SERVER Span

### Task 6: Add `eventsource.receive` SERVER Span to Webhook Source

**Repo:** Argo Events fork
**Files:**
- Modify: `eventsources/sources/webhook/start.go`

The webhook EventSource handles inbound HTTP requests from external systems (gateway, GitHub, etc.). Adding a SERVER span here creates the `gateway -> eventsource` edge.

- [ ] **Step 1: Locate the webhook event handler in `sources/webhook/start.go`**

Search for the HTTP handler function that processes incoming webhook requests. This is where the incoming HTTP request context is first available, before the event enters the common publish loop.

- [ ] **Step 2: Add `eventsource.receive` SERVER span wrapping the handler**

At the top of the handler, before any event processing, create a SERVER span:

```go
ctx, receiveSpan := tracing.StartServerSpan(r.Context(), tracer, "eventsource.receive",
    attribute.String("eventsource.name", eventSourceName),
    attribute.String("eventsource.type", "webhook"),
    attribute.String("http.method", r.Method),
    attribute.String("http.route", r.URL.Path),
)
defer receiveSpan.End()
```

Use this `ctx` for all subsequent operations so that `eventsource.publish` (PRODUCER) becomes a child of `eventsource.receive` (SERVER).

- [ ] **Step 3: Run webhook source tests**

```bash
go test -v ./eventsources/sources/webhook/...
```

Expected: All existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add eventsources/sources/webhook/start.go
git commit -s -m "feat(eventsource/webhook): add eventsource.receive SERVER span for inbound webhooks"
```

---

### Task 7: Push, Merge Branches, Codegen, Build and Push Image

**Repo:** Argo Events fork

This task handles the branch workflow: push tracing commits to the OTel branch, merge both PR branches into the combined branch, run codegen, and build/push the image.

- [ ] **Step 1: Run the full test suite on `feat/cloudevents-compliance-otel-tracing`**

```bash
go test -race -count=1 ./...
```

Expected: All tests pass.

- [ ] **Step 2: Push tracing commits to the OTel branch**

Ensure you're on `feat/cloudevents-compliance-otel-tracing` and push only the OTel/CloudEvents-related commits:

```bash
git log --oneline origin/feat/cloudevents-compliance-otel-tracing..HEAD
```

Review the log — it should only contain commits from Tasks 1-6 (span kind helpers, messaging attributes, PRODUCER/CONSUMER/CLIENT changes, webhook receive span). No Kafka optimization or unrelated changes.

```bash
git push origin feat/cloudevents-compliance-otel-tracing
```

- [ ] **Step 3: Switch to the combined branch and merge both PR branches**

```bash
git checkout feat/combined-prs-3961-3983
git merge feat/cloudevents-compliance-otel-tracing
```

If the PR #3983 branch also needs to be merged (and isn't already in the combined branch):

```bash
git merge <pr-3983-branch-name>
```

Resolve any merge conflicts if they arise.

- [ ] **Step 4: Run codegen**

```bash
PATH="$(go env GOPATH)/bin:$PATH" make codegen
```

If codegen produces changes, commit them:

```bash
git add -A
git diff --cached --quiet || git commit -s -m "chore: run codegen after merging tracing span kind changes"
```

- [ ] **Step 5: Run the full test suite on the combined branch**

```bash
go test -race -count=1 ./...
```

Expected: All tests pass on the combined branch.

- [ ] **Step 6: Build the container image**

Build using the existing image tag (overwrites the previous image):

```bash
docker build -t ghcr.io/kaio6fellipe/argo-events:prs-3961-3983 .
```

Adjust the Dockerfile path and build args based on the fork's build system (check the existing Makefile or Dockerfile).

- [ ] **Step 7: Push the image**

```bash
docker push ghcr.io/kaio6fellipe/argo-events:prs-3961-3983
```

- [ ] **Step 8: Push the combined branch**

```bash
git push origin feat/combined-prs-3961-3983
```

---

## Phase 3: Tempo Config and Deployment (go-http-server repo)

### Task 8: Update Tempo Config and Deploy

**Repo:** go-http-server
**Files:**
- Modify: `deploy/observability/local/tempo-values.yaml`

The image tag stays `prs-3961-3983` (Task 7 overwrites the same tag), so no deploy manifest changes needed — just Tempo config and a rollout restart to pull the updated image.

- [ ] **Step 1: Enable messaging system histogram and add messaging.system dimension**

In `deploy/observability/local/tempo-values.yaml`, update the `service_graphs` block:

```yaml
      service_graphs:
        enable_virtual_node_label: true
        enable_messaging_system_latency_histogram: true
        histogram_buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
        dimensions:
          - http.method
          - k8s.namespace.name
          - k8s.cluster.name
          - messaging.system
```

Note: `enable_virtual_node_label` may already be present from the Alloy transform plan. If so, only add `enable_messaging_system_latency_histogram` and the `messaging.system` dimension.

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/tempo-values.yaml
git commit -m "$(cat <<'EOF'
feat(observability): enable messaging system latency histogram in Tempo service graph

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Redeploy Tempo**

```bash
helm upgrade --install tempo grafana/tempo \
  -n observability \
  --kube-context=k3d-bookinfo-local \
  -f deploy/observability/local/tempo-values.yaml \
  --wait --timeout 120s
```

- [ ] **Step 4: Restart Argo Events pods to pull updated image**

The image tag is the same (`prs-3961-3983`) but the content changed. Force a rollout restart of EventSource and Sensor pods to pull the new image:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo rollout restart deployment -l app.kubernetes.io/part-of=argo-events
```

If the above selector doesn't match, restart the specific deployments:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo delete pods -l managed-by=events-source
kubectl --context=k3d-bookinfo-local -n bookinfo delete pods -l managed-by=sensor-controller
```

Alternatively, redeploy the full app stack which recreates them:

```bash
make k8s-apps
```

- [ ] **Step 6: Remove Alloy transform workaround (if deployed)**

If the Alloy span kind transform from the companion plan was deployed, it can now be removed since Argo Events emits correct span kinds natively. Revert the transform processor block from:
- `deploy/observability/local/alloy-metrics-traces-config.alloy`
- `deploy/observability/local/alloy-metrics-traces-values.yaml`

Rewire the cluster attributes output back to the batch processor and remove the `otelcol.processor.transform "argo_events"` block.

Then redeploy Alloy:

```bash
helm upgrade --install alloy-metrics-traces grafana/alloy \
  -n observability \
  --kube-context=k3d-bookinfo-local \
  -f deploy/observability/local/alloy-metrics-traces-values.yaml \
  --wait --timeout 120s
```

- [ ] **Step 7: Verify end-to-end**

Trigger a POST request:

```bash
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id":"1","rating":5}'
```

Wait ~10 seconds, then verify in Grafana Tempo:

1. **Trace view:** Find the trace and confirm:
   - `eventsource.receive` span with kind=SERVER
   - `eventsource.publish` span with kind=PRODUCER and `messaging.system` attribute
   - `sensor.consume` span with kind=CONSUMER and `messaging.system` attribute
   - `sensor.trigger` span with kind=CLIENT and `server.address` attribute

2. **Service graph:** Confirm three new edges:
   - `gateway -> eventsource` (CLIENT/SERVER)
   - `eventsource -> sensor` (PRODUCER/CONSUMER — the EventBus messaging edge)
   - `sensor -> write-service` (CLIENT/SERVER)

3. **Prometheus metrics:**
   ```promql
   traces_service_graph_request_messaging_system_seconds_count
   ```
   Expected: Histogram data with `messaging_system="kafka"` (or `nats` depending on EventBus type).
