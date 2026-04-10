# Alloy Span Kind Transform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Argo Events sensors and EventSources visible in the Grafana Tempo service graph by transforming their span kinds from INTERNAL to CLIENT/SERVER in the Alloy collector pipeline.

**Architecture:** Add an `otelcol.processor.transform` block to the Alloy trace pipeline that rewrites span kinds for Argo Events spans before they reach Tempo. Also enable `enable_virtual_node_label` in Tempo's service graph processor for resilience against unpaired spans.

**Tech Stack:** Grafana Alloy (OTTL transform processor), Grafana Tempo (metrics generator), Helm

**Spec:** `docs/superpowers/specs/2026-04-10-alloy-span-kind-transform-design.md`

---

## File Map

| File | Action | Responsibility |
| --- | --- | --- |
| `deploy/observability/local/alloy-metrics-traces-values.yaml` | Modify | Helm values with inline Alloy config — **this is the deployed config** |
| `deploy/observability/local/alloy-metrics-traces-config.alloy` | Modify | Reference copy of the Alloy config — must stay in sync with values.yaml |
| `deploy/observability/local/tempo-values.yaml` | Modify | Tempo Helm values — add `enable_virtual_node_label` to service graph processor |

---

### Task 1: Add Transform Processor to Alloy Config (Reference File)

**Files:**
- Modify: `deploy/observability/local/alloy-metrics-traces-config.alloy:33-42` (cluster attributes output wiring)

The transform processor slots between the cluster attributes processor and the batch processor. Two changes are needed: (1) rewire the cluster attributes output to feed the transform, and (2) add the transform block that feeds the batch processor.

- [ ] **Step 1: Rewire cluster attributes output to feed transform instead of batch**

In `deploy/observability/local/alloy-metrics-traces-config.alloy`, change the cluster attributes processor output from `otelcol.processor.batch.default.input` to `otelcol.processor.transform.argo_events.input`:

```alloy
// -- Static Cluster Name Attribute --
otelcol.processor.attributes "cluster" {
  action {
    key    = "k8s.cluster.name"
    value  = "bookinfo-local"
    action = "insert"
  }
  output {
    traces = [otelcol.processor.transform.argo_events.input]
  }
}
```

- [ ] **Step 2: Add transform processor block after cluster attributes and before batch**

Insert this new block between the cluster attributes block (line 42) and the batch processor block (line 44) in `deploy/observability/local/alloy-metrics-traces-config.alloy`:

```alloy
// -- Fix Argo Events span kinds for Tempo service graph --
// Workaround: Argo Events PR #3961 sets all spans to INTERNAL.
// Tempo service graph only processes CLIENT/SERVER pairs.
// This transform rewrites known Argo Events span patterns so they
// appear as nodes/edges in the service graph.
// Remove when Argo Events fork ships proper span kinds.
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

  output {
    traces = [otelcol.processor.batch.default.input]
  }
}
```

- [ ] **Step 3: Verify the final pipeline order in the reference file**

Read the file and confirm the pipeline flows:

```
receiver.otlp → processor.k8sattributes → processor.attributes.cluster → processor.transform.argo_events → processor.batch → exporter.otlp.tempo
```

- [ ] **Step 4: Commit reference file changes**

```bash
git add deploy/observability/local/alloy-metrics-traces-config.alloy
git commit -m "feat(observability): add Argo Events span kind transform to Alloy config reference"
```

---

### Task 2: Sync Transform to Alloy Helm Values (Deployed Config)

**Files:**
- Modify: `deploy/observability/local/alloy-metrics-traces-values.yaml:44-54` (cluster attributes output wiring in inline config)

The inline config in the Helm values must match the reference `.alloy` file exactly. Apply the same two changes: rewire cluster output and add the transform block.

- [ ] **Step 1: Rewire cluster attributes output in values.yaml inline config**

In `deploy/observability/local/alloy-metrics-traces-values.yaml`, find the inline cluster attributes block (around line 45-54) and change its output from `otelcol.processor.batch.default.input` to `otelcol.processor.transform.argo_events.input`:

```alloy
      // -- Static Cluster Name Attribute --
      otelcol.processor.attributes "cluster" {
        action {
          key    = "k8s.cluster.name"
          value  = "bookinfo-local"
          action = "insert"
        }
        output {
          traces = [otelcol.processor.transform.argo_events.input]
        }
      }
```

- [ ] **Step 2: Add transform processor block in values.yaml inline config**

Insert the transform block after the cluster attributes block and before the batch processor block in the inline config. The indentation must be 6 spaces (matching the surrounding inline config blocks):

