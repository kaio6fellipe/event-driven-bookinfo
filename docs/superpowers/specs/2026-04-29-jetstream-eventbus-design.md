# NATS JetStream as a Second EventBus — Design

**Date:** 2026-04-29
**Status:** Draft (pending implementation plan)
**Author:** Claude (Opus 4.7) — drafted from brainstorming session with @kaio6fellipe

## Goal

Make this repo a personal testing setup for Argo Events and event-driven
patterns by supporting **NATS JetStream as an alternative EventBus** alongside
Kafka. The selected bus is chosen at `make run-k8s` time via an `eventbus`
parameter and provisions an isolated k3d cluster per bus. Application services
run from a single multi-bus image and pick their backend at startup via env
vars; the Helm chart renders the appropriate Argo Events `EventSource` based on
a `events.bus.type` value. AsyncAPI specs grow a second server entry so
downstream tooling sees both buses. CQRS write flow, sensor wiring, DLQ flow,
docker-compose, and `make e2e` are unchanged.

## Non-Goals

- Production / non-local NATS deployment.
- Side-by-side coexistence of both buses in one cluster.
- Switching buses without recreating the cluster.
- Generic message-broker abstraction beyond Kafka + JetStream.
- AsyncAPI message-binding declarations beyond `protocol: kafka` / `protocol: nats`.
- Changing the docker-compose stack, `make e2e`, or DLQ flow.

## Cluster topology

Two mutually exclusive k3d clusters:

| Bus | Cluster name | EventBus CR | Platform components |
|---|---|---|---|
| Kafka (default) | `bookinfo-kafka-local` | `default` (spec.kafka) | Strimzi operator, Kafka KRaft single-node, Argo Events controller (custom CRDs from `argoproj/argo-events#3961` + `#3983`), Envoy Gateway, full observability stack |
| JetStream | `bookinfo-jetstream-local` | `default` (spec.jetstreamExotic) | Standalone NATS Helm chart with JetStream enabled, Argo Events controller (same custom CRDs), Envoy Gateway, full observability stack |

`make run-k8s eventbus=kafka` (default) → kafka cluster.
`make run-k8s eventbus=jetstream` → jetstream cluster.
Starting one refuses if the other is running — user must `make stop-k8s` first
(coexistence is intentionally blocked to avoid double-spending docker memory).

The runtime image is identical on both clusters. Backend selection happens at
pod startup via env vars (`EVENT_BACKEND=kafka|jetstream`).

## Go code: `pkg/` and per-service adapters

### Rename and split `pkg/eventskafka` → `pkg/eventsmessaging`

New package layout:

```
pkg/eventsmessaging/
├── publisher.go       // Publisher interface
├── kafkapub/          // existing franz-go producer (renamed)
│   ├── producer.go
│   └── producer_test.go
└── natspub/           // new NATS JetStream producer
    ├── producer.go
    └── producer_test.go
```

`Publisher` interface (in `publisher.go`):

```go
type Publisher interface {
    Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error
    Close()
}
```

`kafkapub.NewProducer(ctx, brokers, topic) (Publisher, error)` returns the
existing franz-go-backed producer, behaviour identical (including
`ensureTopic`).

`natspub.NewProducer(ctx, url, token, streamName, subject) (Publisher, error)`:

- Creates a `nats.Conn` with `nats.Token(token)` (only set when token non-empty).
- Acquires a `JetStreamContext`.
- Calls `js.AddStream(&nats.StreamConfig{Name: streamName, Subjects: []string{subject}})` and tolerates the "stream already exists" error (idempotent — analogue of `ensureTopic`).
- Publishes via `js.PublishMsg(&nats.Msg{ Subject, Data, Header })` where `Header` carries CloudEvents binary attributes (`ce-specversion`, `ce-type`, `ce-source`, `ce-id`, `ce-time`, `ce-subject`, `content-type`) and `traceparent` for OTel context propagation.

