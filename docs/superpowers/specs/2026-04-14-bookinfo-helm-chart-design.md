# Bookinfo Service Helm Chart — Design Specification

**Date:** 2026-04-14
**Status:** Draft
**Scope:** Single reusable Helm chart for all 6 bookinfo services, replacing Kustomize base+overlays

## Overview

Create a single `bookinfo-service` Helm chart that eliminates the ~95% manifest duplication across the 6 application services. The chart supports CQRS read/write split deployments, Argo Events EventSource/Sensor generation with DLQ auto-wiring, and per-environment values files. Published to GitHub Pages via chart-releaser-action.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Single chart vs per-service | Single reusable chart | 95% structural duplication across services; one place to fix bugs |
| CQRS modeling | Boolean toggle (`cqrs.enabled`) with template conditionals | Simple values, generates distinct read/write Deployments when enabled |
| EventSource modeling | Templated from `cqrs.endpoints` | Fully parameterizable — name, port, method, endpoint follow clear patterns |
| Sensor modeling | Templated triggers with raw payload support | Triggers defined explicitly in values; DLQ triggers auto-generated |
| Notification triggers | Explicit (no shorthand) | Payload fields vary across services; treated as regular triggers |
| Local k8s deployment | `helm upgrade --install` per service | Replaces `kubectl apply -k`; standard Helm workflow |
| Kustomize helmCharts integration | Rejected | `kubectl apply -k` silently ignores `helmCharts`; officially discouraged for production |
| Chart versioning | Independent of app versions | Chart version tracks template changes; app version set via `image.tag` at install time |
| Values file location | `deploy/{service}/values-{env}.yaml` for real envs; `charts/bookinfo-service/ci/` for ct test fixtures | Clean separation of test fixtures from deployment configs |
| Infra manifests | Unchanged | Gateway, postgres, redis, observability, platform keep current kustomize/helm-values approach |

## Chart Structure

```
charts/
  bookinfo-service/
    Chart.yaml
    values.yaml
    values.schema.json
    templates/
      _helpers.tpl
      deployment.yaml
      deployment-write.yaml
      service.yaml
      service-write.yaml
      configmap.yaml
      eventsource.yaml
      eventsource-service.yaml
      sensor.yaml
      hpa.yaml
      NOTES.txt
    ci/
      values-details-cqrs.yaml
      values-ratings-simple.yaml
      values-productpage-readonly.yaml
```

## values.yaml — Full Shape

```yaml
# -- Service identity
nameOverride: ""
fullnameOverride: ""
serviceName: ""  # maps to SERVICE_NAME env var

image:
  repository: event-driven-bookinfo/details
  tag: latest
  pullPolicy: IfNotPresent

# -- Deployment
replicaCount: 1
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi

# -- Ports
ports:
  http: 8080
  admin: 9090

# -- Probes (on admin port)
probes:
  liveness:
    path: /healthz
    port: admin
  readiness:
    path: /readyz
    port: admin

# -- Config (injected as ConfigMap env vars)
config:
  LOG_LEVEL: "info"
  STORAGE_BACKEND: "memory"
  # Service-specific entries added per values file:
  # RATINGS_SERVICE_URL, DETAILS_SERVICE_URL, DATABASE_URL, etc.

# -- CQRS
cqrs:
  enabled: false
  read:
    replicas: 1
    resources: {}  # inherits top-level resources if empty
  write:
    replicas: 1
    resources: {}  # inherits top-level resources if empty
  eventBusName: kafka
  endpoints: {}
    # See "Endpoint Abstraction" section below

# -- Sensor defaults (applied to all triggers unless overridden per-trigger)
sensor:
  retryStrategy:
    steps: 3
    duration: 2s
    factor: "2.0"
    jitter: "1"
  atLeastOnce: true
  dlq:
    enabled: true
    url: ""  # must be set per environment (e.g., "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12004/dlq-event-received")
    retryStrategy:
      steps: 5
      duration: 2s
      factor: "2.0"
      jitter: "1"

# -- Observability (injected as env vars on deployments, eventsources, sensors)
observability:
  otelEndpoint: ""
  pyroscopeAddress: ""

# -- Autoscaling
autoscaling:
  enabled: false
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilization: 70
```

