# Observability Stack Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the observability stack — full log-trace-metric correlation, RED metrics from traces, service graph, Envoy Gateway metrics, exemplars, and provisioned Grafana dashboards.

**Architecture:** Modify the Go logging middleware to inject trace context into logs, configure Tempo's metrics generator processors (span-metrics + service-graphs) to produce RED metrics from traces, enable Envoy Gateway Prometheus metrics with Alloy scraping, fix Grafana datasource cross-links, and provision two dashboards via ConfigMap sidecar.

**Tech Stack:** Go (slog + OTel), Grafana Alloy, Grafana Tempo, Prometheus, Loki, Envoy Gateway, Grafana dashboards (JSON provisioning)

**Spec:** `docs/superpowers/specs/2026-04-09-observability-stack-completion-design.md`

---

## File Map

| File | Action | Purpose |
| ---- | ------ | ------- |
| `pkg/logging/middleware.go` | Modify | Inject `trace_id` and `span_id` from OTel span context into structured logs |
| `pkg/logging/logging_test.go` | Modify | Add test for trace context injection |
| `deploy/observability/local/alloy-metrics-traces-values.yaml` | Modify | Add `k8s.cluster.name` attribute processor + Envoy scrape pipeline (this is the deployed config — the inline `configMap.content`) |
| `deploy/observability/local/alloy-metrics-traces-config.alloy` | Modify | Keep standalone config file in sync with values file |
| `deploy/observability/local/tempo-values.yaml` | Modify | Configure span-metrics + service-graphs processors with dimensions and exemplars |
| `deploy/gateway/base/envoy-proxy.yaml` | Modify | Add `metrics` section to enable Prometheus endpoint + virtual host stats |
| `deploy/observability/local/kube-prometheus-stack-values.yaml` | Modify | Fix Tempo datasource tracesToLogsV2 and tracesToMetrics config |
| `deploy/observability/local/dashboards/app-observability.json` | Create | Application observability dashboard JSON |
| `deploy/observability/local/dashboards/envoy-gateway.json` | Create | Envoy Gateway per-route dashboard JSON |
| `deploy/observability/local/dashboards/kustomization.yaml` | Create | Kustomization with configMapGenerator for dashboard provisioning via `grafana_dashboard: "1"` label |

---

### Task 1: Log-Trace Correlation — Test

**Files:**
- Modify: `pkg/logging/logging_test.go`

- [ ] **Step 1: Write failing test for trace_id and span_id in logs**

Add this test at the end of `pkg/logging/logging_test.go` (before the `splitJSONLines` helper):

```go
func TestMiddleware_InjectsTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	// Create a request with a valid OTel span context injected
	traceID, _ := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	spanCtx := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), spanCtx)
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	foundTrace := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["trace_id"] == "4bf92f3577b34da6a3ce929d0e0e4736" &&
			entry["span_id"] == "00f067aa0ba902b7" {
			foundTrace = true
			break
		}
	}

	if !foundTrace {
		t.Errorf("expected trace_id and span_id in log output, got:\n%s", output)
	}
}

func TestMiddleware_NoTraceContext_OmitsFields(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWithWriter("info", "test-service", &buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxLogger := logging.FromContext(r.Context())
		ctxLogger.Info("inside handler")
		w.WriteHeader(http.StatusOK)
	})

	handler := logging.Middleware(logger)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	lines := splitJSONLines(output)

	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if _, ok := entry["trace_id"]; ok {
			t.Errorf("did not expect trace_id when no span context, got:\n%s", output)
		}
	}
}
```

