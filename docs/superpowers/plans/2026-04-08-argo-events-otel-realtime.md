# Argo Events OTel Tracing + Kafka Real-Time Processing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade Argo Events to a custom image with OTel distributed tracing and real-time Kafka processing, configure all EventSources and Sensors with tracing, and validate the full stack.

**Architecture:** Custom Argo Events image (`ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`) replaces the upstream image via Helm `global.image` override. Updated CRDs are downloaded from the GitHub release at deploy time. EventBus gets `consumerBatchMaxWait: "0"` for real-time processing. All EventSources and Sensors get OTEL env vars to send traces to the Alloy collector.

**Tech Stack:** Argo Events (custom build), Helm, Kustomize, Kafka EventBus, OpenTelemetry, Alloy (OTLP collector), Tempo (trace backend)

**Spec:** `docs/superpowers/specs/2026-04-08-argo-events-otel-realtime-design.md`

---

### Task 1: Override Argo Events Image in Helm Values

**Files:**
- Modify: `deploy/platform/local/argo-events-values.yaml`

- [ ] **Step 1: Add global.image block to Helm values**

Open `deploy/platform/local/argo-events-values.yaml` and add the `global.image` block at the top, before the existing `controller` section:

```yaml
# Argo Events Controller - local dev values
# Chart: argo/argo-events
global:
  image:
    repository: ghcr.io/kaio6fellipe/argo-events
    tag: "prs-3961-3983"

controller:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 250m
      memory: 256Mi

configs:
  jetstream:
    versions:
      - version: latest
        natsImage: nats:latest
        metricsExporterImage: natsio/prometheus-nats-exporter:latest
        configReloaderImage: natsio/nats-server-config-reloader:latest
        startCommand: /nats-server
```

- [ ] **Step 2: Commit**

```bash
git add deploy/platform/local/argo-events-values.yaml
git commit -m "feat(deploy): override Argo Events image with custom OTel+realtime build"
```

---

### Task 2: Add CRD Download to Makefile

**Files:**
- Modify: `Makefile` (lines 276-282, the Argo Events section of `k8s-platform`)

- [ ] **Step 1: Add CRD download step before Helm install**

In the `k8s-platform` target, insert a CRD download step between the `[4/5] Installing Argo Events controller...` printf and the `helm repo add` command. The section should become:

```makefile
	@printf "$(BOLD)[4/5] Installing Argo Events controller...$(NC)\n"
	@printf "  Downloading custom CRDs (PRs #3961 + #3983)...\n"
	@curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_eventbus.yaml | $(KUBECTL) apply -f -
	@curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_eventsources.yaml | $(KUBECTL) apply -f -
	@curl -sL https://github.com/kaio6fellipe/event-driven-bookinfo/releases/download/argo-events-prs-3961-3983/argoproj.io_sensors.yaml | $(KUBECTL) apply -f -
	@printf "  $(GREEN)Custom CRDs applied.$(NC)\n"
	@$(HELM) repo add argo https://argoproj.github.io/argo-helm --force-update 2>/dev/null || true
	@$(HELM) upgrade --install argo-events argo/argo-events \
		-n $(K8S_NS_PLATFORM) \
		-f deploy/platform/local/argo-events-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Argo Events controller ready.$(NC)\n"
```

The key change: 4 new lines (printf + 3 curl commands + printf) are inserted right after the `[4/5]` printf and before the `helm repo add` line.

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -m "feat(deploy): download custom Argo Events CRDs at deploy time"
```

---

### Task 3: Enable Real-Time Processing on EventBus

**Files:**
- Modify: `deploy/argo-events/overlays/local/eventbus.yaml`

- [ ] **Step 1: Add consumerBatchMaxWait to EventBus spec**

Replace the full content of `deploy/argo-events/overlays/local/eventbus.yaml`:

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

The `consumerBatchMaxWait: "0"` disables batching for all sensors using this EventBus, enabling real-time message processing (from PR #3983).

- [ ] **Step 2: Commit**

```bash
git add deploy/argo-events/overlays/local/eventbus.yaml
git commit -m "feat(deploy): enable real-time Kafka processing on EventBus"
```

---

### Task 4: Create Local Overlay EventSources with OTel Env Vars

**Files:**
- Create: `deploy/argo-events/overlays/local/eventsources/book-added.yaml`
- Create: `deploy/argo-events/overlays/local/eventsources/review-submitted.yaml`
- Create: `deploy/argo-events/overlays/local/eventsources/rating-submitted.yaml`
- Modify: `deploy/argo-events/overlays/local/kustomization.yaml`

- [ ] **Step 1: Create the eventsources directory**

```bash
mkdir -p deploy/argo-events/overlays/local/eventsources
```

- [ ] **Step 2: Create book-added EventSource overlay**

Create `deploy/argo-events/overlays/local/eventsources/book-added.yaml`:

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

- [ ] **Step 3: Create review-submitted EventSource overlay**

Create `deploy/argo-events/overlays/local/eventsources/review-submitted.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: review-submitted
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "review-submitted-eventsource"
  webhook:
    review-submitted:
      port: "12001"
      endpoint: /review-submitted
      method: POST
