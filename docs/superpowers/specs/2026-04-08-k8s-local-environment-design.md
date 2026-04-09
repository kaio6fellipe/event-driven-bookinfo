# Local Kubernetes Development Environment Design

**Date:** 2026-04-08
**Status:** Approved

## Overview

A complete local Kubernetes development environment for the event-driven bookinfo monorepo, accessed via `make run-k8s`. Deploys all 5 microservices with CQRS-style read/write deployment separation, event-driven architecture (Argo Events + Kafka), full observability stack (Prometheus, Grafana, Tempo, Loki, Alloy), and Envoy Gateway API for traffic routing — all on a k3d cluster.

## Namespace Layout

| Namespace | Components |
|---|---|
| **platform** | Strimzi operator, Kafka (KRaft single-node), Argo Events controller, EventBus (Kafka-backed), Gateway `default-gw` |
| **envoy-gateway-system** | Envoy Gateway controller, GatewayClass |
| **observability** | kube-prometheus-stack (Prometheus + Grafana), Tempo, Loki, Alloy (logs DaemonSet + metrics/traces Deployment) |
| **bookinfo** | 8 app deployments, PostgreSQL StatefulSet, 3 EventSources, 3 Sensors, HTTPRoutes, ReferenceGrant |

## k3d Cluster

- **Name:** `bookinfo-local`
- **Distribution:** k3d (k3s in Docker)
- **Port mappings (at cluster creation):**
  - `8080:80@loadbalancer` — Envoy Gateway web listener (productpage)
  - `8443:443@loadbalancer` — Envoy Gateway webhook listener (EventSources)
  - `3000:30300@server:0` — Grafana (NodePort)
  - `9090:30900@server:0` — Prometheus (NodePort)
- **Image loading:** `k3d image import` (no local registry)
- **Context:** `k3d-bookinfo-local` — all kubectl/helm calls use `--context` flag, never mutate user's active context

## CQRS Deployment Split

Each backend service runs as independent read and write deployments (same image, separate k8s Deployments and Services). This enables independent scaling of read and write paths.

| Service | Read Deployment | Write Deployment | K8s Service Names |
|---|---|---|---|
| **productpage** | Yes (BFF, read-only) | No | `productpage` |
| **details** | Yes | Yes | `details` / `details-write` |
| **reviews** | Yes | Yes | `reviews` / `reviews-write` |
| **ratings** | Yes | Yes | `ratings` / `ratings-write` |
| **notification** | No | Yes (write-only) | `notification` |

**Total: 8 app deployments**

### Read Path (sync)

```
Browser → localhost:8080 → Envoy Gateway → HTTPRoute → productpage
  productpage → details:80      (GET /v1/details/*)
  productpage → reviews:80      (GET /v1/reviews/*)
    reviews   → ratings:80      (GET /v1/ratings/*)
```

### Write Path (event-driven)

```
Webhook → localhost:8443 → Envoy Gateway → HTTPRoute → EventSource
  EventSource → Kafka (platform) → Sensor → HTTP trigger
    book-added     → details-write:80   (POST /v1/details)
                   → notification:80    (POST /v1/notifications)
    review-submitted → reviews-write:80 (POST /v1/reviews)
                     → notification:80  (POST /v1/notifications)
    rating-submitted → ratings-write:80 (POST /v1/ratings)
                     → notification:80  (POST /v1/notifications)
```

## Envoy Gateway API

### Controller (envoy-gateway-system)

- **Helm chart:** `envoyproxy/gateway-helm` v1.7.0
- **Creates:** GatewayClass `eg`

### Gateway (platform)

- **Resource:** `Gateway` named `default-gw` in `platform` namespace
- **GatewayClass:** `eg`
- **Listeners:**
  - `web` — port 80 (HTTP), protocol HTTP
  - `webhooks` — port 443, protocol HTTP (plain HTTP for local dev, no TLS termination)

### HTTPRoutes (bookinfo)

All HTTPRoutes live in `bookinfo` namespace with cross-namespace parent reference:

```yaml
parentRefs:
  - name: default-gw
    namespace: platform
```

| HTTPRoute | Path | Backend Service |
|---|---|---|
| `productpage` | `/` | `productpage:80` |
| `book-added-webhook` | `/v1/book-added` | `book-added-eventsource-svc:12000` |
| `review-submitted-webhook` | `/v1/review-submitted` | `review-submitted-eventsource-svc:12001` |
| `rating-submitted-webhook` | `/v1/rating-submitted` | `rating-submitted-eventsource-svc:12002` |

### ReferenceGrant (bookinfo)