Both impls reuse `pkg/events.Descriptor` for CE attribute values — the
descriptor stays bus-neutral.

### Telemetry helper

Add `pkg/telemetry/nats.go` mirroring `pkg/telemetry/kafka.go`. Exposes
`StartProducerSpan(ctx, subject, idempotencyKey)` and
`InjectTraceContext(ctx, msg *nats.Msg)`. Span attributes:
`messaging.system=jetstream`, `messaging.destination.name=<subject>`,
`messaging.operation=publish`. Existing kafka helper unchanged.

### Per-service adapter rename

For every producer service (`details`, `reviews`, `ratings`, `ingestion`):

- `services/<svc>/internal/adapter/outbound/kafka/` → `outbound/messaging/`
- `producer.go`: replace embedded `*eventskafka.Producer` with an `eventsmessaging.Publisher` interface field. Typed wrappers (`PublishBookAdded`, etc.) keep their public signatures.
- `exposed.go`: unchanged — `[]events.Descriptor` is bus-neutral.
- `noop.go` (where present): unchanged — implements `Publisher` already.

### `cmd/main.go` per service

Reads `EVENT_BACKEND` env. Wires the right impl:

```go
var pub eventsmessaging.Publisher
switch os.Getenv("EVENT_BACKEND") {
case "kafka":
    pub, err = kafkapub.NewProducer(ctx, os.Getenv("KAFKA_BROKERS"), topic)
case "jetstream":
    pub, err = natspub.NewProducer(ctx, os.Getenv("NATS_URL"), os.Getenv("NATS_TOKEN"), streamName, subject)
default:
    pub = noop.New()
}
```

Empty / unset backend keeps the no-op publisher path (compose mode unchanged).

### Tests

- `kafkapub/producer_test.go`: existing tests, renamed package.
- `natspub/producer_test.go`: new — uses `github.com/nats-io/nats-server/v2/test` embedded server (no docker dep).
- Service-level `*_test.go`: pivot from concrete `*kafkapub.Producer` to a `Publisher` interface mock.

## Helm chart: `charts/bookinfo-service`

### Values schema

Replace the `events.busName` + `events.kafka.broker` shape with:

```yaml
events:
  bus:
    type: kafka              # kafka | jetstream — drives template selection
  kafka:
    broker: ""               # set in values-local-kafka.yaml
  jetstream:
    url: ""                  # set in values-local-jetstream.yaml
    tokenSecret:
      name: nats-client-token
      key: token             # mounted as NATS_TOKEN via valueFrom.secretKeyRef
  exposed: {}                # generic; topic field used as kafka topic AND nats subject
  consumed: {}               # unchanged shape
```

EventBus name is hardcoded to `default` everywhere in templates
(`eventBusName: default` in `eventsource.yaml`, the kafka and jetstream
`-eventsource.yaml` templates, `sensor.yaml`, `consumer-sensor.yaml`).

### Template changes

| Template | Change |
|---|---|
| `kafka-eventsource.yaml` | Wrap range body in `{{- if eq .Values.events.bus.type "kafka" }}` so it only renders on kafka clusters. |
| `jetstream-eventsource.yaml` (new) | Mirrors kafka template. Renders `apiVersion: argoproj.io/v1alpha1`, `kind: EventSource`, `spec.jetstream.<exposureKey>: { url, accessSecret, subject, jsonBody }` per `events.exposed` entry when `bus.type=jetstream`. |
| `kafka-eventsource-rbac.yaml` | Rename to `eventsource-rbac.yaml`. Drop kafka-specific naming — leader-election RBAC is bus-agnostic; render whenever `events.exposed` is non-empty. |
| `configmap.yaml` | Add `EVENT_BACKEND: {{ .Values.events.bus.type }}`. If kafka → emit `KAFKA_BROKERS`; if jetstream → emit `NATS_URL`. |
| `deployment.yaml` + `deployment-write.yaml` | If `bus.type=jetstream`, add an `env` entry `NATS_TOKEN` with `valueFrom.secretKeyRef` from `events.jetstream.tokenSecret`. |
| `eventsource.yaml`, `sensor.yaml`, `consumer-sensor.yaml` | No structural change — switch `eventBusName` to literal `default`. |

