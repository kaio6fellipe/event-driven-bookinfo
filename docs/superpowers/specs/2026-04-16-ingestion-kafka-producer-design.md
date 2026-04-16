# Ingestion Kafka Producer & Event-Driven Consumption

**Date:** 2026-04-16
**Status:** Approved

## Overview

Refactor the ingestion service from an HTTP Gateway publisher to a native Kafka producer, and extend the Helm chart with `events.exposed` / `events.consumed` abstractions for declaring Kafka-based event pipelines independent of the existing CQRS pattern.

## Goals

1. Ingestion publishes CloudEvents-compliant messages directly to a dedicated Kafka topic (`raw_books_details`) using a native Go Kafka client
2. Ingestion auto-creates the topic if it doesn't exist
3. The Helm chart creates a Kafka-type EventSource (Argo Events) from `events.exposed` declarations
4. Consuming services declare `events.consumed` to generate a separate Sensor with triggers, retry, and DLQ — independent of the CQRS Sensor
5. Existing CQRS pattern (webhook EventSources, Sensors, HTTPRoutes) remains untouched

## Non-Goals

- Modifying the CQRS webhook EventSource / Sensor pattern
- Adding Kafka consumers in Go code (consumption is handled by Argo Events Sensors)
- Schema registry or Avro serialization (CloudEvents JSON is sufficient)

## Architecture

### Two Coexisting Event Patterns

**CQRS Pattern (existing, unchanged):**
```
HTTP Client → Gateway → Webhook EventSource → EventBus → CQRS Sensor → Write Service
```

**Ingestion/Producer Pattern (new):**
```
External API → Ingestion Service → Kafka Topic → Kafka EventSource → Consumer Sensor → Write Service
```

Both patterns coexist. A service like `details` can have both a CQRS Sensor (from `cqrs.endpoints`) and a Consumer Sensor (from `events.consumed`).

### End-to-End Data Flow

```
OpenLibrary API
    | HTTP GET (poll on interval)
Ingestion Service (Go, franz-go producer)
    | CloudEvents envelope, topic auto-created if missing
Kafka: raw_books_details (dedicated topic, 3 partitions)
    |
Kafka EventSource: ingestion-raw-books-details (Argo Events)
    | publishes to EventBus
Consumer Sensor: details-consumer-sensor (in details Helm release)
    | trigger: ingest-book-detail
Details Write Service: POST /v1/details
    | idempotency check (ingestion-isbn-{ISBN})
PostgreSQL (bookinfo_details)
```

**DLQ path** (on trigger failure after retries exhausted):
```
Consumer Sensor → dlqTrigger → dlq-event-received EventSource → DLQueue Service
```

## Helm Chart Changes

### New Values Schema

```yaml
events:
  kafka:
    broker: ""                            # Kafka bootstrap server address

  exposed:                                # Map of events this service produces
    {}
    # event-name:
    #   topic: kafka_topic_name           # Kafka topic the Go service produces to
    #   eventBusName: kafka               # Argo Events EventBus reference
    #   contentType: application/json     # CloudEvents datacontenttype

  consumed:                               # Map of events this service consumes
    {}
    # event-name:
    #   eventSourceName: producer-event-name  # Cross-release Kafka EventSource name
    #   eventName: event-name                 # Event name within that EventSource
    #   triggers:                             # Same trigger schema as cqrs.endpoints
    #     - name: trigger-name
    #       url: self                         # "self" resolves to write deployment
    #       method: POST
    #       payload:
    #         - passthrough
    #   retryStrategy: {}                     # Optional per-event override; falls back to sensor.retryStrategy defaults
    #   dlq:
    #     enabled: true                       # Falls back to sensor.dlq defaults
    #     url: ""
```

### New Templates

**`kafka-eventsource.yaml`** — one Kafka-type EventSource per `events.exposed` entry:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: {{ fullname }}-{{ eventName }}
spec:
  eventBusName: {{ .eventBusName }}
  kafka:
    {{ eventName }}:
      url: {{ events.kafka.broker }}
      topic: {{ .topic }}
      contentType: {{ .contentType }}
      jsonBody: true
```

**`consumer-sensor.yaml`** — one Sensor if `events.consumed` is non-empty, separate from the CQRS Sensor:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: {{ fullname }}-consumer-sensor
spec:
  eventBusName: kafka
  dependencies:
    - name: {{ eventName }}-dep
      eventSourceName: {{ .eventSourceName }}
      eventName: {{ .eventName }}
  triggers:
    # Same trigger logic as CQRS sensor:
    # - self URL resolution to write deployment
    # - passthrough / structured payload mapping
    # - retry strategy (per-event override or sensor defaults)
    # - DLQ auto-trigger if dlq.enabled
```