Also add the import for `oteltrace` at the top of the test file. The full imports block becomes:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/logging/... -run TestMiddleware_InjectsTraceContext -v`

Expected: FAIL — logs won't contain `trace_id` or `span_id` because the middleware doesn't extract them yet.

- [ ] **Step 3: Commit failing test**

```bash
git add pkg/logging/logging_test.go
git commit -s -m "test(logging): add tests for trace context injection in middleware"
```

---

### Task 2: Log-Trace Correlation — Implementation

**Files:**
- Modify: `pkg/logging/middleware.go`

- [ ] **Step 1: Add trace context extraction to middleware**

In `pkg/logging/middleware.go`, add the `oteltrace` import and inject trace fields after the request logger is created.

Updated imports:

```go
import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"
)
```

In the `Middleware` function, add the span context extraction **after** the `reqLogger` is created (after line 45 in current file) and **before** `ctx := WithContext(...)`:

```go
			reqLogger := logger.With(
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			)

			// Inject trace context if an active span exists.
			// otelhttp middleware runs before this, so span context is available.
			spanCtx := oteltrace.SpanContextFromContext(r.Context())
			if spanCtx.IsValid() {
				reqLogger = reqLogger.With(
					slog.String("trace_id", spanCtx.TraceID().String()),
					slog.String("span_id", spanCtx.SpanID().String()),
				)
			}

			ctx := WithContext(r.Context(), reqLogger)
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./pkg/logging/... -v`

Expected: ALL PASS — both new tests and existing tests.

- [ ] **Step 3: Run full test suite**

Run: `make test`

Expected: All tests pass. The new `go.opentelemetry.io/otel/trace` import is already an indirect dependency in `go.mod` (line 44: `go.opentelemetry.io/otel/trace v1.43.0 // indirect`). Running `go mod tidy` after may promote it to direct.

- [ ] **Step 4: Run go mod tidy**

Run: `go mod tidy`

This promotes `go.opentelemetry.io/otel/trace` from indirect to direct dependency in `go.mod`.

- [ ] **Step 5: Commit**

```bash
git add pkg/logging/middleware.go go.mod go.sum
git commit -s -m "feat(logging): inject trace_id and span_id from OTel context into structured logs"
```

---

### Task 3: Alloy Config — k8s.cluster.name + Envoy Scrape Pipeline

**Files:**
- Modify: `deploy/observability/local/alloy-metrics-traces-values.yaml`
- Modify: `deploy/observability/local/alloy-metrics-traces-config.alloy`

The Alloy config exists in two places:
1. **Inline in the Helm values** (`alloy-metrics-traces-values.yaml` under `alloy.configMap.content`) — this is what gets deployed
2. **Standalone file** (`alloy-metrics-traces-config.alloy`) — reference copy, keep in sync

- [ ] **Step 1: Update the Alloy config with cluster attribute processor and Envoy scrape**

Replace the full `alloy.configMap.content` in `deploy/observability/local/alloy-metrics-traces-values.yaml` with this content (changes: new `otelcol.processor.attributes` between k8sattributes and batch, updated k8sattributes output, new Envoy scrape pipeline at the bottom):