## Endpoint Abstraction — CQRS EventSource + Sensor

Each entry in `cqrs.endpoints` generates:
1. One **EventSource** (webhook with name, port, method, endpoint)
2. One **EventSource Service** (K8s Service exposing the webhook port)
3. **Sensor dependencies** (aggregated into a single Sensor per release)
4. **Sensor triggers** (from the `triggers` list)
5. **DLQ triggers** (auto-generated per main trigger when `sensor.dlq.enabled: true`)

### Endpoint Schema

```yaml
cqrs:
  endpoints:
    <event-name>:              # e.g., "rating-submitted"
      port: <int>              # webhook port (e.g., 12002)
      method: <string>         # HTTP method (e.g., POST)
      endpoint: <string>       # webhook path (e.g., /v1/ratings)
      triggers:
        - name: <string>       # trigger name (e.g., "create-rating")
          url: <string>        # "self" or explicit URL
          method: <string>     # optional, defaults to POST
          headers: {}          # optional, additional headers
          payload:             # list of payload mappings
            - passthrough      # shorthand: body → "" (request as-is)
            # OR explicit mapping:
            - src:
                dependencyName: <string>  # auto-derived from endpoint if omitted
                dataKey: <string>
                value: <string>           # static value (mutually exclusive with dataKey)
              dest: <string>
```

### Shorthand Resolutions

**`url: self`** resolves to:
- CQRS enabled: `http://{fullname}-write.{namespace}.svc.cluster.local{endpoint}`
- CQRS disabled: `http://{fullname}.{namespace}.svc.cluster.local{endpoint}`

**`payload: [passthrough]`** resolves to:
```yaml
payload:
  - src:
      dependencyName: {event-name}-dep
      dataKey: body
    dest: ""
```

**`dependencyName` auto-derivation:** When omitted from a `src` block, defaults to `{event-name}-dep` (the dependency for the endpoint that owns this trigger).

### DLQ Auto-Generation

For every trigger (except triggers in a service where `sensor.dlq.enabled: false`), the chart auto-generates a paired DLQ trigger with:
- Name: `dlq-{trigger-name}`
- URL: `sensor.dlq.url`
- Method: POST
- RetryStrategy: `sensor.dlq.retryStrategy`
- Standard 11-field payload mapping:

| # | Source | Destination |
|---|---|---|
| 1 | `dataKey: body` | `original_payload` |
| 2 | `contextKey: id` | `event_id` |
| 3 | `contextKey: type` | `event_type` |
| 4 | `contextKey: source` | `event_source` |
| 5 | `contextKey: subject` | `event_subject` |
| 6 | `contextKey: time` | `event_timestamp` |
| 7 | `contextKey: datacontenttype` | `datacontenttype` |
| 8 | `dataKey: header` | `original_headers` |
| 9 | `value: {sensor-name}` | `sensor_name` |
| 10 | `value: {trigger-name}` | `failed_trigger` |
| 11 | `value: {eventsource-url}` | `eventsource_url` |
| 12 | `value: {namespace}` | `namespace` |

## Concrete Examples

### Ratings Service — `deploy/ratings/values-local.yaml`

```yaml
serviceName: ratings
image:
  repository: event-driven-bookinfo/ratings
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_ratings?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
  write:
    replicas: 1
  endpoints:
    rating-submitted:
      port: 12002
      method: POST
      endpoint: /v1/ratings
      triggers:
        - name: create-rating
          url: self
          payload:
            - passthrough
        - name: notify-rating-submitted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: rating-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New rating submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
```

### Reviews Service — `deploy/reviews/values-local.yaml` (Multi-Endpoint)