```

- [ ] **Step 4: Create rating-submitted EventSource overlay**

Create `deploy/argo-events/overlays/local/eventsources/rating-submitted.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: EventSource
metadata:
  name: rating-submitted
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "rating-submitted-eventsource"
  webhook:
    rating-submitted:
      port: "12002"
      endpoint: /rating-submitted
      method: POST
```

- [ ] **Step 5: Update kustomization.yaml to reference local overlay EventSources**

Replace the full content of `deploy/argo-events/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - eventbus.yaml
  - eventsource-services.yaml
  - eventsources/book-added.yaml
  - eventsources/review-submitted.yaml
  - eventsources/rating-submitted.yaml
  - sensors/book-added-sensor.yaml
  - sensors/review-submitted-sensor.yaml
  - sensors/rating-submitted-sensor.yaml
```

The 3 EventSource references changed from `../../eventsources/<name>.yaml` to `eventsources/<name>.yaml` (local overlay copies).

- [ ] **Step 6: Verify kustomize builds cleanly**

```bash
kubectl kustomize deploy/argo-events/overlays/local/ --load-restrictor LoadRestrictionsNone
```

Expected: YAML output containing all 3 EventSources with `template.container.env` blocks, all 3 Sensors, the EventBus, and the eventsource Services. No errors.

- [ ] **Step 7: Commit**

```bash
git add deploy/argo-events/overlays/local/eventsources/ deploy/argo-events/overlays/local/kustomization.yaml
git commit -m "feat(deploy): add local overlay EventSources with OTel tracing env vars"
```

---

### Task 5: Add OTel Env Vars to Sensors

**Files:**
- Modify: `deploy/argo-events/overlays/local/sensors/book-added-sensor.yaml`
- Modify: `deploy/argo-events/overlays/local/sensors/review-submitted-sensor.yaml`
- Modify: `deploy/argo-events/overlays/local/sensors/rating-submitted-sensor.yaml`

- [ ] **Step 1: Add OTel env vars to book-added-sensor**

Replace the full content of `deploy/argo-events/overlays/local/sensors/book-added-sensor.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: book-added-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "book-added-sensor"
  dependencies:
    - name: book-added-dep
      eventSourceName: book-added
      eventName: book-added
  triggers:
    - template:
        name: create-detail
        http:
          url: http://details-write.bookinfo.svc.cluster.local/v1/details
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-book-added
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: book-added-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: book-added-dep
                value: "New book added"
              dest: body
            - src:
                dependencyName: book-added-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: book-added-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 2: Add OTel env vars to review-submitted-sensor**

Replace the full content of `deploy/argo-events/overlays/local/sensors/review-submitted-sensor.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: review-submitted-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "review-submitted-sensor"
  dependencies:
    - name: review-submitted-dep
      eventSourceName: review-submitted
      eventName: review-submitted
  triggers:
    - template:
        name: create-review
        http:
          url: http://reviews-write.bookinfo.svc.cluster.local/v1/reviews
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-review-submitted
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: review-submitted-dep
                value: "New review submitted"
              dest: body
            - src:
                dependencyName: review-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: review-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 3: Add OTel env vars to rating-submitted-sensor**

Replace the full content of `deploy/argo-events/overlays/local/sensors/rating-submitted-sensor.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Sensor
metadata:
  name: rating-submitted-sensor
  namespace: bookinfo
