# Sharpening values-generated.yaml — design

**Date:** 2026-04-27
**Status:** Approved (brainstorming)
**Owner:** Kaio Fellipe
**Context:** specgen tool was introduced in `2026-04-25-api-spec-generation-design.md` and enriched in `2026-04-26-specgen-enrichment-design.md`. This change extends it so per-service Helm values files (`deploy/<svc>/values-local.yaml`) no longer carry CQRS endpoint or exposed-event configuration.

## Goals

1. `deploy/<svc>/values-local.yaml` carries no `cqrs.endpoints` block, no `events.exposed` block, no `cqrs.eventBusName` key for any service.
2. `deploy/<svc>/values-generated.yaml` becomes the single source of truth for endpoint method/path, exposed event topic, content type, and CE types.
3. Chart provides defaults for every other CQRS / event setting that is currently boilerplate (port, eventBusName, trigger shape).
4. specgen extension stays narrow — walker already extracts the data it needs; only the emitter and the chart change behavior.

## Non-Goals

- No EventSource consolidation. The chart keeps creating one `EventSource` CR per CQRS endpoint.
- No changes to `events.consumed`. Notification and details still author their consumer triggers by hand because they have payload transforms specgen cannot synthesize.
- No changes to `events.kafka.broker`. That is cluster-infra config and stays in values-local.
- No changes to productpage (no `cqrs.endpoints` or `events.exposed` today).

## Background

### Current shape

`deploy/details/values-generated.yaml` today emits a minimal map:

```yaml
cqrs:
  endpoints:
    book-added:
      method: POST
      endpoint: /v1/details
events:
  exposed:
    events:
      contentType: application/json
      eventTypes:
        - com.bookinfo.details.book-added
```

`deploy/details/values-local.yaml` adds the rest:

```yaml
cqrs:
  endpoints:
    book-added:
      port: 12000
      triggers:
        - name: create-detail
          url: self
          payload:
            - passthrough
events:
  exposed:
    events:
      topic: bookinfo_details_events
      eventBusName: kafka
```

Across services, the same boilerplate is duplicated. Ports 12000-12004 are assigned globally-unique by hand. Triggers are `url: self / payload: passthrough` everywhere except `reviews/review-deleted`, which extracts `body.review_id` server-side via the sensor.

### Argo Events port behavior (verified)

- One `EventSource` CR ⇒ one pod with one HTTP server (Argo Events controller spawns it).
- Within a single EventSource's `spec.webhook` map, multiple webhook entries can share the same port; they are routed by path inside the pod (per Argo Events docs: `docs/eventsources/naming.md`).
- Across EventSource CRs, pods are isolated, so port 12000 on pod A and pod B never collide.

The 12000-12004 spread in the current `values-local.yaml` files was a convention, not a constraint. Every endpoint can use port `12000`.

### Live cluster check

`kubectl --context=k3d-bookinfo-local get pods -n bookinfo` (2026-04-27): 9 EventSource pods running, one per CR. Reviews has 3 (`review-submitted`, `review-deleted`, `reviews-events`); kafka EventSources cannot merge with webhook EventSources (different EventSource type), so consolidation buys at most 1 pod and is out of scope here.

## Decisions

| Question | Choice | Why |
|---|---|---|
| Trigger transformations | All passthrough; refactor `reviews/review-deleted` handler to accept full body | One special case removed; chart can synthesize a uniform default |
| Port allocation | Hardcoded `12000` for all endpoints in chart, omitted from values | No real constraint requires uniqueness; simplest design |
| Trigger emission | Chart synthesizes default passthrough trigger when `endpoint.triggers` is absent | Less generated YAML, behavior lives in chart |
| `eventBusName` placement | Single top-level `events.busName: kafka` chart default | Replaces both `cqrs.eventBusName` and per-event `events.exposed.<key>.eventBusName` |

## Architecture & data flow

