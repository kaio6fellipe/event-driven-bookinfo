# k6 Load Generator for Service Graph and Observability

## Problem

The Tempo service graph requires continuous trace data to maintain stable edges between services. In the local k8s environment, low traffic causes Tempo's metrics generator to reset counters between Prometheus scrape intervals, resulting in intermittent or missing edges (e.g., `gateway -> eventsource` appearing and disappearing).

The current `make k8s-traffic` target sends a handful of one-shot requests — insufficient for a consistently populated service graph.

## Goal

Replace `make k8s-traffic` with a k6-based load generator that:
1. Runs locally via Docker for quick ad-hoc load (`make k8s-load`, default 30s)
2. Runs continuously inside the cluster via k6 Operator CronJob for always-populated observability data
3. Produces aleatory (variable) traffic volume to create realistic patterns without overwhelming services
4. Pushes k6 metrics to Prometheus for a dedicated Grafana dashboard

## Scope

- k6 test script exercising all productpage routes (read + write paths)
- Docker-based local runner (Makefile target)
- k6 Operator deployment with CronJob for sustained background load
- k6 Grafana dashboard
- Remove existing `make k8s-traffic` target

## Design

### k6 Test Script (`test/k6/bookinfo-traffic.js`)

A single script used by both the local Docker runner and the in-cluster operator. Exercises the full application flow through productpage routes only (no direct `/v1` API calls):

**Read path (every iteration):**
- `GET /` — home page (productpage -> details fan-out)
- `GET /products/{id}` — product page (productpage -> details + reviews + ratings fan-out)
- `GET /partials/details/{id}` — HTMX partial (productpage -> details)
- `GET /partials/reviews/{id}` — HTMX partial (productpage -> reviews -> ratings)

**Write path (every iteration):**
- `POST /partials/rating` — submit rating + review (productpage -> gateway -> EventSource -> Sensor -> write services)

The script discovers a real product ID from the home page response (same approach as the current `make k8s-traffic`), falling back gracefully if no products exist.

**Executor:** `ramping-arrival-rate` with stages that cycle through variable request rates. The base rate and duration are configurable via environment variables:

| Env var | Default (local) | Default (cluster) | Description |
| --- | --- | --- | --- |
| `BASE_URL` | `http://host.docker.internal:8080` | `http://gateway.envoy-gateway-system.svc.cluster.local` | Target URL |
| `DURATION` | `30s` | `5m` | Total test duration |
| `BASE_RATE` | `2` | `1` | Base request rate per second |

Stages cycle through: base rate -> 2x base -> base -> 0.5x base -> 3x base spike -> base. This produces aleatory volume while keeping average throughput predictable.

**Thresholds:** The script includes k6 thresholds (p95 < 2s, error rate < 10%) for basic health validation, but failures don't block — this is load generation, not acceptance testing.

### Local Docker Runner (`make k8s-load`)

Runs k6 via the `grafana/k6` Docker image. Replaces `make k8s-traffic`.

```bash
make k8s-load                    # 30s default, 2 req/s base rate
DURATION=5m make k8s-load        # 5 minutes
BASE_RATE=5 make k8s-load        # higher rate
```

The Makefile target:
- Mounts `test/k6/bookinfo-traffic.js` into the container
- Sets `BASE_URL=http://host.docker.internal:8080` (Docker host networking)
- Sets `K6_PROMETHEUS_RW_SERVER_URL` to push metrics to local Prometheus (`http://host.docker.internal:9090/api/v1/write`)
- Uses `-o experimental-prometheus-rw` output for Prometheus metrics
- Passes `DURATION` and `BASE_RATE` as environment variables

### k6 Operator (Continuous In-Cluster Load)