```yaml
serviceName: reviews
image:
  repository: event-driven-bookinfo/reviews
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_reviews?sslmode=disable"
  RUN_MIGRATIONS: "true"
  RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
  write:
    replicas: 1
  endpoints:
    review-submitted:
      port: 12001
      method: POST
      endpoint: /v1/reviews
      triggers:
        - name: create-review
          url: self
          payload:
            - passthrough
        - name: notify-review-submitted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: review-submitted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "New review submitted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
    review-deleted:
      port: 12003
      method: POST
      endpoint: /v1/reviews/delete
      triggers:
        - name: delete-review-write
          url: self
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
        - name: delete-review-read
          url: "http://reviews.bookinfo.svc.cluster.local/v1/reviews/delete"
          method: POST
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.review_id
              dest: review_id
        - name: notify-review-deleted
          url: "http://notification.bookinfo.svc.cluster.local/v1/notifications"
          method: POST
          payload:
            - src:
                dependencyName: review-deleted-dep
                dataKey: body.product_id
              dest: subject
            - src:
                value: "Review deleted"
              dest: body
            - src:
                value: "system@bookinfo"
              dest: recipient
            - src:
                value: "email"
              dest: channel
```

### Productpage — `deploy/productpage/values-local.yaml` (No CQRS, No Events)

```yaml
serviceName: productpage
image:
  repository: event-driven-bookinfo/productpage
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "memory"
  DETAILS_SERVICE_URL: "http://details.bookinfo.svc.cluster.local"
  REVIEWS_SERVICE_URL: "http://reviews.bookinfo.svc.cluster.local"
  REDIS_URL: "redis://redis-master.bookinfo.svc.cluster.local:6379"
  TEMPLATE_DIR: "/app/templates"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"
```

### DLQueue — `deploy/dlqueue/values-local.yaml` (CQRS, No DLQ Triggers)

```yaml
serviceName: dlqueue
image:
  repository: event-driven-bookinfo/dlqueue
  tag: local

config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_dlqueue?sslmode=disable"
  RUN_MIGRATIONS: "true"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

sensor:
  dlq:
    enabled: false  # dlqueue IS the DLQ — no DLQ triggers for itself

cqrs:
  enabled: true
  read:
    replicas: 1
  write:
    replicas: 1
  endpoints:
    dlq-event-received:
      port: 12004
      method: POST
      endpoint: /dlq-event-received
      triggers:
        - name: ingest-dlq-event
          url: self
          payload:
            - passthrough
```

## Template Logic Summary

| Template | Condition | Produces |
|---|---|---|
| `deployment.yaml` | Always | Read Deployment (or single Deployment when CQRS off) |
| `deployment-write.yaml` | `cqrs.enabled` | Write Deployment with `-write` suffix |
| `service.yaml` | Always | ClusterIP Service for read/single |
| `service-write.yaml` | `cqrs.enabled` | ClusterIP Service for write with `-write` suffix |
| `configmap.yaml` | Always | ConfigMap from `config` map + computed fields (SERVICE_NAME, ports) |
| `eventsource.yaml` | `cqrs.endpoints` non-empty | One EventSource per endpoint entry |
| `eventsource-service.yaml` | `cqrs.endpoints` non-empty | One K8s Service per EventSource (exposes webhook port) |
| `sensor.yaml` | `cqrs.endpoints` has triggers | Single Sensor aggregating all dependencies and triggers |
| `hpa.yaml` | `autoscaling.enabled` | HorizontalPodAutoscaler for read deployment |
| `NOTES.txt` | Always | Post-install usage notes |

## CI/CD Workflows

### `helm-lint-test.yml` (PR validation)

Triggered on PRs touching `charts/**`.

1. Checkout + `helm/chart-testing-action` setup
2. `ct list-changed` — detect changed charts
3. `ct lint` — helm lint + yamllint + values schema validation
4. `helm/kind-action` — spin up ephemeral Kind cluster
5. `ct install` — install chart once per `ci/*.yaml` values file

### `helm-release.yml` (publish to GitHub Pages)

Triggered on push to `main` touching `charts/**`.

1. Checkout
2. `helm/chart-releaser-action` with `charts_dir: charts`
3. Packages chart as `.tgz`, creates GitHub Release
4. Updates `index.yaml` on `gh-pages` branch
5. Chart available via: `helm repo add bookinfo https://kaio6fellipe.github.io/event-driven-bookinfo`

### Makefile Changes