Required to allow HTTPRoutes in `bookinfo` to reference the Gateway in `platform`:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-platform-gateway
  namespace: platform
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: bookinfo
  to:
    - group: gateway.networking.k8s.io
      kind: Gateway
```

## Platform Components

### Strimzi Kafka (platform)

- **Strimzi operator:** Helm chart `strimzi/strimzi-kafka-operator`
- **Kafka cluster:** KRaft mode, single combined controller+broker node
- **KafkaNodePool:** `dual-role`, 1 replica, roles: [controller, broker]
- **Listeners:** plain (9092, internal, no TLS)
- **Config:** replication factor 1, min ISR 1 (single-node safe)
- **Storage:** persistent-claim, small volume (1Gi for local dev)

### Argo Events (platform + bookinfo)

- **Controller:** Helm chart `argo-events` in `platform` namespace, watches all namespaces
- **EventBus:** Kafka-backed, connects to Strimzi Kafka at `my-cluster-kafka-bootstrap.platform.svc.cluster.local:9092`
- **EventSources:** 3 webhook EventSources in `bookinfo` (book-added :12000, review-submitted :12001, rating-submitted :12002)
- **Sensors:** 3 sensors in `bookinfo` with HTTP triggers pointing to `-write` services

### PostgreSQL (bookinfo)

- **Type:** Simple StatefulSet (no operator)
- **Image:** `postgres:17-alpine`
- **Storage:** PVC (1Gi)
- **Init script:** Reuses existing `dev/init-databases.sql` for multi-database setup
- **Seed data:** Reuses existing `services/*/seeds/seed.sql`
- **Service:** `postgres:5432` in `bookinfo` namespace

## Observability Stack

### Grafana Alloy — Two Instances

**alloy-logs (DaemonSet):**
- One pod per node
- Mounts `/var/log/pods` from host
- Collects container logs via `loki.source.kubernetes`
- Enriches with k8s metadata (namespace, pod, container, app)
- Forwards to Loki via `loki.write`

**alloy-metrics-traces (Deployment, 1 replica):**
- OTLP gRPC receiver on `:4317` — apps send traces here
- Scrapes app admin ports (`:9090/metrics`) via `prometheus.scrape` with k8s service discovery
- Enriches telemetry with k8s attributes via `otelcol.processor.k8sattributes`
- Forwards traces to Tempo via `otelcol.exporter.otlp`
- Forwards metrics to Prometheus via `prometheus.remote_write`

### Prometheus (StatefulSet)

- **Chart:** `prometheus-community/kube-prometheus-stack`
- **Mode:** Single replica
- **Receives:** remote_write from Alloy + scrapes own ServiceMonitors (kube-state-metrics, node-exporter, kubelet)
- **Storage:** PVC
- **Exposed:** NodePort 30900 → localhost:9090

### Grafana (Deployment)

- **Chart:** bundled with kube-prometheus-stack
- **Datasources (auto-provisioned):**
  - Prometheus: `http://prometheus-kube-prometheus-prometheus.observability.svc.cluster.local:9090`
  - Tempo: `http://tempo.observability.svc.cluster.local:3100`
  - Loki: `http://loki.observability.svc.cluster.local:3100`
- **Exposed:** NodePort 30300 → localhost:3000

### Tempo (StatefulSet, monolithic)

- **Chart:** `grafana/tempo`
- **Mode:** Single binary (monolithic)
- **Receives:** OTLP gRPC from Alloy
- **Storage:** local filesystem (PVC, no MinIO)

### Loki (StatefulSet, SingleBinary)

- **Chart:** `grafana/loki`
- **Mode:** SingleBinary (deploymentMode: SingleBinary)
- **Receives:** log streams from Alloy DaemonSet
- **Storage:** local filesystem (PVC, no MinIO)

### App Configuration

Services deployed in the `local` overlay set these environment variables:

```
OTEL_EXPORTER_OTLP_ENDPOINT=alloy-metrics-traces.observability.svc.cluster.local:4317
STORAGE_BACKEND=postgres
DATABASE_URL=postgres://bookinfo:<password>@postgres.bookinfo.svc.cluster.local:5432/<service>_db?sslmode=disable
```

`PYROSCOPE_SERVER_ADDRESS` is left empty — profiling data is scraped by Grafana via pod annotations on `:9090` (already present in base deployments).

## Kustomize Overlay Structure

### New `local` Overlay Per Service

```
deploy/<service>/overlays/local/
├── kustomization.yaml        # extends base, adds namespace, read/write split
├── deployment-read.yaml      # patches base deployment with role: read label
├── deployment-write.yaml     # new write deployment resource
├── service-write.yaml        # new ClusterIP svc: <service>-write
└── configmap-patch.yaml      # postgres + OTLP env vars
```

**Exceptions:**
- `productpage/overlays/local/` — no write deployment (read-only BFF)
- `notification/overlays/local/` — no read deployment (write-only consumer)

### Argo Events Local Overlay

```
deploy/argo-events/overlays/local/
├── kustomization.yaml
├── eventbus.yaml             # Kafka EventBus pointing to platform Kafka
├── eventsources/             # same 3 EventSources, namespaced to bookinfo
└── sensors/                  # sensors with -write service targets
```

Sensor trigger URLs change from `http://details/v1/details` to `http://details-write.bookinfo.svc.cluster.local/v1/details`.

### Gateway API Manifests

```
deploy/gateway/
├── base/
│   ├── gateway.yaml          # Gateway: default-gw
│   ├── reference-grant.yaml  # ReferenceGrant for cross-NS
│   └── kustomization.yaml
└── overlays/local/
    ├── kustomization.yaml
    └── httproutes.yaml       # all 4 HTTPRoutes for bookinfo
```

## Make Targets

### Orchestrator

```
make run-k8s          # calls all layers in order with health gates
make stop-k8s         # deletes k3d cluster
make k8s-status       # prints pod status + URLs
make k8s-logs         # tails bookinfo namespace logs
make k8s-rebuild      # fast iteration: rebuild images + reimport + rollout restart
```

### Layered Targets (each independently callable)

```
make k8s-cluster      # create k3d cluster with port mappings
                      # health: k3d cluster list + kubectl cluster-info

make k8s-platform     # install Strimzi operator, Kafka cluster, Argo Events,
                      # EventBus, Envoy Gateway, Gateway default-gw
                      # health: wait for operator pods + Kafka Ready + Gateway programmed

make k8s-observability # install kube-prometheus-stack, Tempo, Loki, Alloy (x2)
                       # health: wait for all pods Ready in observability NS

make k8s-deploy       # build images, k3d image import, create bookinfo NS,
                      # apply Postgres StatefulSet, run migrations + seeds,
                      # apply Kustomize local overlays for all services,
                      # apply Argo Events local overlays, apply HTTPRoutes
                      # health: wait for all deployments Ready + /healthz checks
```

### Context Safety

- Make variable: `K8S_CONTEXT := k3d-bookinfo-local`
- All `kubectl` calls: `kubectl --context=$(K8S_CONTEXT)`
- All `helm` calls: `helm --kube-context=$(K8S_CONTEXT)`
- **Context validation guard** at the start of every k8s target:
  1. Check `K8S_CONTEXT` resolves to a real context
  2. Verify it matches the `k3d-*` pattern (refuse non-k3d contexts)
  3. Fail fast with clear error message

### Idempotency

All targets are safe to re-run:
- `k8s-cluster`: checks if cluster exists before creating
- `k8s-platform`/`k8s-observability`: `helm upgrade --install`
- `k8s-deploy`: `kubectl apply` is inherently idempotent

## Host Access Summary

| URL | Service |
|---|---|
| `http://localhost:8080` | Productpage (via Envoy Gateway) |
| `http://localhost:8443/v1/book-added` | Book-added webhook (via Envoy Gateway) |
| `http://localhost:8443/v1/review-submitted` | Review-submitted webhook (via Envoy Gateway) |
| `http://localhost:8443/v1/rating-submitted` | Rating-submitted webhook (via Envoy Gateway) |
| `http://localhost:3000` | Grafana (dashboards, explore) |
| `http://localhost:9090` | Prometheus (query, targets) |

## Helm Charts Summary

| Component | Chart | Namespace | Version |
|---|---|---|---|
| Envoy Gateway | `envoyproxy/gateway-helm` | envoy-gateway-system | v1.7.0 |
| Strimzi Operator | `strimzi/strimzi-kafka-operator` | platform | latest stable |
| Argo Events | `argo/argo-events` | platform | latest stable |
| kube-prometheus-stack | `prometheus-community/kube-prometheus-stack` | observability | latest stable |
| Tempo | `grafana/tempo` | observability | latest stable |
| Loki | `grafana/loki` | observability | latest stable |
| Alloy (logs) | `grafana/alloy` | observability | latest stable |
| Alloy (metrics+traces) | `grafana/alloy` | observability | latest stable |

## Prerequisites

- Docker Desktop running
- `k3d` CLI installed
- `kubectl` CLI installed
- `helm` CLI installed
- Sufficient resources: ~8GB RAM recommended (Kafka ~512MB, Postgres ~256MB, observability ~2GB, apps ~1GB, k3s ~512MB)
