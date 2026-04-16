# Per-Service PostgreSQL Migration

**Date:** 2026-04-16
**Status:** Approved

## Problem

All 5 postgres-backed services (details, ratings, reviews, notification, dlqueue) share a single PostgreSQL StatefulSet deployed via `deploy/postgres/local/`. While each service already uses its own logical database, they share compute, memory, and storage on one pod. The shared instance's resource limits (500m CPU / 512Mi memory) are oversized for a local dev cluster. Splitting into per-service instances via the Helm chart improves isolation, aligns with microservice ownership, and right-sizes resources.

## Decisions

| Decision | Choice |
|---|---|
| Storage type | StatefulSet + PVC (data survives pod restarts) |
| Implementation | Bitnami PostgreSQL as optional Helm subchart dependency |
| Config wiring | Auto-construct DATABASE_URL, STORAGE_BACKEND, RUN_MIGRATIONS in ConfigMap |
| Resource requests | 25m CPU / 64Mi memory |
| Resource limits | 100m CPU / 128Mi memory |
| PVC size | 256Mi default |
| Shared postgres | Remove `deploy/postgres/` entirely, update Makefile |

## Architecture

### Before

```
┌─────────────────────────────────────────────┐
│           PostgreSQL StatefulSet             │
│         (deploy/postgres/local/)             │
│  500m CPU / 512Mi memory / 1Gi PVC          │
│                                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐    │
│  │ bookinfo │ │ bookinfo │ │ bookinfo │    │
│  │ _details │ │ _ratings │ │ _reviews │    │
│  └──────────┘ └──────────┘ └──────────┘    │
│  ┌──────────────┐ ┌────────────┐           │
│  │  bookinfo    │ │ bookinfo   │           │
│  │ _notification│ │ _dlqueue   │           │
│  └──────────────┘ └────────────┘           │
└─────────────────────────────────────────────┘
```

### After

```
helm install details charts/bookinfo-service -f deploy/details/values-local.yaml
  └── details-postgresql (Bitnami subchart)
      25m/64Mi req, 100m/128Mi lim, 256Mi PVC
      database: bookinfo_details

helm install ratings charts/bookinfo-service -f deploy/ratings/values-local.yaml
  └── ratings-postgresql (Bitnami subchart)
      ...same profile...
      database: bookinfo_ratings

(same pattern for reviews, notification, dlqueue)
```

Each `helm install` deploys both the Go service and its dedicated PostgreSQL instance. Services using memory backend (productpage, ingestion) are unaffected.

**Total resource footprint (5 instances):** 125m CPU / 320Mi memory requests — less than the current single instance (100m/256Mi) by CPU but comparable, and correctly distributed.

## Chart Dependency

Add to `charts/bookinfo-service/Chart.yaml`:

```yaml
dependencies:
  - name: postgresql
    version: "~18"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: postgresql.enabled
```

The `condition: postgresql.enabled` ensures no PostgreSQL resources are created when the toggle is off (default).

## Default Values

New section in `charts/bookinfo-service/values.yaml`:

```yaml
# -- PostgreSQL (Bitnami subchart)
postgresql:
  enabled: false
  auth:
    username: "bookinfo"
    password: "bookinfo"
    database: ""
  primary:
    resources:
      requests:
        cpu: 25m
        memory: 64Mi
      limits:
        cpu: 100m
        memory: 128Mi
    persistence:
      enabled: true
      size: 256Mi
```

Disabled by default. Each service's `values-local.yaml` enables it and sets `auth.database`.

## ConfigMap Auto-Wiring

In `templates/configmap.yaml`, when `postgresql.enabled` is true, auto-inject:

```yaml
{{- if .Values.postgresql.enabled }}
STORAGE_BACKEND: "postgres"
RUN_MIGRATIONS: "true"
DATABASE_URL: "postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}@{{ .Release.Name }}-postgresql:5432/{{ .Values.postgresql.auth.database }}?sslmode=disable"
{{- end }}
```

Placed **before** the `config:` range loop so explicit `config:` values take precedence (last write wins in ConfigMap data).

## Values-Local.yaml Simplification

### Before (example: ratings)

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_ratings?sslmode=disable"
  RUN_MIGRATIONS: "true"
```

### After

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_ratings"

config:
  LOG_LEVEL: "debug"
```

**Services updated (5):** details, ratings, reviews, notification, dlqueue
**Services unchanged (2):** productpage, ingestion

## Values Schema Update

Add `postgresql` to `values.schema.json`:

```json
"postgresql": {
  "type": "object",
  "properties": {
    "enabled": { "type": "boolean" },
    "auth": {
      "type": "object",
      "properties": {
        "username": { "type": "string" },
        "password": { "type": "string" },
        "database": { "type": "string" }
      }
    },
    "primary": {
      "type": "object",
      "properties": {
        "resources": { "type": "object" },
        "persistence": {
          "type": "object",
          "properties": {
            "enabled": { "type": "boolean" },
            "size": { "type": "string" }
          }
        }
      }
    }
  }
}
```

Only validates fields we explicitly document. Bitnami's additional values pass through without schema enforcement.

## Shared PostgreSQL Removal

### Delete

- `deploy/postgres/local/` (statefulset.yaml, service.yaml, init-configmap.yaml, kustomization.yaml)

### Makefile Updates

1. **Remove shared postgres deploy step** — delete `kubectl apply -k deploy/postgres/local/` and the associated `kubectl wait` for `statefulset/postgres`. Renumber remaining steps.

2. **Update seed commands** — retarget to per-service postgres pods:
   ```bash
   # Before:
   kubectl exec statefulset/postgres -- psql -U bookinfo -d bookinfo_$$svc ...

   # After:
   kubectl exec statefulset/$$svc-postgresql -- psql -U bookinfo -d bookinfo_$$svc ...
   ```

3. **`clean-data` target** — no k8s changes needed; PVCs are per-service and cleaned up with `make stop-k8s` (k3d cluster delete).

## CI Test Values

New file `charts/bookinfo-service/ci/values-details-postgres.yaml`:

```yaml
# ct install test: PostgreSQL subchart enabled
serviceName: details
fullnameOverride: details
image:
  repository: event-driven-bookinfo/details
  tag: latest

postgresql:
  enabled: true
  auth:
    database: "bookinfo_details"
```

Ensures `ct lint` and `ct install` validate the postgres-enabled path.

## Validation & Delivery

1. **`make stop-k8s`** — tear down existing cluster for a clean slate
2. **`make run-k8s`** — stand up full cluster with per-service postgres (validates helm install with subchart)
3. **Manual smoke tests** — POST/GET through Gateway to each postgres-backed service (details, ratings, reviews, notification, dlqueue):
   - Migrations ran successfully
   - Writes persist (POST then GET roundtrip)
   - CQRS routing works (POST -> EventSource -> Sensor -> write pod -> read pod serves data)
4. **k6 load test** — run k6 tests to validate under concurrent load
5. **Grafana validation** — check dashboards for:
   - No error-rate spikes in service metrics
   - Traces flowing through Tempo (distributed tracing intact)
   - No postgres connection errors in Loki logs
6. **Open PR** — against `main` with all changes
7. **PR checks green** — wait for CI (helm-lint-test, golangci-lint, go test, ct lint/install) to pass; fix any failures