```alloy
      // -- Fix Argo Events span kinds for Tempo service graph --
      // Workaround: Argo Events PR #3961 sets all spans to INTERNAL.
      // Tempo service graph only processes CLIENT/SERVER pairs.
      // This transform rewrites known Argo Events span patterns so they
      // appear as nodes/edges in the service graph.
      // Remove when Argo Events fork ships proper span kinds.
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

        output {
          traces = [otelcol.processor.batch.default.input]
        }
      }
```

- [ ] **Step 3: Verify inline config matches reference file**

Diff the inline config content (between `content: |-` and the next top-level YAML key) against the reference `.alloy` file. They should be identical except for leading indentation.

- [ ] **Step 4: Commit values file changes**

```bash
git add deploy/observability/local/alloy-metrics-traces-values.yaml
git commit -m "feat(observability): sync Argo Events span kind transform to Alloy Helm values"
```

---

### Task 3: Enable Virtual Node Label in Tempo Service Graph

**Files:**
- Modify: `deploy/observability/local/tempo-values.yaml:24-29` (service_graphs processor config)

- [ ] **Step 1: Add `enable_virtual_node_label` to service graph config**

In `deploy/observability/local/tempo-values.yaml`, add `enable_virtual_node_label: true` to the `service_graphs` block. The full block should become:

```yaml
      service_graphs:
        enable_virtual_node_label: true
        histogram_buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
        dimensions:
          - http.method
          - k8s.namespace.name
          - k8s.cluster.name
```

- [ ] **Step 2: Commit Tempo config change**

```bash
git add deploy/observability/local/tempo-values.yaml
git commit -m "feat(observability): enable virtual node label in Tempo service graph"
```

---

### Task 4: Deploy and Verify

**Prerequisites:** The k3d cluster must be running (`make k8s-status` to check). If not running, start it with `make run-k8s`.

- [ ] **Step 1: Redeploy Tempo with updated config**

```bash
helm upgrade --install tempo grafana/tempo \
  -n observability \
  --kube-context=k3d-bookinfo-local \
  -f deploy/observability/local/tempo-values.yaml \
  --wait --timeout 120s
```

Expected: Helm reports `Release "tempo" has been upgraded. Happy Helming!`

- [ ] **Step 2: Redeploy Alloy (metrics+traces) with updated config**

```bash
helm upgrade --install alloy-metrics-traces grafana/alloy \
  -n observability \
  --kube-context=k3d-bookinfo-local \
  -f deploy/observability/local/alloy-metrics-traces-values.yaml \
  --wait --timeout 120s
```

Expected: Helm reports `Release "alloy-metrics-traces" has been upgraded. Happy Helming!`

- [ ] **Step 3: Verify Alloy pod is running with new config**

```bash
kubectl --context=k3d-bookinfo-local -n observability get pods -l app.kubernetes.io/instance=alloy-metrics-traces
```

Expected: Pod in `Running` state with `1/1` ready.

- [ ] **Step 4: Trigger a POST request to generate a trace with Argo Events spans**

```bash
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{"book_id":"1","rating":5}'
```

Expected: `201 Created` or similar success response.

- [ ] **Step 5: Wait for trace propagation and check Tempo**

Wait ~10 seconds for the trace to propagate through the pipeline. Then open Grafana at `http://localhost:3000`, go to Explore > Tempo, and search for recent traces. Find the trace from the POST request.

Verify:
1. The trace still shows all spans correctly linked (no broken parent-child relationships)
2. The `sensor.trigger` span now shows `kind=Client` instead of `kind=Internal`
3. The `eventsource.publish` span now shows `kind=Server` instead of `kind=Internal`

- [ ] **Step 6: Check the service graph**

In Grafana, go to Explore > Tempo > Service Graph. Verify:
1. `*-eventsource` nodes now appear in the graph
2. `*-sensor` nodes now appear in the graph
3. Edges exist: `gateway → eventsource` and `sensor → write-service`

- [ ] **Step 7: Query Prometheus for service graph metrics**

Open Prometheus at `http://localhost:9090` and run:

```promql
traces_service_graph_request_total{client=~".*sensor.*"}
```

Expected: Results showing sensor → write-service edges with request counts.

```promql
traces_service_graph_request_total{server=~".*eventsource.*"}
```

Expected: Results showing gateway → eventsource edges with request counts.

- [ ] **Step 8: Check unpaired spans metric**

```promql
traces_service_graph_unpaired_spans_total{client=~".*sensor.*|.*eventsource.*"}
```

Expected: Zero or very low counts, indicating spans are being paired correctly.