```yaml
alloy:
  clustering:
    enabled: false
  extraPorts:
    - name: otlp-grpc
      port: 4317
      targetPort: 4317
      protocol: TCP
    - name: otlp-http
      port: 4318
      targetPort: 4318
      protocol: TCP
  configMap:
    content: |-
      // Receives OTLP traces from apps, scrapes Prometheus metrics, forwards to backends.

      // -- OTLP Trace Receiver --
      otelcol.receiver.otlp "default" {
        grpc {
          endpoint = "0.0.0.0:4317"
        }
        http {
          endpoint = "0.0.0.0:4318"
        }
        output {
          traces = [otelcol.processor.k8sattributes.default.input]
        }
      }

      // -- K8s Metadata Enrichment --
      otelcol.processor.k8sattributes "default" {
        extract {
          metadata = [
            "k8s.namespace.name",
            "k8s.deployment.name",
            "k8s.pod.name",
            "k8s.pod.uid",
            "k8s.node.name",
          ]
        }
        output {
          traces = [otelcol.processor.attributes.cluster.input]
        }
      }

      // -- Static Cluster Name Attribute --
      otelcol.processor.attributes "cluster" {
        action {
          key    = "k8s.cluster.name"
          value  = "bookinfo-local"
          action = "insert"
        }
        output {
          traces = [otelcol.processor.batch.default.input]
        }
      }

      // -- Batch Processor --
      otelcol.processor.batch "default" {
        timeout = "2s"
        send_batch_size = 512
        output {
          traces = [otelcol.exporter.otlp.tempo.input]
        }
      }

      // -- Export Traces to Tempo --
      otelcol.exporter.otlp "tempo" {
        client {
          endpoint = "tempo.observability.svc.cluster.local:4317"
          tls {
            insecure = true
          }
        }
      }

      // -- Prometheus Metrics Scraping (Bookinfo apps) --
      discovery.kubernetes "bookinfo_pods" {
        role = "pod"
        namespaces {
          names = ["bookinfo"]
        }
      }

      discovery.relabel "bookinfo_metrics" {
        targets = discovery.kubernetes.bookinfo_pods.targets

        rule {
          source_labels = ["__meta_kubernetes_pod_container_port_number"]
          regex         = "9090"
          action        = "keep"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_ip"]
          target_label  = "__address__"
          replacement   = "$1:9090"
        }
        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_label_app"]
          target_label  = "app"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_label_role"]
          target_label  = "role"
        }
        rule {
          replacement  = "/metrics"
          target_label = "__metrics_path__"
        }
      }

      prometheus.scrape "bookinfo" {
        targets    = discovery.relabel.bookinfo_metrics.output
        forward_to = [prometheus.remote_write.default.receiver]
        scrape_interval = "15s"
      }

      // -- Prometheus Metrics Scraping (Envoy Gateway) --
      discovery.kubernetes "envoy_pods" {
        role = "pod"
        namespaces {
          names = ["envoy-gateway-system"]
        }
      }

      discovery.relabel "envoy_metrics" {
        targets = discovery.kubernetes.envoy_pods.targets

        rule {
          source_labels = ["__meta_kubernetes_pod_label_gateway_envoyproxy_io_owning_gateway_name"]
          regex         = "default-gw"
          action        = "keep"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_ip"]
          target_label  = "__address__"
          replacement   = "$1:19001"
        }
        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }
        rule {
          replacement  = "/stats/prometheus"
          target_label = "__metrics_path__"
        }
      }

      prometheus.scrape "envoy" {
        targets    = discovery.relabel.envoy_metrics.output
        forward_to = [prometheus.remote_write.default.receiver]
        scrape_interval = "15s"
      }

      prometheus.remote_write "default" {
        endpoint {
          url = "http://prometheus-kube-prometheus-prometheus.observability.svc.cluster.local:9090/api/v1/write"
        }
      }

controller:
  type: deployment
  replicas: 1

serviceAccount:
  create: true

rbac:
  create: true
```

- [ ] **Step 2: Update the standalone config file to match**

Replace the full content of `deploy/observability/local/alloy-metrics-traces-config.alloy` with the same Alloy config (the content inside the `|-` block above, without the YAML wrapper). This file is the content between the `content: |-` line and the `controller:` line.

- [ ] **Step 3: Commit**

```bash
git add deploy/observability/local/alloy-metrics-traces-values.yaml deploy/observability/local/alloy-metrics-traces-config.alloy
git commit -s -m "feat(alloy): add k8s.cluster.name attribute processor and Envoy Gateway scrape pipeline"
```

---

### Task 4: Tempo Metrics Generator Processors

**Files:**
- Modify: `deploy/observability/local/tempo-values.yaml`

- [ ] **Step 1: Configure span-metrics and service-graphs processors**

Replace the full content of `deploy/observability/local/tempo-values.yaml` with:

```yaml
tempo:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: "0.0.0.0:4317"
        http:
          endpoint: "0.0.0.0:4318"
  metricsGenerator:
    enabled: true
    remoteWriteUrl: "http://prometheus-kube-prometheus-prometheus.observability.svc.cluster.local:9090/api/v1/write"
    processor:
      span_metrics:
        dimensions:
          - http.method
          - http.target
          - http.status_code
          - k8s.namespace.name
          - k8s.deployment.name
          - k8s.node.name
          - k8s.cluster.name
          - server.address
        histogram_buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
      service_graphs:
        histogram_buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
        dimensions:
          - http.method
          - k8s.namespace.name
          - k8s.cluster.name
  overrides:
    defaults:
      metrics_generator:
        processors:
          - service-graphs
          - span-metrics
  storage:
    trace:
      backend: local
      local:
        path: /var/tempo/traces
      wal:
        path: /var/tempo/wal

persistence:
  enabled: true
  size: 2Gi

resources:
  requests:
    cpu: 50m
    memory: 128Mi
  limits:
    cpu: 300m
    memory: 512Mi
```

**Key changes from current file:**
- Added `processor.span_metrics` with 8 dimensions and histogram buckets
- Added `processor.service_graphs` with 3 dimensions and histogram buckets
- Added `overrides.defaults.metrics_generator.processors` list to enable both processors