**Operator deployment:** Add k6-operator Helm chart to `make k8s-observability` (it's an observability concern, not a platform concern). Deployed in the `observability` namespace.

**CronJob approach:** A Kubernetes CronJob creates a k6 TestRun every 10 minutes. Each run lasts 5 minutes with a low base rate (~0.5-1 req/s). This produces:
- ~150-300 requests per 5-minute run
- Natural 5-minute gaps between runs (storage breathing room)
- Self-healing: if a run fails, the next CronJob iteration starts fresh
- Average ~15-30 req/min sustained, producing enough traces for a stable service graph without overwhelming log/trace/metric storage

**Resource estimates per run (conservative):**
- Traces: ~150-300 traces x ~10 spans each = ~1500-3000 spans per run
- Logs: minimal (k6 pods are ephemeral)
- Metrics: k6 push interval 5s, ~60 data points per metric per run

**Deploy structure:**
```
deploy/k6/
  base/
    configmap.yaml          # k6 script as ConfigMap
    cronjob.yaml            # CronJob that creates TestRun-like Jobs
  overlays/local/
    kustomization.yaml      # Local patches (namespace, URL, rate)
```

Note: Since the k6 Operator's TestRun CRD is designed for one-off runs and doesn't support CronJob natively, we use a standard Kubernetes CronJob that runs the `grafana/k6` image directly (not a TestRun CRD). This avoids the operator dependency entirely while achieving the same result: periodic k6 runs inside the cluster.

Actually, this simplifies the design — **we don't need the k6 Operator at all**. A plain CronJob running the `grafana/k6` image with the script mounted from a ConfigMap achieves the same thing with zero operator overhead. The script ConfigMap is shared between the CronJob and the local Docker runner.

### k6 Prometheus Metrics

k6 pushes metrics to Prometheus via remote write (`-o experimental-prometheus-rw`). Both the local Docker runner and the in-cluster CronJob push to the same Prometheus instance.

Key k6 metrics available in Prometheus:
- `k6_http_req_duration` — request latency histogram
- `k6_http_reqs` — total request count
- `k6_http_req_failed` — failure rate
- `k6_iterations` — completed iterations
- `k6_vus` — active virtual users

### k6 Grafana Dashboard

Add a k6 dashboard to `deploy/observability/local/dashboards/k6-load-testing.json`. Use the official Grafana community dashboard for k6 + Prometheus remote write (dashboard ID 19665) as a base, customized if needed.

The dashboard shows: request rate, latency percentiles, error rate, active VUs, and iteration throughput — giving visibility into the load generator's behavior alongside the application's observability stack.

## File Map

| File | Action | Purpose |
| --- | --- | --- |
| `test/k6/bookinfo-traffic.js` | Create | k6 test script |
| `deploy/k6/base/configmap.yaml` | Create | Script as ConfigMap for in-cluster CronJob |
| `deploy/k6/base/cronjob.yaml` | Create | CronJob running k6 every 10 minutes |
| `deploy/k6/overlays/local/kustomization.yaml` | Create | Local overlay (namespace, URL) |
| `deploy/observability/local/dashboards/k6-load-testing.json` | Create | Grafana k6 dashboard |
| `Makefile` | Modify | Replace `k8s-traffic` with `k8s-load`, add `k8s-load-start`/`k8s-load-stop` for CronJob |

## Makefile Targets

| Target | Description |
| --- | --- |
| `make k8s-load` | Run k6 locally via Docker (default 30s). Configurable: `DURATION=5m BASE_RATE=3 make k8s-load` |
| `make k8s-load-start` | Deploy the k6 CronJob to the cluster for continuous background load |
| `make k8s-load-stop` | Remove the k6 CronJob from the cluster |

## Verification

1. `make k8s-load` runs k6 via Docker, shows k6 output with request stats
2. k6 metrics appear in Prometheus (`k6_http_req_duration`, `k6_http_reqs`)
3. k6 Grafana dashboard shows request rate and latency
4. After `make k8s-load-start`, CronJob creates pods every 10 minutes
5. Service graph in Grafana shows all edges consistently (no more intermittent nodes)
6. After `make k8s-load-stop`, CronJob is removed and load stops
