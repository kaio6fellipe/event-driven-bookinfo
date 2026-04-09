# Argo Events Upgrade: OTel Tracing + Kafka Real-Time Processing

## Overview

Upgrade the Argo Events deployment to use a custom image built from combined upstream PRs [#3961](https://github.com/argoproj/argo-events/pull/3961) (OTel distributed tracing) and [#3983](https://github.com/argoproj/argo-events/pull/3983) (configurable Kafka consumer batch timeout). This enables end-to-end distributed tracing across the event pipeline (EventSource -> EventBus -> Sensor -> Trigger) and real-time message processing for Kafka-based sensors.

**Release:** [argo-events-prs-3961-3983](https://github.com/kaio6fellipe/event-driven-bookinfo/releases/tag/argo-events-prs-3961-3983)
**Image:** `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`

## Changes

### 1. Custom Image Override (Helm Values)

**File:** `deploy/platform/local/argo-events-values.yaml`

Add `global.image` block to override the controller, eventsource, and sensor images:

```yaml
global:
  image:
    repository: ghcr.io/kaio6fellipe/argo-events
    tag: "prs-3961-3983"
```

### 2. Custom CRDs (Makefile — Download at Deploy Time)

**File:** `Makefile`

Download and apply the 3 updated CRD YAMLs from the GitHub release **before** the Helm install in the `k8s-platform` target:

```makefile
curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_eventbus.yaml | $(KUBECTL) apply -f -
curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_eventsources.yaml | $(KUBECTL) apply -f -
curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_sensors.yaml | $(KUBECTL) apply -f -
```

CRDs are not committed to the repo — downloaded fresh each deploy for simplicity.

### 3. EventBus — Real-Time Processing

**File:** `deploy/argo-events/overlays/local/eventbus.yaml`

Add `consumerBatchMaxWait: "0"` to disable batching and enable real-time message processing for all sensors:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventBus
metadata:
  name: kafka
  namespace: bookinfo
spec:
  kafka:
    url: bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092
    topic: argo-events
    version: "4.2.0"
    consumerBatchMaxWait: "0"
```

This is an EventBus-level default. Individual sensors can override with `spec.eventBusConsumerBatchMaxWait` if needed.

### 4. EventSources — OTel Env Vars

**Files:** New local overlay copies of all 3 EventSources under `deploy/argo-events/overlays/local/`

Each EventSource gets a `spec.template.container.env` block with OTEL configuration. Since the OTLP endpoint is environment-specific, these are placed in the local overlay (not base).

Service name pattern: `<event-name>-eventsource`

| EventSource | OTEL_SERVICE_NAME |
|---|---|
| book-added | `book-added-eventsource` |
| review-submitted | `review-submitted-eventsource` |
| rating-submitted | `rating-submitted-eventsource` |

Example (book-added):

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: book-added
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "book-added-eventsource"
  webhook:
    book-added:
      port: "12000"
      endpoint: /book-added
      method: POST
```

### 5. Sensors — OTel Env Vars

**Files:** Existing local overlay sensors under `deploy/argo-events/overlays/local/sensors/`

Each Sensor gets a `spec.template.container.env` block with OTEL configuration.

Service name pattern: `<sensor-name>`

| Sensor | OTEL_SERVICE_NAME |
|---|---|
| book-added-sensor | `book-added-sensor` |
| review-submitted-sensor | `review-submitted-sensor` |
| rating-submitted-sensor | `rating-submitted-sensor` |

Example (book-added-sensor):

```yaml
spec:
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "book-added-sensor"
```

### 6. Kustomization Update

**File:** `deploy/argo-events/overlays/local/kustomization.yaml`

Update EventSource references to point to local overlay copies instead of base:

```yaml
resources:
  - eventbus.yaml
  - eventsource-services.yaml
  - eventsources/book-added.yaml        # was ../../eventsources/book-added.yaml
  - eventsources/review-submitted.yaml   # was ../../eventsources/review-submitted.yaml
  - eventsources/rating-submitted.yaml   # was ../../eventsources/rating-submitted.yaml
  - sensors/book-added-sensor.yaml
  - sensors/review-submitted-sensor.yaml
  - sensors/rating-submitted-sensor.yaml
```

## Files Changed Summary

| File | Action | Description |
|---|---|---|
| `deploy/platform/local/argo-events-values.yaml` | Edit | Add `global.image` block |
| `Makefile` | Edit | Add CRD curl+apply before Helm install |
| `deploy/argo-events/overlays/local/eventbus.yaml` | Edit | Add `consumerBatchMaxWait: "0"` |
| `deploy/argo-events/overlays/local/eventsources/book-added.yaml` | New | Local overlay with OTEL env vars |
| `deploy/argo-events/overlays/local/eventsources/review-submitted.yaml` | New | Local overlay with OTEL env vars |
| `deploy/argo-events/overlays/local/eventsources/rating-submitted.yaml` | New | Local overlay with OTEL env vars |
| `deploy/argo-events/overlays/local/sensors/book-added-sensor.yaml` | Edit | Add OTEL env vars |
| `deploy/argo-events/overlays/local/sensors/review-submitted-sensor.yaml` | Edit | Add OTEL env vars |
| `deploy/argo-events/overlays/local/sensors/rating-submitted-sensor.yaml` | Edit | Add OTEL env vars |
| `deploy/argo-events/overlays/local/kustomization.yaml` | Edit | Point EventSources to local overlays |

## Validation

Run `make run-k8s` to deploy the full stack and verify:

1. Argo Events controller, eventsource, and sensor pods use the custom image
2. CRDs include the new fields (`consumerBatchMaxWait`, `eventBusConsumerBatchMaxWait`, `Extensions` in EventContext)
3. EventBus is configured with `consumerBatchMaxWait: "0"`
4. EventSource and Sensor pods have OTEL env vars set
5. Traces appear in Grafana/Tempo showing `eventsource.publish` and `sensor.trigger` spans
6. End-to-end trace propagation works: productpage -> eventsource -> sensor -> service