**Note on `send_exemplars`:** The Tempo Helm chart's `remoteWriteUrl` is a convenience key. If exemplars don't appear in Prometheus after deployment, the remote write config may need to be switched to the full-form `metricsGenerator.remoteWrite` array with `send_exemplars: true`. Verify after deployment by checking for exemplars on `traces_spanmetrics_latency` in Grafana.

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/tempo-values.yaml
git commit -s -m "feat(tempo): configure span-metrics and service-graphs processors with dimensions"
```

---

### Task 5: Envoy Gateway Metrics

**Files:**
- Modify: `deploy/gateway/base/envoy-proxy.yaml`

- [ ] **Step 1: Add metrics section to EnvoyProxy resource**

Replace the full content of `deploy/gateway/base/envoy-proxy.yaml` with:

```yaml
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: EnvoyProxy
metadata:
  name: otel
  namespace: envoy-gateway-system
spec:
  telemetry:
    tracing:
      samplingRate: 100
      provider:
        backendRefs:
          - name: alloy-metrics-traces
            namespace: observability
            port: 4317
        type: OpenTelemetry
      customTags:
        "k8s.pod.name":
          type: Environment
          environment:
            name: ENVOY_POD_NAME
            defaultValue: "-"
        "k8s.namespace.name":
          type: Environment
          environment:
            name: ENVOY_POD_NAMESPACE
            defaultValue: "envoy-gateway-system"
    metrics:
      prometheus: {}
      enableVirtualHostStats: true
```

**Changes:** Added `metrics` block with:
- `prometheus: {}` — enables `/stats/prometheus` endpoint on Envoy's admin port (19001)
- `enableVirtualHostStats: true` — generates per-route metrics. The default `clusterStatName` template includes HTTPRoute names in labels.

- [ ] **Step 2: Commit**

```bash
git add deploy/gateway/base/envoy-proxy.yaml
git commit -s -m "feat(gateway): enable Prometheus metrics and virtual host stats on Envoy proxy"
```

---

### Task 6: Grafana Datasource Fixes

**Files:**
- Modify: `deploy/observability/local/kube-prometheus-stack-values.yaml`

- [ ] **Step 1: Update Tempo datasource with tracesToLogsV2 and tracesToMetrics config**

Replace the full content of `deploy/observability/local/kube-prometheus-stack-values.yaml` with:

```yaml
prometheus:
  prometheusSpec:
    replicas: 1
    retention: 24h
    enableRemoteWriteReceiver: true
    enableFeatures:
      - exemplar-storage
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
      limits:
        cpu: 500m
        memory: 512Mi
    storageSpec:
      volumeClaimTemplate:
        spec:
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: 2Gi
  service:
    type: NodePort
    nodePort: 30900

grafana:
  enabled: true
  adminPassword: admin
  service:
    type: NodePort
    nodePort: 30300
  sidecar:
    dashboards:
      enabled: true
      searchNamespace: observability
  additionalDataSources:
    - name: Tempo
      type: tempo
      url: http://tempo.observability.svc.cluster.local:3200
      access: proxy
      isDefault: false
      jsonData:
        tracesToLogsV2:
          datasourceUid: loki
          filterByTraceID: true
          filterBySpanID: false
          spanStartTimeShift: "-5m"
          spanEndTimeShift: "5m"
          tags:
            - key: "service.name"
              value: "app"
        tracesToMetrics:
          datasourceUid: prometheus
          spanStartTimeShift: "-5m"
          spanEndTimeShift: "5m"
          tags:
            - key: "service.name"
              value: "service"
          queries:
            - name: "Request rate"
              query: "sum(rate(traces_spanmetrics_calls_total{$$__tags}[5m]))"
            - name: "Error rate"
              query: "sum(rate(traces_spanmetrics_calls_total{$$__tags, status_code=\"STATUS_CODE_ERROR\"}[5m]))"
            - name: "Latency (p95)"
              query: "histogram_quantile(0.95, sum(rate(traces_spanmetrics_latency_bucket{$$__tags}[5m])) by (le))"
        serviceMap:
          datasourceUid: prometheus
        nodeGraph:
          enabled: true
    - name: Loki
      type: loki
      uid: loki
      url: http://loki.observability.svc.cluster.local:3100
      access: proxy
      isDefault: false
      jsonData:
        derivedFields:
          - datasourceUid: tempo
            matcherRegex: '"trace_id":"(\w+)"'
            name: TraceID
            url: "$${__value.raw}"
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 300m
      memory: 256Mi

