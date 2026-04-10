# Alloy Span Kind Transform for Argo Events Service Graph Visibility

## Problem

Argo Events (PR #3961) instruments EventSources and Sensors with OpenTelemetry tracing, but all spans use `SPAN_KIND_INTERNAL`. Tempo's service graph processor only builds edges from `CLIENT`/`SERVER` (and optionally `PRODUCER`/`CONSUMER`) span pairs, so Argo Events components are invisible in the Grafana Tempo service graph despite appearing correctly in individual trace views.

**Current state:** The service graph shows `gateway -> write-services` but the intermediary Argo Events components (`eventsource`, `sensor`) are missing.

**Desired state:** The service graph shows `gateway -> eventsource` and `sensor -> write-service` edges, making the full request flow visible.

## Scope

Infrastructure-only change — no application code modifications. Two files affected:

1. `deploy/observability/local/alloy-metrics-traces-config.alloy` — add transform processor
2. `deploy/observability/local/tempo-values.yaml` — add virtual node support for unpaired spans

## Design

### Span Kind Transform in Alloy

Add an `otelcol.processor.transform` block to the Alloy trace pipeline that rewrites span kinds for known Argo Events span patterns:

| Span name pattern | Current kind | New kind | Rationale |
|---|---|---|---|
| `sensor.trigger*` | Internal | Client | Sensor makes outbound HTTP call to write service; pairs with write service's SERVER span |
| `eventsource.publish*` | Internal | Server | EventSource handles inbound webhook from gateway; pairs with gateway's CLIENT span |

**Why `eventsource.publish` -> Server:** This span represents the EventSource processing an inbound webhook and publishing to the EventBus. Semantically it's a PRODUCER (publishing to Kafka/JetStream), but there's no matching CONSUMER span on the sensor side (PR #3961 doesn't create one). Setting it to Server pairs with the gateway's CLIENT span for the webhook call, creating a `gateway -> eventsource` edge. This is a pragmatic workaround until the Argo Events fork adds proper PRODUCER/CONSUMER spans.

**Pattern matching over hardcoded names:** The transform matches on span name patterns (`sensor.trigger`, `eventsource.publish`) and the Argo Events library names (`argo-events-sensor`, `argo-events-eventsource`), not on individual service names. Any new sensor or EventSource automatically gets the correction.

### Pipeline Insertion Point

The transform slots between the cluster attributes processor and the batch processor:

```
OTLP Receiver -> K8s Attributes -> Cluster Name -> [Transform] -> Batch -> Tempo
```

This ensures K8s metadata is already enriched before the transform runs, allowing conditions to reference resource attributes if needed.

### Alloy Configuration

```alloy
// -- Fix Argo Events span kinds for Tempo service graph --
otelcol.processor.transform "argo_events" {
  error_mode = "ignore"

  trace_statements {
    context = "span"
    conditions = [
      `IsMatch(resource.attributes["service.name"], ".*-sensor$")`,
    ]
    statements = [
      `set(span.kind.string, "Client") where IsMatch(name, "^sensor\\.trigger")`,
    ]
  }

  trace_statements {
    context = "span"
    conditions = [
      `IsMatch(resource.attributes["service.name"], ".*-eventsource$")`,
    ]
    statements = [
      `set(span.kind.string, "Server") where IsMatch(name, "^eventsource\\.publish")`,
    ]
  }
}
```

**Key syntax details:**
- `span.kind.string` with title-case values ("Client", "Server") — the `SPAN_KIND_*` constants are deprecated in OTTL
- `conditions` pre-filter by service name pattern so statements only evaluate on Argo Events spans
- `error_mode = "ignore"` ensures non-matching spans pass through unmodified

### Tempo Configuration Enhancement

Add `enable_virtual_node_label` to the service graph processor so unpaired spans (e.g., a sensor CLIENT span where the write service is temporarily down) still appear as nodes with a "virtual_node" label rather than being silently dropped:

```yaml
service_graphs:
  enable_virtual_node_label: true
  histogram_buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
  dimensions:
    - http.method
    - k8s.namespace.name
    - k8s.cluster.name
```

## Resulting Service Graph Edges

After this change, the service graph will show:

| Client | Server | Created by |
|---|---|---|
| `default-gw` (Envoy Gateway) | `*-eventsource` | Gateway CLIENT + eventsource "Server" |
| `*-sensor` | `*-write` services | Sensor "Client" + write service SERVER |
| `productpage` | `details-read`, `reviews-read` | Existing (unchanged) |
| `reviews-read` | `ratings-read` | Existing (unchanged) |

**Not visible:** The `eventsource -> sensor` edge (Kafka/EventBus link) — this requires PRODUCER/CONSUMER spans with messaging attributes, which is a separate Argo Events code change.

## Lifecycle

This transform is a **workaround** with a planned deprecation path. When the Argo Events fork adds proper span kinds (CLIENT for sensor triggers, SERVER for webhook receive, PRODUCER/CONSUMER for EventBus messaging), the Alloy transform conditions will stop matching (the spans will no longer be INTERNAL) and can be removed. The transform is idempotent — it only acts on spans that are currently INTERNAL.

## Verification

After deploying, verify by:

1. Trigger a POST request through the gateway (e.g., submit a review)
2. Find the trace in Tempo — confirm spans still appear correctly linked
3. Check the service graph in Grafana — confirm `*-eventsource` and `*-sensor` nodes now appear
4. Query Prometheus for `traces_service_graph_request_total` — confirm new client/server label pairs exist
5. Check `traces_service_graph_unpaired_spans_total` — should be low/zero for Argo Events spans