spec:
  eventBusName: kafka
  template:
    container:
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
        - name: OTEL_SERVICE_NAME
          value: "rating-submitted-sensor"
  dependencies:
    - name: rating-submitted-dep
      eventSourceName: rating-submitted
      eventName: rating-submitted
  triggers:
    - template:
        name: create-rating
        http:
          url: http://ratings-write.bookinfo.svc.cluster.local/v1/ratings
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body
              dest: ""
      retryStrategy:
        steps: 3
        duration: 2s
    - template:
        name: notify-rating-submitted
        http:
          url: http://notification.bookinfo.svc.cluster.local/v1/notifications
          method: POST
          headers:
            Content-Type: application/json
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.title
              dest: subject
            - src:
                dependencyName: rating-submitted-dep
                value: "New rating submitted"
              dest: body
            - src:
                dependencyName: rating-submitted-dep
                value: "system@bookinfo"
              dest: recipient
            - src:
                dependencyName: rating-submitted-dep
                value: "email"
              dest: channel
      retryStrategy:
        steps: 3
        duration: 2s
```

- [ ] **Step 4: Verify kustomize builds cleanly**

```bash
kubectl kustomize deploy/argo-events/overlays/local/ --load-restrictor LoadRestrictionsNone
```

Expected: All 3 Sensors now include `template.container.env` blocks with `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`. No errors.

- [ ] **Step 5: Commit**

```bash
git add deploy/argo-events/overlays/local/sensors/
git commit -m "feat(deploy): add OTel tracing env vars to all Sensors"
```

---

### Task 6: Deploy and Validate Full Stack

**Files:** None (validation only)

- [ ] **Step 1: Tear down existing cluster if running**

```bash
make stop-k8s
```

- [ ] **Step 2: Deploy the full stack**

```bash
make run-k8s
```

This runs: `k8s-cluster` -> `k8s-platform` (includes CRD download + custom image) -> `k8s-observability` -> `k8s-deploy` (applies EventBus, EventSources, Sensors).

Expected: All steps complete without errors. Watch for:
- CRD download succeeds (curl output)
- Argo Events controller pod starts with the custom image
- EventSource and Sensor pods start successfully

- [ ] **Step 3: Verify custom image is in use**

```bash
kubectl get pods -n platform --context=k3d-bookinfo-local -o jsonpath='{range .items[?(@.metadata.labels.app\.kubernetes\.io/name=="argo-events")]}{.spec.containers[0].image}{"\n"}{end}'
```

Expected: `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`

```bash
kubectl get pods -n bookinfo --context=k3d-bookinfo-local -l eventsource-name -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
```

Expected: All eventsource pods use `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`

```bash
kubectl get pods -n bookinfo --context=k3d-bookinfo-local -l sensor-name -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'
```

Expected: All sensor pods use `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`

- [ ] **Step 4: Verify OTEL env vars are set on EventSource pods**

```bash
kubectl get pods -n bookinfo --context=k3d-bookinfo-local -l eventsource-name=book-added -o jsonpath='{.items[0].spec.containers[0].env[*].name}'
```

Expected output includes: `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`

- [ ] **Step 5: Verify OTEL env vars are set on Sensor pods**

```bash
kubectl get pods -n bookinfo --context=k3d-bookinfo-local -l sensor-name=book-added-sensor -o jsonpath='{.items[0].spec.containers[0].env[*].name}'
```

Expected output includes: `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`

- [ ] **Step 6: Verify EventBus has consumerBatchMaxWait**

```bash
kubectl get eventbus kafka -n bookinfo --context=k3d-bookinfo-local -o jsonpath='{.spec.kafka.consumerBatchMaxWait}'
```

Expected: `0`

- [ ] **Step 7: Trigger an event and verify traces in Tempo**

Send a test event through the book-added webhook:

```bash
curl -X POST http://localhost:8443/v1/book-added \
  -H "Content-Type: application/json" \
  -d '{"title":"Trace Test Book","author":"Test Author","year":2026,"isbn":"978-0-000-00000-0"}'
```

Expected: 201 response. Then check Grafana at http://localhost:3000 -> Explore -> Tempo:
- Search for traces with service name `book-added-eventsource`
- Verify you see `eventsource.publish` spans
- Search for traces with service name `book-added-sensor`
- Verify you see `sensor.trigger` spans
- Verify trace context propagation: the sensor trigger HTTP call to `details-write` should carry the trace context, linking the eventsource and sensor spans into a connected trace

- [ ] **Step 8: Verify end-to-end trace propagation**

In Grafana Tempo, find the trace from Step 7 and verify the span tree shows:
1. `eventsource.publish` (book-added-eventsource) -> EventBus
2. `sensor.trigger` (book-added-sensor) -> HTTP POST to details-write
3. `sensor.trigger` (book-added-sensor) -> HTTP POST to notification

The productpage -> eventsource link depends on whether productpage injects `traceparent` in webhook calls (it does via otelhttp from the previous OTel work).