alertmanager:
  enabled: false
kubeControllerManager:
  enabled: false
kubeScheduler:
  enabled: false
kubeProxy:
  enabled: false
kubeEtcd:
  enabled: false
```

**Key changes from current file:**
- Added `prometheus.prometheusSpec.enableFeatures: ["exemplar-storage"]` — required for Prometheus to accept and store exemplars from Tempo
- Added `grafana.sidecar.dashboards.enabled: true` and `searchNamespace: observability` — enables ConfigMap-based dashboard provisioning
- Updated `tracesToLogsV2` — added `filterBySpanID: false`, `spanStartTimeShift`, `spanEndTimeShift`, and `tags` for service-level log filtering
- Added `tracesToMetrics` — configured with span metrics queries (rate, error rate, p95 latency) for inline metric links from traces

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/kube-prometheus-stack-values.yaml
git commit -s -m "feat(grafana): fix datasource cross-links and enable exemplar storage and dashboard sidecar"
```

---

### Task 7: Application Observability Dashboard

**Files:**
- Create: `deploy/observability/local/dashboards/app-observability.json`

- [ ] **Step 1: Create dashboards directory**

Run: `mkdir -p deploy/observability/local/dashboards`

- [ ] **Step 2: Create the application observability dashboard JSON**

Create `deploy/observability/local/dashboards/app-observability.json`:

```json
{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "links": [],
  "panels": [
    {
      "title": "Request Rate by Service (from Traces)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (service) (rate(traces_spanmetrics_calls_total[5m]))",
          "legendFormat": "{{ service }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Error Rate by Service (from Traces)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (service) (rate(traces_spanmetrics_calls_total{status_code=\"STATUS_CODE_ERROR\"}[5m]))",
          "legendFormat": "{{ service }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 },
          "color": { "mode": "palette-classic" }
        },
        "overrides": []
      }
    },
    {
      "title": "p50 / p90 / p99 Latency by Service (from Traces)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 8 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "histogram_quantile(0.50, sum by (le, service) (rate(traces_spanmetrics_latency_bucket[5m])))",
          "legendFormat": "{{ service }} p50"
        },
        {
          "expr": "histogram_quantile(0.90, sum by (le, service) (rate(traces_spanmetrics_latency_bucket[5m])))",
          "legendFormat": "{{ service }} p90"
        },
        {
          "expr": "histogram_quantile(0.99, sum by (le, service) (rate(traces_spanmetrics_latency_bucket[5m])))",
          "legendFormat": "{{ service }} p99"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "s",
          "custom": { "drawStyle": "line", "fillOpacity": 5 }
        },
        "overrides": []
      }
    },
    {
      "title": "App HTTP Request Duration (from /metrics)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 16 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum by (le, app) (rate(http_server_request_duration_seconds_bucket[5m])))",
          "legendFormat": "{{ app }} p95"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "s",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "App HTTP Requests Total (from /metrics)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 16 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (app) (rate(http_server_requests_total[5m]))",
          "legendFormat": "{{ app }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Business Metrics",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 24 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(ratings_submitted_total[5m]))",
          "legendFormat": "ratings submitted"
        },
        {
          "expr": "sum(rate(reviews_submitted_total[5m]))",
          "legendFormat": "reviews submitted"
        },
        {
          "expr": "sum(rate(books_added_total[5m]))",
          "legendFormat": "books added"
        },
        {
          "expr": "sum(rate(notifications_dispatched_total[5m]))",
          "legendFormat": "notifications dispatched"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Goroutines by Service",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 24 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (app) (process_runtime_go_goroutines)",
          "legendFormat": "{{ app }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Recent Logs",
      "type": "logs",
      "gridPos": { "h": 10, "w": 24, "x": 0, "y": 32 },
      "datasource": { "type": "loki", "uid": "loki" },
      "targets": [
        {
          "expr": "{namespace=\"bookinfo\"} | json",
          "refId": "A"
        }
      ],
      "options": {
        "showTime": true,
        "showLabels": false,
        "showCommonLabels": false,
        "wrapLogMessage": true,
        "prettifyLogMessage": false,
        "enableLogDetails": true,
        "dedupStrategy": "none",
        "sortOrder": "Descending"
      }
    }
  ],
  "schemaVersion": 39,
  "tags": ["bookinfo", "observability"],
  "templating": {
    "list": []
  },
  "time": {
    "from": "now-30m",
    "to": "now"
  },
  "timepicker": {},
  "timezone": "browser",
  "title": "Bookinfo - Application Observability",
  "uid": "bookinfo-app-observability"
}
```