```makefile
# New: deploy apps via Helm
k8s-deploy:
	@for svc in productpage details reviews ratings notification dlqueue; do \
		$(HELM) upgrade --install $$svc charts/bookinfo-service \
			--namespace $(K8S_NS_BOOKINFO) --create-namespace \
			-f deploy/$$svc/values-local.yaml; \
	done

# New: fast rebuild (images + helm upgrade)
k8s-rebuild:
	@for svc in productpage details reviews ratings notification dlqueue; do \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . && \
		k3d image import event-driven-bookinfo/$$svc:local --cluster $(K8S_CLUSTER); \
	done
	@$(MAKE) k8s-deploy
	@$(KUBECTL) -n $(K8S_NS_BOOKINFO) rollout restart deploy
```

## Migration Plan

### Removed After Validation

```
deploy/{details,reviews,ratings,notification,productpage,dlqueue}/base/     # replaced by chart templates
deploy/{details,reviews,ratings,notification,productpage,dlqueue}/overlays/  # replaced by values files
```

### Kept Unchanged

```
deploy/gateway/          # Envoy Gateway + HTTPRoutes (not our chart)
deploy/observability/    # Helm values for Prometheus/Grafana/Tempo/Loki/Pyroscope/Alloy
deploy/platform/         # Strimzi Kafka + Argo Events operator
deploy/postgres/         # PostgreSQL StatefulSet
deploy/redis/            # Bitnami Redis Helm values
deploy/k6/               # Load testing CronJob
```

## Chart Version Policy

- **Chart version** (`version` in Chart.yaml): bumped on template/helper changes. Independent semver.
- **App version** (`appVersion` in Chart.yaml): informational only. Each service sets `image.tag` at install time matching its own release tag (`{service}-vX.Y.Z`).
- **Release workflow**: chart-releaser-action on `charts/` changes to main. Completely independent from the existing `auto-tag.yml` + `release.yml` service release pipeline.

## Documentation and Tooling Updates

### CLAUDE.md Updates

The following sections need updating after migration:

- **Deploy Structure**: Replace the kustomize-based tree with the new `charts/` + `deploy/{service}/values-{env}.yaml` layout
- **Local Kubernetes**: Update `k8s-deploy` description to reflect `helm upgrade --install` instead of `kubectl apply -k`
- **Run Locally**: No changes (local `go run` is unaffected)

### `.claude/skills/new-service/SKILL.md` Updates

Step 8 currently reads:
> 8. Add a Dockerfile and Kustomize base manifests.

Must change to:
> 8. Add a Dockerfile and Helm values file (`deploy/{{service}}/values-local.yaml`).

Add guidance to use an existing service's values file as a template (e.g., `deploy/ratings/values-local.yaml`).

### `.claude/skills/test-service/SKILL.md`

No changes needed — tests are Go-level, not deployment-level.

### `.claude/rules/release.md`

Add a section documenting the chart release process:
- Chart version lifecycle (independent of service tags)
- `helm-release.yml` workflow
- `helm repo add` instructions

### Makefile Updates

The following targets change:

| Target | Current | New |
|---|---|---|
| `k8s-deploy` | `kubectl apply -k deploy/{service}/overlays/local` per service | `helm upgrade --install {service} charts/bookinfo-service -f deploy/{service}/values-local.yaml` per service |
| `k8s-rebuild` | `kubectl apply -k` + `rollout restart` | `helm upgrade --install` per service + `rollout restart` |

New targets to consider:

| Target | Purpose |
|---|---|
| `helm-lint` | `ct lint` on local chart |
| `helm-template` | `helm template` for dry-run inspection |

Targets that remain unchanged: `k8s-cluster`, `stop-k8s`, `k8s-platform`, `k8s-observability`, `k8s-seed`, `k8s-status`, `k8s-logs`, `k8s-load`, `k8s-load-start`, `k8s-load-stop`.

## Reference Charts

- **bjw-s app-template**: Named `controllers` map pattern for CQRS; `rawResources` for CRDs
- **stakater/application**: Per-resource section structure; ServiceMonitor; HPA patterns
- **bitnami/common**: Naming/label helper conventions
- **podinfo**: Monorepo chart CI/CD with GitHub Pages publishing