```
services/<svc>/internal/adapter/inbound/http/endpoints.go    →  api.Endpoint{Method, Path, EventName, ...}
services/<svc>/internal/adapter/outbound/kafka/exposed.go    →  events.Descriptor{ExposureKey, Topic, CEType, ContentType, ...}
                       │                                                       │
                       ▼                                                       ▼
            tools/specgen/internal/walker (already extracts both)
                                     │
                                     ▼
            tools/specgen/internal/values.Build()  (emit topic, no port, no triggers, no eventBusName)
                                     │
                                     ▼
                  deploy/<svc>/values-generated.yaml
                                     │ (helm merges with values-local.yaml)
                                     ▼
                       charts/bookinfo-service templates
                                     │ (apply defaults: port 12000, trigger=passthrough, busName=kafka)
                                     ▼
                EventSource / Sensor / HTTPRoute / kafka-EventSource CRs
```

Chart-applied defaults (not present in any values file):

- `cqrs.eventSource.port` → `12000` — consumed by `eventsource.yaml`, `eventsource-service.yaml`, `httproute.yaml`, sensor DLQ url builder.
- `cqrs.endpoints.<name>.triggers` → synthesized in `sensor.yaml` as `[{name: <eventName>, url: self, payload: [passthrough]}]` when absent.
- `events.busName` → `kafka` — consumed by `eventsource.yaml`, `sensor.yaml`, `consumer-sensor.yaml`, `kafka-eventsource.yaml`.

## File shapes after migration

### `deploy/details/values-generated.yaml`

```yaml
# DO NOT EDIT — generated by tools/specgen
cqrs:
  endpoints:
    book-added:
      method: POST
      endpoint: /v1/details
events:
  exposed:
    events:
      topic: bookinfo_details_events
      contentType: application/json
      eventTypes:
        - com.bookinfo.details.book-added
```

Diff vs today: `topic` added under `events.exposed.events`. Nothing else changes structurally on the generated side.

### `deploy/details/values-local.yaml`

```yaml
serviceName: details
fullnameOverride: details
image:
  repository: event-driven-bookinfo/details
  tag: local

postgresql:
  enabled: true
  auth:
    database: "bookinfo_details"

config:
  LOG_LEVEL: "debug"
  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  KAFKA_TOPIC: "bookinfo_details_events"

observability:
  otelEndpoint: "http://alloy-metrics-traces.observability.svc.cluster.local:4317"
  pyroscopeAddress: "http://pyroscope.observability.svc.cluster.local:4040"

cqrs:
  enabled: true
  read:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  write:
    replicas: 1
    resources:
      requests:
        cpu: 50m
        memory: 64Mi

sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12000/v1/events"

gateway:
  name: default-gw
  namespace: platform
  sectionName: web

events:
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  consumed:
    raw-books-details:
      eventSourceName: ingestion-raw-books-details
      eventName: raw-books-details
      triggers:
        - name: ingest-book-detail
          url: self
          path: /v1/details
          method: POST
          payload:
            - passthrough
      dlq:
        enabled: true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12000/v1/events"
```

Diff vs today: removed `cqrs.endpoints` block (12 lines), removed `events.exposed` block (5 lines), removed `cqrs.eventBusName` if present, dlq URL port updated 12004 → 12000.

### Chart `values.yaml` defaults

```yaml
cqrs:
  enabled: false
  read: {...}
  write: {...}
  eventSource:
    port: 12000        # default port for all webhook EventSources
  # eventBusName removed — replaced by events.busName
  endpoints: {}

events:
  busName: kafka       # default eventBus for CQRS sensor, kafka-eventsource, consumer-sensor
  kafka:
    broker: ""
  exposed: {}
  consumed: {}
```

## Chart template changes

The table below describes the **end-state edits** (post-migration). Migration steps 3 (additive) and 6 (breaking cleanup) split them into safe phases — step 3 wraps each new key with `default <new> <old>` to keep both shapes valid simultaneously; step 6 drops the fallbacks and arrives at the form shown here.