- [ ] **Step 3: Commit**

```bash
git add deploy/observability/local/dashboards/app-observability.json
git commit -s -m "feat(grafana): add application observability dashboard with RED metrics and logs"
```

---

### Task 8: Envoy Gateway Dashboard

**Files:**
- Create: `deploy/observability/local/dashboards/envoy-gateway.json`

- [ ] **Step 1: Create the Envoy Gateway dashboard JSON**

Create `deploy/observability/local/dashboards/envoy-gateway.json`:

```json
{
  "annotations": {
    "list": []
  },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "links": [],
  "panels": [
    {
      "title": "Total Gateway Throughput",
      "type": "stat",
      "gridPos": { "h": 4, "w": 8, "x": 0, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(envoy_http_downstream_rq_total[5m]))",
          "legendFormat": "req/s"
        }
      ],
      "fieldConfig": {
        "defaults": { "unit": "reqps" },
        "overrides": []
      }
    },
    {
      "title": "Global Error Rate (5xx)",
      "type": "stat",
      "gridPos": { "h": 4, "w": 8, "x": 8, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(envoy_http_downstream_rq_xx{envoy_response_code_class=\"5\"}[5m])) / sum(rate(envoy_http_downstream_rq_total[5m]))",
          "legendFormat": "error %"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "green", "value": null },
              { "color": "yellow", "value": 0.01 },
              { "color": "red", "value": 0.05 }
            ]
          }
        },
        "overrides": []
      }
    },
    {
      "title": "Gateway p95 Latency",
      "type": "stat",
      "gridPos": { "h": 4, "w": 8, "x": 16, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(envoy_http_downstream_rq_time_bucket[5m])) by (le))",
          "legendFormat": "p95"
        }
      ],
      "fieldConfig": {
        "defaults": { "unit": "ms" },
        "overrides": []
      }
    },
    {
      "title": "Request Rate per HTTPRoute",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (envoy_http_conn_manager_prefix) (rate(envoy_http_downstream_rq_total[5m]))",
          "legendFormat": "{{ envoy_http_conn_manager_prefix }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Error Rate per HTTPRoute (5xx)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (envoy_http_conn_manager_prefix) (rate(envoy_http_downstream_rq_xx{envoy_response_code_class=\"5\"}[5m]))",
          "legendFormat": "{{ envoy_http_conn_manager_prefix }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 },
          "color": { "mode": "palette-classic" }
        },
        "overrides": []
      }
    },
    {
      "title": "Latency per HTTPRoute (p95)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 12 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum by (le, envoy_http_conn_manager_prefix) (rate(envoy_http_downstream_rq_time_bucket[5m])))",
          "legendFormat": "{{ envoy_http_conn_manager_prefix }} p95"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "ms",
          "custom": { "drawStyle": "line", "fillOpacity": 5 }
        },
        "overrides": []
      }
    },
    {
      "title": "Upstream Request Rate per Cluster",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 20 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (envoy_cluster_name) (rate(envoy_cluster_upstream_rq_total[5m]))",
          "legendFormat": "{{ envoy_cluster_name }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Active Upstream Connections",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 20 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum by (envoy_cluster_name) (envoy_cluster_upstream_cx_active)",
          "legendFormat": "{{ envoy_cluster_name }}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "short",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    }
  ],
  "schemaVersion": 39,
  "tags": ["bookinfo", "envoy", "gateway"],
  "templating": {
    "list": []
  },
  "time": {
    "from": "now-30m",
    "to": "now"
  },
  "timepicker": {},
  "timezone": "browser",
  "title": "Bookinfo - Envoy Gateway",
  "uid": "bookinfo-envoy-gateway"
}
```