### `ci/` test values

Add jetstream variants for chart-testing:
`charts/bookinfo-service/ci/values-<svc>-jetstream.yaml` per service.
`helm-lint-test.yml` runs ct against both kafka and jetstream variants.

### Specgen impact

`tools/specgen` writes `events.exposed.<key>.{topic, contentType, eventTypes}`
— same field names work for both buses. The chart re-uses the `topic` string
as the JetStream subject. **No specgen changes needed for the values output**;
only the source-comment header tweaks (see below).

## Specgen + generated artifacts

### `tools/specgen/internal/runner/metadata.go`

Replace `AsyncAPIServer ServerEntry` with `AsyncAPIServers map[string]ServerEntry`:

```go
AsyncAPIServers: map[string]ServerEntry{
    "kafka":     {URL: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092", Description: "Local Kafka bootstrap"},
    "jetstream": {URL: "nats://nats.platform.svc.cluster.local:4222", Description: "Local NATS JetStream"},
}
```

### `tools/specgen/internal/asyncapi/asyncapi.go`

Replace the single hard-wired `kafkaServerNode` block (current lines 315–322)
with a range over `Metadata.AsyncAPIServers`. Each entry renders with its own
protocol — `protocol: kafka` for kafka, `protocol: nats` for jetstream. Add a
`Protocol string` field to `ServerEntry` (or branch on the map key).

### Source-file comment updates

The Go folder rename `outbound/kafka/` → `outbound/messaging/` cascades into:

- `tools/specgen/internal/asyncapi/asyncapi.go:357` — change `outbound/kafka/exposed.go` → `outbound/messaging/exposed.go`.
- `tools/specgen/internal/values/values.go:188` — same.
- All regenerated `services/*/api/asyncapi.yaml` and `deploy/*/values-generated.yaml` get the comment rewrite.

### Regenerated artifacts (committed)

- `services/*/api/asyncapi.yaml` — second server block (jetstream).
- `deploy/*/values-generated.yaml` — body unchanged; only the `# generated by tools/specgen from …` header line updates the Go path.
- `services/*/api/openapi.yaml` — unchanged (HTTP-only).
- `services/*/api/catalog-info.yaml` — unchanged.

### Tests

`tools/specgen/internal/asyncapi/asyncapi_test.go` testdata fixture
regenerated; new assertion that both servers render with correct protocols.

## Platform layer

### Existing kafka platform (renamed for symmetry)

`deploy/platform/local/`:
- `kafka-cluster.yaml`, `kafka-nodepool.yaml`, `strimzi-values.yaml` unchanged.
- `eventbus.yaml` renamed to `eventbus-kafka.yaml`. `metadata.name` flips from `kafka` to `default` to match the chart's hardcoded `eventBusName: default`. **Breaking change for in-flight kafka clusters** — anyone with the old cluster running needs `stop-k8s` then `run-k8s` to pick up the new EventBus name.
- `argo-events-values.yaml` unchanged.

### New jetstream platform

`deploy/platform/local/jetstream/` (new directory):

- `nats-values.yaml` — values for the `nats-io/nats` Helm chart. JetStream enabled (`config.jetstream.enabled: true`), single-replica, file storage on emptyDir, port 4222 exposed via ClusterIP service `nats.platform.svc.cluster.local`. Token auth: `auth.token` set to a literal local-dev token value (not committed as plain text — sourced from a Kubernetes Secret rendered separately, see below).
- `nats-token-secret.yaml` — `Secret` named `nats-client-token` with key `token` holding the shared local-dev token. Rendered into both `bookinfo` and `platform` namespaces (Argo Events EventBus needs it via `accessSecret`).
- `eventbus-jetstream.yaml` — `EventBus` named `default` with:
  ```yaml
  spec:
    jetstreamExotic:
      url: nats://nats.platform.svc.cluster.local:4222
      accessSecret:
        name: nats-client-token
        key: token
      streamConfig: ""
  ```

