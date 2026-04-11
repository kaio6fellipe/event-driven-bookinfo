# Pyroscope Continuous Profiling with Trace Correlation

## Overview

Deploy Pyroscope as the continuous profiling backend and enable trace-to-profile correlation using `grafana/otel-profiling-go`. This completes the observability stack: metrics (Prometheus), logs (Loki), traces (Tempo), and now profiles (Pyroscope).

## Approach

Push-based profiling using the existing `grafana/pyroscope-go` SDK. The `grafana/otel-profiling-go` package wraps the OTel TracerProvider to automatically label profiling samples with span IDs, enabling Grafana to link Tempo traces directly to Pyroscope profiles.

## Section 1: Pyroscope Server Deployment

### Helm Chart

- **Chart:** `grafana/pyroscope` (monolithic / single-binary mode)
- **Namespace:** `observability` (alongside Prometheus, Grafana, Tempo, Loki)
- **Values file:** `deploy/observability/local/pyroscope-values.yaml`

### Configuration

Minimal monolithic config for local dev:
- Local filesystem storage (matching Tempo pattern)
- Small resource limits appropriate for k3d
- Default port 4040
- No authentication (local dev)

### Makefile

Add Pyroscope as a numbered step in `k8s-observability` target, between Loki and Alloy. Bump subsequent step numbers. Uses the same `$(HELM) upgrade --install` pattern as all other observability components.

## Section 2: Go SDK Changes — Trace-to-Profile Correlation

### New Dependency

`github.com/grafana/otel-profiling-go` — wraps the OTel TracerProvider to inject `span_id` and `profile_id` labels into profiling samples when a span is active.

### `pkg/telemetry/telemetry.go`

After creating the TracerProvider, conditionally wrap it with `otelpyroscope.NewTracerProvider(tp)` before calling `otel.SetTracerProvider()`. The wrap is conditional on `PYROSCOPE_SERVER_ADDRESS` being set — no point wrapping if profiling is disabled.

The function signature changes to accept a `pyroscopeEnabled bool` parameter so it can decide whether to apply the wrapper. The caller checks `cfg.PyroscopeServerAddress != ""` and passes the result. This keeps telemetry decoupled from the config package.

### `pkg/profiling/profiling.go`

Add support for `Tags` in the Pyroscope config. Tags are passed as a `map[string]string` populated from environment variables:
- `service_name` — already set via `ApplicationName`
- `namespace` — from `POD_NAMESPACE` env var (Kubernetes downward API)
- `pod` — from `POD_NAME` env var (Kubernetes downward API)

The function signature changes to accept tags (or reads them from env vars directly).

### Service `cmd/main.go` Files

No changes needed. All 5 services already call `telemetry.Setup()` then `profiling.Start()` in the correct order. The TracerProvider wrapping happens inside `telemetry.Setup`, and profiling samples are automatically labeled by the wrapped provider.

### End-to-End Flow

1. `telemetry.Setup` creates OTel TracerProvider, wraps with `otelpyroscope.NewTracerProvider` (if Pyroscope enabled)
2. `profiling.Start` begins pushing continuous profiles to Pyroscope server
3. When a span is active, `otel-profiling-go` injects `span_id` and `profile_id` labels into profiling data
4. Pyroscope stores profiles with these labels plus pod-level tags
5. Grafana queries Pyroscope by span ID when viewing a trace span

## Section 3: Kubernetes Configuration Changes

### ConfigMap Patches

All 5 services in `deploy/<service>/overlays/local/configmap-patch.yaml` — add:
```
PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```
Same pattern as `OTEL_EXPORTER_OTLP_ENDPOINT`.

### Downward API Environment Variables

All deployment manifests (base + write overlays) — add `POD_NAME` and `POD_NAMESPACE` via Kubernetes downward API `fieldRef`:
```yaml
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
```

### Remove Pull-Based Annotations

Delete the `profiles.grafana.com/*` annotations from all deployment manifests (5 base deployments + 3 write overlays). Not needed with push-based approach.

## Section 4: Grafana Datasource & Trace-to-Profile Linking

### Pyroscope Datasource

Add to `kube-prometheus-stack-values.yaml` under `grafana.additionalDataSources`:
```yaml
- name: Pyroscope
  type: grafana-pyroscope-datasource
  uid: pyroscope
  url: http://pyroscope.observability.svc.cluster.local:4040
  access: proxy
  isDefault: false
```

### Tempo `tracesToProfiles` Configuration

Add to the existing Tempo datasource `jsonData` block in `kube-prometheus-stack-values.yaml`, alongside `tracesToLogsV2` and `tracesToMetrics`:
```yaml
tracesToProfilesV2:
  datasourceUid: pyroscope
  profileTypeId: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
  customQuery: true
  query: '{service_name="${__span.tags["service.name"]}"}'
  tags:
    - key: "service.name"
      value: "service_name"
    - key: "k8s.namespace.name"
      value: "namespace"
    - key: "k8s.pod.name"
      value: "pod"
```

Tags map span attributes (enriched by Alloy's k8sattributes processor) to Pyroscope labels. More tags = tighter filter = faster Pyroscope queries. The `span_id` correlation from `otel-profiling-go` provides the precise span-level link.

### CLAUDE.md

Update access URLs and project overview to reflect Pyroscope as part of the observability stack.

## Files Changed

| File | Change |
|---|---|
| `deploy/observability/local/pyroscope-values.yaml` | New — Helm values for Pyroscope |
| `deploy/observability/local/kube-prometheus-stack-values.yaml` | Add Pyroscope datasource + tracesToProfiles on Tempo |
| `Makefile` | Add Pyroscope Helm install step in k8s-observability |
| `go.mod` / `go.sum` | Add `grafana/otel-profiling-go` dependency |
| `pkg/telemetry/telemetry.go` | Wrap TracerProvider with otelpyroscope (conditional) |
| `pkg/profiling/profiling.go` | Add Tags support (namespace, pod from env vars) |
| `pkg/profiling/profiling_test.go` | Update tests for tags |
| `deploy/*/overlays/local/configmap-patch.yaml` (x5) | Add PYROSCOPE_SERVER_ADDRESS |
| `deploy/*/base/deployment.yaml` (x5) | Remove profiles.grafana.com annotations, add downward API env vars |
| `deploy/*/overlays/local/deployment-write.yaml` (x3) | Remove profiles.grafana.com annotations, add downward API env vars |
| `CLAUDE.md` | Update observability description |
