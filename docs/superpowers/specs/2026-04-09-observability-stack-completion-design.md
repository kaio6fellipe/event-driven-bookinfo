# Observability Stack Completion Design

**Date:** 2026-04-09
**Status:** Approved
**Scope:** Complete the observability stack — full correlation between logs/traces/metrics, RED metrics from traces, service graph, Envoy Gateway metrics, exemplars, and provisioned Grafana dashboards.

---

## Problem Statement

The local k8s cluster has all three observability pillars deployed (Prometheus, Tempo, Loki) and apps emit traces, metrics, and structured logs. However, the pillars are not connected:

- **Logs lack trace context** — no `trace_id`/`span_id` in log lines, so Grafana's Tempo-to-Loki and Loki-to-Tempo links have nothing to match
- **Tempo metrics generator has no processors configured** — `metricsGenerator.enabled: true` but no `span-metrics` or `service-graphs` processors, so zero RED metrics are generated from traces
- **Service graph is unavailable** — depends on the missing `service-graphs` processor
- **Envoy Gateway exports no metrics** — only tracing is configured in the EnvoyProxy resource
- **No exemplars** — Tempo's remote write doesn't enable `send_exemplars`, so trace-derived histograms lack trace ID links
- **No Grafana dashboards** — no provisioned dashboards for app observability or gateway monitoring
- **`k8s.cluster.name` not available** — Alloy's k8sattributes processor doesn't extract or inject this attribute

---

## Design

### Section 1: Log-Trace Correlation

**File:** `pkg/logging/middleware.go`

Modify the logging middleware to extract trace context from the OTel span and inject `trace_id` and `span_id` as structured log fields.

```go
spanCtx := trace.SpanContextFromContext(r.Context())
if spanCtx.IsValid() {
    reqLogger = reqLogger.With(
        slog.String("trace_id", spanCtx.TraceID().String()),
        slog.String("span_id", spanCtx.SpanID().String()),
    )
}
```

**Why this works:** The `otelhttp.NewHandler()` wrapper runs before the logging middleware in `pkg/server/server.go`, so the span context is already in the request context.

**New import:** `go.opentelemetry.io/otel/trace` (already an indirect dependency).

**Result:**
- Loki `derivedFields` regex `'"trace_id":"(\w+)"'` matches — logs link to Tempo
- Tempo `tracesToLogsV2` with `filterByTraceID: true` finds related logs in Loki
- Bidirectional log-trace correlation complete

---

### Section 2: Tempo Metrics Generator + Alloy Cluster Attribute

#### 2a: Alloy — Inject `k8s.cluster.name`

**File:** `deploy/observability/local/alloy-metrics-traces-config.alloy`

Add an `otelcol.processor.attributes` step to insert `k8s.cluster.name: "bookinfo-local"` on all traces. Insert it between k8sattributes and batch processors:

```
otelcol.receiver.otlp → k8sattributes → attributes (k8s.cluster.name) → batch → otlp export
```

```alloy
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
```

Update the k8sattributes output to chain into this new processor instead of directly into batch.