Argo Events controller install (Helm) is identical on both clusters and pulls
the same custom CRDs from
`ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`.

## Makefile

### Variables

- `EVENTBUS ?= kafka` — settable via `make run-k8s eventbus=jetstream`. Lowercased and validated against `kafka|jetstream`.
- `K8S_CLUSTER := bookinfo-$(EVENTBUS)-local` — always derived. No default-name special case.

### Target changes

- `k8s-platform`: branches on `$(EVENTBUS)`.
  - kafka path: existing Strimzi flow + `kubectl apply -f deploy/platform/local/eventbus-kafka.yaml`.
  - jetstream path: `helm install nats nats-io/nats -f deploy/platform/local/jetstream/nats-values.yaml`, then `kubectl apply -f deploy/platform/local/jetstream/nats-token-secret.yaml` (to both `bookinfo` and `platform` namespaces) + `eventbus-jetstream.yaml`.
- `k8s-deploy`: picks the right values file per service: `-f deploy/<svc>/values-local-$(EVENTBUS).yaml`. The `values-generated.yaml` line stays unchanged.
- `run-k8s`: prints which cluster + which bus is active. **Refuses to start** if the *other* cluster exists; instructs user to run `make stop-k8s` first.
- `stop-k8s`: deletes `$(K8S_CLUSTER)`. Optional flag to delete both, but default is just the active one.
- `k8s-status`: shows bus type plus access URLs (NATS port-forward URL added when jetstream).
- `k8s-rebuild`: works against the active cluster.
- All existing `--context=k3d-bookinfo-local` flags switch to `--context=k3d-bookinfo-$(EVENTBUS)-local`.

## Per-service `values-local-*.yaml` split

For each service in `details`, `reviews`, `ratings`, `ingestion`,
`productpage`, `notification`, `dlqueue`:

**`values-local-kafka.yaml`** (rename of current `values-local.yaml`):

```yaml
config:
  KAFKA_BROKERS: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
  EVENT_BACKEND: "kafka"
events:
  bus:
    type: kafka
  kafka:
    broker: "bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092"
# ... rest of service config (postgres, observability, cqrs, sensor, gateway, consumed) unchanged
```

**`values-local-jetstream.yaml`** (new):

```yaml
config:
  NATS_URL: "nats://nats.platform.svc.cluster.local:4222"
  EVENT_BACKEND: "jetstream"
events:
  bus:
    type: jetstream
  jetstream:
    url: "nats://nats.platform.svc.cluster.local:4222"
    tokenSecret:
      name: nats-client-token
      key: token
# ... rest of service config unchanged
```

`consumed:` blocks are identical across both files. The trigger spec is
bus-agnostic — the chart sensor template uses `eventSourceName` and
`eventName`, which point at whatever EventSource the producing service's chart
rendered (kafka or jetstream variant). EventSource names match across buses.

Services that don't produce events (`productpage`, `notification`, `dlqueue`)
get the same split for consistency, even though they only set
`events.bus.type` and consumer-side fields.

## Documentation

### `docs/kafka-eventbus.md` (new)

How the Kafka path works in this repo. Sections:
- Cluster topology (Strimzi+Kafka in `platform`).
- EventBus CR shape (`spec.kafka`).
- Exposed-event flow (producer → topic → kafka EventSource → EventBus → Sensor → trigger).
- CQRS write flow (Webhook EventSource → EventBus → Sensor → HTTP).
- ConfigMap env wiring (`KAFKA_BROKERS`).
- Idempotency + `ensureTopic` semantics.
- Where to look in code: `pkg/eventsmessaging/kafkapub`, chart `kafka-eventsource.yaml`, `deploy/platform/local/eventbus-kafka.yaml`.
- Mermaid diagram.
- Reference links: Strimzi, franz-go, Argo Events kafka EventSource docs.