**Note on metric label names:** The `envoy_http_conn_manager_prefix` and `envoy_cluster_name` labels are the standard Envoy Prometheus metric labels. With `enableVirtualHostStats: true` and the default `clusterStatName` template, the cluster names will contain HTTPRoute references (e.g., `HTTPRoute/bookinfo/details-read/rule/0`). After deployment, check actual label names with `curl localhost:19001/stats/prometheus | head -50` (port-forwarded from the Envoy pod) and adjust the dashboard queries if needed.

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/dashboards/envoy-gateway.json
git commit -s -m "feat(grafana): add Envoy Gateway per-route dashboard"
```

---

### Task 9: Dashboard ConfigMaps

**Files:**
- Create: `deploy/observability/local/dashboards/dashboard-configmaps.yaml`

- [ ] **Step 1: Create ConfigMap manifests for both dashboards**

Create `deploy/observability/local/dashboards/dashboard-configmaps.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboard-app-observability
  namespace: observability
  labels:
    grafana_dashboard: "1"
data:
  app-observability.json: |
    %%APP_OBSERVABILITY_JSON%%
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboard-envoy-gateway
  namespace: observability
  labels:
    grafana_dashboard: "1"
data:
  envoy-gateway.json: |
    %%ENVOY_GATEWAY_JSON%%
```

**Important:** The `%%APP_OBSERVABILITY_JSON%%` and `%%ENVOY_GATEWAY_JSON%%` placeholders must be replaced with the actual JSON content from the dashboard files, indented by 4 spaces to be valid YAML under `data:`. The cleanest approach is to use `kubectl create configmap` commands instead of a static manifest. Replace this file with a shell script or use `kubectl` directly:

```bash
kubectl create configmap grafana-dashboard-app-observability \
  --from-file=app-observability.json=deploy/observability/local/dashboards/app-observability.json \
  -n observability --dry-run=client -o yaml | \
  kubectl label -f - --dry-run=client -o yaml --local grafana_dashboard=1 > /tmp/cm-app.yaml

kubectl create configmap grafana-dashboard-envoy-gateway \
  --from-file=envoy-gateway.json=deploy/observability/local/dashboards/envoy-gateway.json \
  -n observability --dry-run=client -o yaml | \
  kubectl label -f - --dry-run=client -o yaml --local grafana_dashboard=1 > /tmp/cm-envoy.yaml
```

**Better approach:** Add a Makefile target to apply these. Update the `dashboard-configmaps.yaml` to be a kustomization that uses `configMapGenerator`:

Create `deploy/observability/local/dashboards/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: observability

configMapGenerator:
  - name: grafana-dashboard-app-observability
    options:
      disableNameSuffixHash: true
      labels:
        grafana_dashboard: "1"
    files:
      - app-observability.json
  - name: grafana-dashboard-envoy-gateway
    options:
      disableNameSuffixHash: true
      labels:
        grafana_dashboard: "1"
    files:
      - envoy-gateway.json
```

This is cleaner — `kubectl apply -k deploy/observability/local/dashboards/` generates ConfigMaps from the JSON files with the correct label.

- [ ] **Step 2: Delete the dashboard-configmaps.yaml placeholder**

Remove `deploy/observability/local/dashboards/dashboard-configmaps.yaml` if it was created — the kustomization approach replaces it.

- [ ] **Step 3: Commit**

```bash
git add deploy/observability/local/dashboards/kustomization.yaml
git rm -f deploy/observability/local/dashboards/dashboard-configmaps.yaml 2>/dev/null || true
git commit -s -m "feat(grafana): add kustomization for dashboard ConfigMap provisioning"
```

---

### Task 10: Update Makefile for Dashboard Deployment

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add dashboard deployment step to k8s-observability target**

In the `Makefile`, add a step at the end of the `k8s-observability` target (after the Alloy metrics+traces install, before the final "complete" printf) to apply the dashboard ConfigMaps:

Find the line:
```
	@printf "  $(GREEN)Alloy (metrics+traces) ready.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Observability layer complete.$(NC)\n\n"
```

Insert between them:
```
	@printf "$(BOLD)[6/6] Applying Grafana dashboards...$(NC)\n"
	@$(KUBECTL) apply -k deploy/observability/local/dashboards/
	@printf "  $(GREEN)Grafana dashboards applied.$(NC)\n"
```

Also update the step numbering in the existing targets from `[1/5]` through `[5/5]` to `[1/6]` through `[5/6]`.

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -s -m "feat(makefile): add Grafana dashboard deployment step to k8s-observability"
```