**Why at the Alloy level:** Single config point, applies to all trace sources including Envoy Gateway traces (which don't go through the app OTel SDK).

#### 2b: Tempo — Configure Processors

**File:** `deploy/observability/local/tempo-values.yaml`

Enable both processors with dimensions and exemplars:

```yaml
tempo:
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
```

Remote write must have `send_exemplars: true` enabled (exact Helm chart key to be confirmed during implementation based on chart version).

**Intrinsic dimensions note:** `service.name`, `span.name`, `span.kind`, `status.code` are built-in intrinsic dimensions in span_metrics — they are included by default and must NOT be added as custom dimensions.

**Cardinality note:** `k8s.pod.name` and `client.address` are intentionally excluded — too high-cardinality for aggregated metrics. They belong in traces, not metric labels.

**Metrics generated:**
- `traces_spanmetrics_latency` (histogram) — per service/span/status duration
- `traces_spanmetrics_calls_total` (counter) — request count
- `traces_spanmetrics_size_total` (counter) — payload size
- `traces_service_graph_request_total` — requests between service pairs
- `traces_service_graph_request_failed_total` — failed requests between pairs
- `traces_service_graph_request_server_seconds` — latency between pairs

---

### Section 3: Envoy Gateway Metrics + Alloy Scraping

#### 3a: Enable Envoy Prometheus Metrics

**File:** `deploy/gateway/base/envoy-proxy.yaml`

Add metrics section to the existing EnvoyProxy resource:

```yaml
spec:
  telemetry:
    tracing: ... # existing, unchanged
    metrics:
      prometheus: {}
      enableVirtualHostStats: true
```

- `prometheus: {}` enables `/stats/prometheus` on admin port 19001
- `enableVirtualHostStats: true` provides per-route metrics
- Default `clusterStatName` template (`%ROUTE_KIND%/%ROUTE_NAMESPACE%/%ROUTE_NAME%/rule/%ROUTE_RULE_NUMBER%`) includes HTTPRoute names in metric labels

#### 3b: Alloy Scrape Pipeline for Envoy

**File:** `deploy/observability/local/alloy-metrics-traces-config.alloy`

Add a second scrape pipeline targeting Envoy proxy pods:

```alloy
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
```

**Key metrics available:**
- `envoy_http_downstream_rq_total` — total requests per route
- `envoy_http_downstream_rq_xx` — requests by response code class
- `envoy_http_downstream_rq_time` — request duration histogram
- `envoy_cluster_upstream_rq_total` — upstream requests per cluster (named by HTTPRoute)
- `envoy_cluster_upstream_cx_active` — active connections per upstream

---

### Section 4: Grafana Dashboards (ConfigMap Provisioned)

**Directory:** `deploy/observability/local/dashboards/`

Two dashboards provisioned as ConfigMaps with label `grafana_dashboard: "1"` for kube-prometheus-stack's sidecar auto-discovery.

#### 4a: Application Observability Dashboard

Panels:
- **RED from Tempo** — rate, error rate, p50/p90/p99 latency per service (from `traces_spanmetrics_*`)
- **Service graph** — topology visualization (Grafana built-in service map panel)
- **App-side metrics** — `http_server_request_duration_seconds`, `http_server_requests_total` from services' `/metrics`
- **Business metrics** — `ratings_submitted_total`, `reviews_submitted_total`, `books_added_total`, `notifications_dispatched_total`
- **Runtime** — goroutines, memory usage per service
- **Log panel** — recent logs from Loki filtered by service, with trace ID links

#### 4b: Envoy Gateway Dashboard

Panels:
- **Per-HTTPRoute RED** — request rate, error rate, latency for each of the 8 routes (details-read/write, ratings-read/write, reviews-read/write, productpage, productpage-partials)
- **Upstream health** — active connections per backend
- **Overview row** — total gateway throughput, global error rate, top-level latency

**Provisioning:** Grafana sidecar needs the dashboards' ConfigMaps to be in a namespace it watches. kube-prometheus-stack's sidecar watches for ConfigMaps with the `grafana_dashboard: "1"` label. ConfigMaps will be applied to the `observability` namespace.

**Note:** Dashboard JSON will be generated during implementation once metric names are confirmed in Prometheus.

---

### Section 5: Grafana Datasource Fixes

**File:** `deploy/observability/local/kube-prometheus-stack-values.yaml`

#### Tempo Datasource — tracesToLogsV2

Add time shift and tags for broader log search:

```yaml
tracesToLogsV2:
  datasourceUid: loki
  filterByTraceID: true
  filterBySpanID: false
  spanStartTimeShift: "-5m"
  spanEndTimeShift: "5m"
  tags:
    - key: "service.name"
      value: "app"
```

#### Tempo Datasource — tracesToMetrics

Add span metrics queries for inline metric links from traces:

```yaml
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
```

#### Loki Datasource

No changes needed — `derivedFields` regex `'"trace_id":"(\w+)"'` will work once logs emit trace_id (Section 1).

---

## Deployment Order

1. **App code** — modify logging middleware, rebuild service images
2. **Infrastructure config** — Alloy (cluster attribute + Envoy scrape), Tempo (processors), EnvoyProxy (metrics)
3. **Grafana config** — datasource fixes, dashboard ConfigMaps
4. **Redeploy** — observability stack (Alloy, Tempo, Prometheus/Grafana), gateway, app services

---

## Verification Plan

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | POST a rating submission | Request processed through full CQRS pipeline |
| 2 | Check logs in Loki | Log entries contain `trace_id` and `span_id` fields |
| 3 | Click trace_id in Loki | Opens corresponding trace in Tempo |
| 4 | Open trace in Tempo, click "Logs" tab | Shows related log entries from Loki |
| 5 | Open Explore Traces app | RED metrics (rate, errors, duration) appear grouped by service |
| 6 | Open Service Graph in Grafana | Shows topology: Envoy -> productpage -> details/reviews -> ratings |
| 7 | Open Envoy Gateway dashboard | Per-route request rates visible for all 8 HTTPRoutes |
| 8 | Open App Observability dashboard | RED metrics, business metrics, runtime stats, log panel all populated |
| 9 | Click exemplar on Tempo-generated histogram | Opens the specific trace in Tempo |

---

## Files Changed Summary

| File | Change Type | Section |
|------|------------|---------|
| `pkg/logging/middleware.go` | Modify | 1 |
| `deploy/observability/local/alloy-metrics-traces-config.alloy` | Modify | 2a, 3b |
| `deploy/observability/local/tempo-values.yaml` | Modify | 2b |
| `deploy/gateway/base/envoy-proxy.yaml` | Modify | 3a |
| `deploy/observability/local/kube-prometheus-stack-values.yaml` | Modify | 5 |
| `deploy/observability/local/dashboards/app-observability.json` | New | 4a |
| `deploy/observability/local/dashboards/envoy-gateway.json` | New | 4b |
| Dashboard ConfigMap manifests | New | 4 |

---

## Decisions

- **No app-side exemplars** — rely on Tempo-generated RED metrics for exemplar support. Avoids mixing OTel + prometheus/client_golang in the metrics package.
- **Envoy metrics via Prometheus scrape** — consistent with app metrics collection pattern (Alloy scrapes → Prometheus remote_write). Avoids adding OTLP metrics receiver in Alloy.
- **`k8s.cluster.name` injected at Alloy level** — single config point, applies to all trace sources including Envoy Gateway.
- **Dashboard provisioning via ConfigMap sidecar** — GitOps-friendly, survives pod restarts, version-controlled.
- **High-cardinality dimensions excluded** — `k8s.pod.name` and `client.address` intentionally excluded from metric dimensions to avoid metric explosion.