### Modified Templates

**`configmap.yaml`** — inject `KAFKA_BROKERS` env var when `events.kafka.broker` is set.

### Unmodified Templates

`eventsource.yaml`, `sensor.yaml`, `deployment.yaml`, `deployment-write.yaml`, `service.yaml`, `service-write.yaml`, `httproute.yaml`, `eventsource-service.yaml` — CQRS pattern untouched.

### CI Test Values

- `ci/values-ingestion-kafka.yaml` — validates `events.exposed` generates Kafka EventSource + ConfigMap with `KAFKA_BROKERS`
- `ci/values-details-consumer.yaml` — validates `events.consumed` generates Consumer Sensor with triggers and DLQ

## Ingestion Service Go Code Changes

### Remove

- `internal/adapter/outbound/gateway/publisher.go` — HTTP gateway publisher adapter
- `GATEWAY_URL` references from `cmd/main.go` wiring

### Add

- `internal/adapter/outbound/kafka/producer.go` — new outbound adapter:
  - Implements `EventPublisher` port (`PublishBookAdded(ctx, book) error`)
  - Uses `franz-go` (`github.com/twmb/franz-go/pkg/kgo`) — pure Go, no CGO
  - Uses `kadm` package for topic auto-creation on startup (3 partitions, RF 1 for local)
  - Uses `cloudevents/sdk-go` for CloudEvents envelope:
    - `type`: `com.bookinfo.ingestion.book-added`
    - `source`: `ingestion`
    - `id`: UUID per message
    - `subject`: ISBN
    - `time`: publish timestamp
    - `datacontenttype`: `application/json`
  - Graceful shutdown: flushes pending messages on context cancellation

### Modify

- `cmd/main.go` — wire Kafka producer instead of gateway publisher; read `KAFKA_BROKERS` and `KAFKA_TOPIC` from env/config
- `pkg/config` — add `KafkaBrokers` (string) and `KafkaTopic` (string) fields, optional

### Unchanged

- `internal/core/port/outbound.go` — `EventPublisher` interface stays the same
- `internal/core/port/inbound.go` — no changes
- `internal/core/service/ingestion_service.go` — no changes (calls `publisher.PublishBookAdded` as before)
- `internal/adapter/outbound/openlibrary/client.go` — no changes
- `internal/adapter/inbound/http/handler.go` — no changes

The hexagonal architecture means only the adapter and composition root change.

## Deploy Values Changes

### `deploy/ingestion/values-local.yaml`

Remove `GATEWAY_URL` from config. Add `events` block:

```yaml
config:
  LOG_LEVEL: "debug"
  POLL_INTERVAL: "5m"
  SEARCH_QUERIES: "programming,golang,kubernetes,python,devops"
  MAX_RESULTS_PER_QUERY: "10"

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  exposed:
    raw-books-details:
      topic: raw_books_details
      eventBusName: kafka
      contentType: application/json
```

### `deploy/details/values-local.yaml`

Add `events` block alongside existing CQRS config (CQRS untouched):

```yaml
events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  consumed:
    raw-books-details:
      eventSourceName: ingestion-raw-books-details
      eventName: raw-books-details
      triggers:
        - name: ingest-book-detail
          url: self
          method: POST
          payload:
            - passthrough
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/v1/events"
```

### Unchanged Deploys

`deploy/ratings/`, `deploy/reviews/`, `deploy/notification/`, `deploy/dlqueue/`, `deploy/productpage/` — no changes.

## Naming Conventions

| Resource | Name Pattern | Example |
|----------|-------------|---------|
| Kafka EventSource | `{fullname}-{eventName}` | `ingestion-raw-books-details` |
| Consumer Sensor | `{fullname}-consumer-sensor` | `details-consumer-sensor` |
| CQRS Sensor (existing) | `{fullname}-sensor` | `details-sensor` |
| Kafka Topic | snake_case, prefixed by domain | `raw_books_details` |
| CloudEvents type | `com.bookinfo.{service}.{action}` | `com.bookinfo.ingestion.book-added` |

## Dependencies

- `github.com/twmb/franz-go` — Kafka client (producer + admin)
- `github.com/twmb/franz-go/pkg/kadm` — topic administration
- `github.com/cloudevents/sdk-go/v2` — CloudEvents SDK

## Testing

- **Unit tests**: Kafka producer adapter with mock Kafka client (franz-go supports test helpers)
- **CI helm-lint**: new CI values files validate template rendering for both `events.exposed` and `events.consumed`
- **E2E (local k8s)**: deploy full stack, verify ingestion → Kafka → EventSource → Consumer Sensor → details-write flow