| Template | Change |
|---|---|
| `values.yaml` | Remove `cqrs.eventBusName: kafka`. Add `cqrs.eventSource.port: 12000`. Add `events.busName: kafka`. Update example comments to drop per-event `eventBusName`. |
| `templates/eventsource.yaml` | L11 `eventBusName: {{ $.Values.cqrs.eventBusName }}` → `eventBusName: {{ $.Values.events.busName }}`. L23 `port: {{ $endpoint.port }}` → `port: {{ $.Values.cqrs.eventSource.port }}`. |
| `templates/eventsource-service.yaml` | L14-15 replace `$endpoint.port` with `$.Values.cqrs.eventSource.port`. |
| `templates/httproute.yaml` | L27 `port: {{ $endpoint.port }}` → `port: {{ $.Values.cqrs.eventSource.port }}`. |
| `templates/sensor.yaml` | L18 use `events.busName`. L2-9 `$hasEndpoints` gate becomes `$hasEndpoints = (gt (len .Values.cqrs.endpoints) 0)`. L34-84 trigger-loop synthesizes a default passthrough trigger when `$endpoint.triggers` is empty. L93 DLQ url builder uses `$.Values.cqrs.eventSource.port`. |
| `templates/kafka-eventsource.yaml` | L15 `eventBusName: {{ default "kafka" $event.eventBusName }}` → `eventBusName: {{ $.Values.events.busName }}`. |
| `templates/consumer-sensor.yaml` | L20 use `events.busName`. |
| `charts/bookinfo-service/ci/*.yaml` | Drop `cqrs.eventBusName`, per-endpoint `port`, per-endpoint `triggers`, per-event `eventBusName`. Re-run ct lint+install. |

Synthesized-trigger pseudo-template (sensor.yaml):

```gotemplate
{{- $triggers := $endpoint.triggers }}
{{- if not $triggers }}
{{- $triggers = list (dict "name" $eventName "url" "self" "payload" (list "passthrough")) }}
{{- end }}
{{- range $trigger := $triggers }}
…existing trigger rendering…
{{- end }}
```

## specgen changes

`tools/specgen/internal/values/values.go` — `Build()`:

1. Continue not emitting `port` or `triggers` under `cqrs.endpoints.<name>` (already absent today).
2. Under `events.exposed.<exposureKey>`, add `topic`. Source: `walker.DescriptorInfo.Topic`.
3. When multiple descriptors share `ExposureKey` (reviews case), the `Topic` value is identical across them today. Assert and emit once. If they ever diverge, fail with a clear error: `"events.exposed.<key>: descriptors disagree on Topic (<a> vs <b>)"`.
4. Output keys remain sorted (deterministic diffs).

Header comment on the generated file lists inputs:

```yaml
# DO NOT EDIT — generated by tools/specgen from
#   services/<svc>/internal/adapter/inbound/http/endpoints.go
#   services/<svc>/internal/adapter/outbound/kafka/exposed.go
```

(Already present; no change to the comment block.)

`tools/specgen/internal/values/values_test.go`:

- Table case asserting `topic` emission for single-descriptor service (e.g. ratings).
- Table case asserting `topic` emission for multi-descriptor / shared-`ExposureKey` service (reviews).
- Table case asserting absent triggers / port / eventBusName are NEVER emitted.
- Table case for divergent-`Topic`-same-`ExposureKey` failure.

## Service-code changes

`services/reviews/internal/adapter/inbound/http/{handler.go,dto.go}` — `delete-review` handler refactor:

- Today the sensor extracts `body.review_id` and POSTs `{"review_id": "..."}` to the write pod.
- After: sensor passes the original body unchanged. Handler must extract `review_id` from the same body shape that productpage produces.
- Implementation step: read the productpage producer (the JS/HTMX client that POSTs the delete) to confirm the wire shape, then update the reviews delete DTO accordingly.
- Add a handler test asserting the new full-body parsing.

No changes to `endpoints.go` or `exposed.go` files. No walker / metadata changes.

## Migration order