---

### Task 11: Deploy and Verify

This task deploys all changes to the running k3d cluster and runs the verification plan.

- [ ] **Step 1: Rebuild app images (logging change)**

Run: `make k8s-rebuild`

This rebuilds all 5 service images with the trace context logging change and redeploys them.

- [ ] **Step 2: Upgrade observability stack (Alloy + Tempo + Prometheus/Grafana)**

Run the individual Helm upgrade commands for the changed components:

```bash
# Upgrade Tempo (processors)
helm upgrade --install tempo grafana/tempo \
  -n observability \
  -f deploy/observability/local/tempo-values.yaml \
  --wait --timeout 120s \
  --kube-context=k3d-bookinfo-local

# Upgrade Alloy (k8s.cluster.name + Envoy scrape)
helm upgrade --install alloy-metrics-traces grafana/alloy \
  -n observability \
  -f deploy/observability/local/alloy-metrics-traces-values.yaml \
  --wait --timeout 120s \
  --kube-context=k3d-bookinfo-local

# Upgrade kube-prometheus-stack (datasource fixes + exemplar storage + sidecar)
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  -n observability \
  -f deploy/observability/local/kube-prometheus-stack-values.yaml \
  --wait --timeout 300s \
  --kube-context=k3d-bookinfo-local
```

- [ ] **Step 3: Apply Envoy Gateway config**

Run: `kubectl apply -k deploy/gateway/base/ --context=k3d-bookinfo-local`

The Envoy proxy pods will be recreated with the new metrics configuration.

- [ ] **Step 4: Apply dashboard ConfigMaps**

Run: `kubectl apply -k deploy/observability/local/dashboards/ --context=k3d-bookinfo-local`

- [ ] **Step 5: Wait for pods to stabilize**

Run:
```bash
kubectl get pods -n observability --context=k3d-bookinfo-local
kubectl get pods -n envoy-gateway-system --context=k3d-bookinfo-local
kubectl get pods -n bookinfo --context=k3d-bookinfo-local
```

All pods should be Running. If any are restarting, check logs with `kubectl logs <pod> -n <ns> --context=k3d-bookinfo-local`.

- [ ] **Step 6: Generate traffic — POST a rating**

```bash
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id": "1", "rating": 5}' | jq .
```

Expected: 201 Created response with the rating object.

- [ ] **Step 7: Verify logs contain trace_id**

Open Grafana at http://localhost:3000, go to Explore, select Loki datasource, run:

```
{namespace="bookinfo"} | json | trace_id != ""
```

Expected: Log entries show `trace_id` and `span_id` fields.

- [ ] **Step 8: Verify Loki-to-Tempo link**

In the log result, click the TraceID link on a log entry. It should open the trace in Tempo.

- [ ] **Step 9: Verify Tempo-to-Loki link**

In Tempo, open a trace. Click the "Logs for this span" button. It should show related logs from Loki.

- [ ] **Step 10: Verify RED metrics in Explore Traces**

Open http://localhost:3000/a/grafana-exploretraces-app/explore

Expected: RED metrics (rate, errors, duration) appear grouped by `resource.service.name`.

- [ ] **Step 11: Verify Service Graph**

In Grafana, go to Explore, select Tempo datasource, switch to "Service Graph" tab.

Expected: Shows topology with edges between services (Envoy -> productpage -> details/reviews -> ratings).

- [ ] **Step 12: Verify Envoy metrics in Prometheus**

Open Prometheus at http://localhost:9090, query:

```
envoy_http_downstream_rq_total
```

Expected: Metrics appear with route-level labels.

- [ ] **Step 13: Verify dashboards loaded**

Open Grafana at http://localhost:3000, go to Dashboards. Look for:
- "Bookinfo - Application Observability"
- "Bookinfo - Envoy Gateway"

Both should appear and show data.

- [ ] **Step 14: Verify exemplars (if available)**

In Grafana, open a panel with `traces_spanmetrics_latency` histogram. Enable "Exemplars" toggle. If Tempo's remote write sent exemplars, small dots should appear on the histogram linking to traces.

**Note:** If exemplars don't appear, the Tempo Helm chart may need the `send_exemplars` setting in a different location. See the note in Task 4. This can be addressed as a follow-up if needed.