### `docs/jetstream-eventbus.md` (new)

The parallel doc for NATS JetStream. Sections:
- Cluster topology (standalone NATS Helm in `platform`, `jetstreamExotic` EventBus pointing at it, token auth via `nats-client-token` secret).
- Exposed-event flow (producer → subject → jetstream EventSource → EventBus → Sensor → trigger).
- Why streams must be ensured at producer startup (no Strimzi-style auto-creation).
- ConfigMap env wiring (`NATS_URL` + `NATS_TOKEN`).
- Where to look: `pkg/eventsmessaging/natspub`, chart `jetstream-eventsource.yaml`, `deploy/platform/local/jetstream/`.
- Mermaid diagram.
- Reference links: Argo Events jetstream EventSource docs, nats.go JetStream API, NATS JetStream design docs.

### `README.md` updates

- Top-level architecture diagram annotated to show the bus is pluggable (`Kafka **or** JetStream EventBus`).
- New "Choosing an EventBus" subsection under "Local Kubernetes": one paragraph each pointing at the two new docs, plus `make run-k8s eventbus=kafka|jetstream` usage.
- Update the `make run-k8s` table cell description.
- Update the `make k8s-platform` cell to mention conditional NATS install.
- Update cluster name reference (`bookinfo-kafka-local` default).
- Mention the dual-server AsyncAPI output in the existing AsyncAPI/specgen section.

## Testing & observability

### Tests

- Unit: `kafkapub` keeps existing tests; `natspub` gets new tests using embedded `nats-server/v2/test` package.
- Service-level adapter tests: pivot from concrete `*kafkapub.Producer` to a `Publisher` interface mock — bus-agnostic.
- `helm-lint-test.yml`: ct lint + ct install run twice (once per kafka values, once per jetstream values).
- Smoke: post-install, hit a productpage POST → assert it shows in the read path on either cluster.

### Observability validation

- After `make run-k8s eventbus=jetstream`, verify Tempo trace links from producer → jetstream EventSource pod → Sensor → trigger handler.
- Use the existing producer→consumer trace check, adapted for NATS subjects.

## Risks and contingencies

### JetStream tracing risk + argo-events fork update playbook

If spans don't connect across the JetStream EventSource pod (i.e., trace
propagation breaks at the new `spec.jetstream` EventSource type), follow this
playbook to update the upstream argo-events fork:

1. **Rebase `feat/cloudevents-compliance-otel-tracing` on `argoproj:master`.** Known conflicts in `go.mod` / `go.sum` — resolve by accepting upstream and re-running `go mod tidy`. Keep the 15 existing commits intact (no squash). No `Co-Authored-By` trailer. Push with `git push --force-with-lease origin feat/cloudevents-compliance-otel-tracing` — this automatically updates PR argoproj/argo-events#3961 head SHA. No PR description or comment edits.
2. Apply any JetStream tracing fix on top.
3. Rebase / cherry-pick the new commits onto `feat/combined-prs-3961-3983`.
4. `make image-multi` → push `ghcr.io/kaio6fellipe/argo-events:prs-3961-3983`.
5. No tag bump in Makefile `k8s-platform` unless the image SHA forces a CRD-asset regeneration (those are GitHub release assets — bump release tag only if the assets need refreshing).

### Other known consequences

- **EventBus rename** (`metadata.name: kafka` → `default`) is a one-time breaking change for in-flight kafka clusters. `stop-k8s` then `run-k8s`.
- **Wide refactor**: `pkg/eventskafka` rename + `outbound/kafka/` → `outbound/messaging/` touches every producer service's `cmd/main.go` and adapter. Single PR, gated behind `make build-all && make test && make helm-lint`.