1. **Documentation audit (no-code).** Sweep:
   - `CLAUDE.md` (Helm Events Configuration paragraph; CQRS deployments paragraph).
   - `charts/bookinfo-service/values.yaml` example comments.
   - `charts/bookinfo-service/README.md` (if generated) and `tools/specgen/README.md` (if present) — note new `topic` field, note that port / triggers / eventBusName are chart defaults.
   - `services/<svc>/internal/adapter/outbound/kafka/exposed.go` header comments — mention `topic` is now emitted.
   - `services/<svc>/internal/adapter/inbound/http/endpoints.go` header comments — verify alignment.
   - `.claude/rules/*.md` — re-scan; no changes expected, confirm.
   - Prior ADR / spec archives that mention the old shape (treat as historical; don't rewrite).

2. **specgen first.** Extend `values.Build()` to emit `topic`. `make generate-specs`. Diff-only commit; existing chart still works because it ignores extra fields. CI green.

3. **Chart additions (additive).** Add `cqrs.eventSource.port`, `events.busName` defaults to chart `values.yaml`. Update templates with `default <new-key> <old-key>` so both shapes work simultaneously. CI green; rendered manifests byte-identical to main.

4. **Reviews handler refactor.** Update `delete-review` handler to accept full body. Update `deploy/reviews/values-local.yaml` review-deleted trigger to plain passthrough. e2e + handler tests.

5. **Per-service values-local cleanup.** One PR per service or one bundled PR. Each PR removes `cqrs.endpoints`, `events.exposed`, `cqrs.eventBusName`. Updates dlq URLs from 12001-12004 → 12000.

6. **Chart cleanup (breaking, last).** Drop the `default` fallbacks for old keys. Drop `cqrs.eventBusName` from chart `values.yaml`. Bump chart version (breaking change in chart contract).

## Validation

### Per-step CI

- `make helm-lint` (lints all per-service values files)
- `make helm-template SERVICE=<svc>` — diff rendered manifests vs main; byte-identical for steps 2-3 and step 5-6.
- `make test` (race + coverage) and `make lint` for any Go-touching steps.
- `make e2e` (compose-level smoke).

### Local k3d full validation (mandatory before PR merge)

- `make stop-k8s && make run-k8s` — fresh full bring-up.
- `make k8s-status` — confirm 9 EventSource pods Running, all backend services Ready.
- Smoke flow:
  1. `GET /productpage?id=0` returns 200 with details + reviews + ratings populated.
  2. `POST` review via productpage → reviews-write records the review → `review-submitted` CE published → notification consumer logs it.
  3. `POST` delete review via productpage → reviews-delete-write processes full body → `review-deleted` CE published → notification consumer logs it.
  4. ingestion publishes a book on its poll interval → details consumer-sensor processes → details record created.
  5. Force a sensor retry exhaustion (scale a write Deployment to 0, fire a request, scale back). Confirm DLQ entry appears in `dlqueue` with `eventsource_url` containing port `12000`. Replay restores the event.
- Open Grafana Tempo at `http://localhost:3000`. Search by `service.name = productpage / details / reviews / ratings / notification / ingestion / dlqueue`. Confirm trace spans connect across the gateway → eventsource → sensor → write pod chain for at least one flow per service. Inspect the `book-added`, `review-submitted`, `review-deleted`, `rating-submitted`, `dlq-event-received`, and `raw-books-details` paths.

### PR status gate (mandatory before merge)

- `gh pr checks <PR-number> --watch` until all checks complete.
- All required checks must be green: build, unit tests, golangci-lint, helm-lint, helm-test (chart-testing), e2e, spec-validation. No exceptions; no merging on red or pending.

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| dlqueue port drift 12004 → 12000 silently breaks `sensor.dlq.url` references in other services | `grep -rn '1200[0-4]'` across `deploy/` to enumerate; update atomically with chart breaking step. |
| Reviews delete body shape mismatch between productpage producer and reviews handler | Read productpage producer first; add handler test for full-body shape before deploying to k3d. |
| External chart pinners (chart published via `helm-release.yml` to GitHub Pages) break on values shape change | Bump chart version on step 6; document breaking change in chart `CHANGELOG.md` if one exists. |
| `events.exposed.<key>.topic` divergence across descriptors with same `ExposureKey` | specgen asserts equality at emit time and fails with a clear error. |

## Open questions

None at design time. Implementation may surface productpage delete-review wire shape details that affect step 4; resolve inline.

## References

- `2026-04-25-api-spec-generation-design.md` — specgen foundation and `values-generated.yaml` introduction.
- `2026-04-26-specgen-enrichment-design.md` — per-op + spec-level metadata enrichment.
- Argo Events docs: webhook EventSource naming and shared-port semantics.
- Live cluster: `kubectl --context=k3d-bookinfo-local get eventsources -n bookinfo` (verified 2026-04-27).