## Out of scope (confirmed)

- `docker-compose` stack (no event chain anyway, per `CLAUDE.md`).
- `make e2e` shell smoke tests (HTTP-level, no bus).
- DLQ flow (bus-agnostic; sensor failure path → dlqueue webhook stays the same).
- Production / non-local NATS deployments.
- AsyncAPI 3.x bindings beyond `protocol: kafka` and `protocol: nats`.

## Files changed (high-level inventory)

```
Makefile                                                                modified
README.md                                                               modified
CLAUDE.md                                                               modified (cluster name + bus selection)
docs/kafka-eventbus.md                                                  new
docs/jetstream-eventbus.md                                              new

pkg/eventskafka/                                                        renamed → pkg/eventsmessaging/kafkapub/
pkg/eventsmessaging/publisher.go                                        new
pkg/eventsmessaging/natspub/producer.go                                 new
pkg/eventsmessaging/natspub/producer_test.go                            new
pkg/telemetry/nats.go                                                   new
pkg/telemetry/nats_test.go                                              new

services/details/internal/adapter/outbound/kafka/                       renamed → .../messaging/
services/reviews/internal/adapter/outbound/kafka/                       renamed → .../messaging/
services/ratings/internal/adapter/outbound/kafka/                       renamed → .../messaging/
services/ingestion/internal/adapter/outbound/kafka/                     renamed → .../messaging/
services/<svc>/internal/adapter/outbound/messaging/producer.go          modified (Publisher interface)
services/<svc>/cmd/main.go                                              modified (EVENT_BACKEND switch)

charts/bookinfo-service/values.yaml                                     modified (events.bus shape)
charts/bookinfo-service/templates/kafka-eventsource.yaml                modified (gated on bus.type=kafka)
charts/bookinfo-service/templates/jetstream-eventsource.yaml            new
charts/bookinfo-service/templates/kafka-eventsource-rbac.yaml           renamed → eventsource-rbac.yaml (bus-agnostic)
charts/bookinfo-service/templates/configmap.yaml                        modified (NATS_URL / EVENT_BACKEND)
charts/bookinfo-service/templates/deployment.yaml                       modified (NATS_TOKEN env)
charts/bookinfo-service/templates/deployment-write.yaml                 modified (NATS_TOKEN env)
charts/bookinfo-service/templates/eventsource.yaml                      modified (eventBusName: default)
charts/bookinfo-service/templates/sensor.yaml                           modified (eventBusName: default)
charts/bookinfo-service/templates/consumer-sensor.yaml                  modified (eventBusName: default)
charts/bookinfo-service/ci/values-<svc>-jetstream.yaml                  new (per service)

deploy/platform/local/eventbus.yaml                                     renamed → eventbus-kafka.yaml; metadata.name → default
deploy/platform/local/jetstream/nats-values.yaml                        new
deploy/platform/local/jetstream/nats-token-secret.yaml                  new
deploy/platform/local/jetstream/eventbus-jetstream.yaml                 new

deploy/<svc>/values-local.yaml                                          renamed → values-local-kafka.yaml
deploy/<svc>/values-local-jetstream.yaml                                new (per service)
deploy/<svc>/values-generated.yaml                                      modified (header comment only)

tools/specgen/internal/runner/metadata.go                               modified (AsyncAPIServers map)
tools/specgen/internal/asyncapi/asyncapi.go                             modified (multi-server, comment path)
tools/specgen/internal/asyncapi/testdata/                               regenerated
tools/specgen/internal/values/values.go                                 modified (comment path)

services/*/api/asyncapi.yaml                                            regenerated (dual server)

.github/workflows/helm-lint-test.yml                                    modified (run ct against both bus variants)
```

## Open questions

None at spec time — all forks resolved during brainstorming. Implementation
plan (next step) decomposes this spec into ordered phases with explicit
dependencies and verification gates.
