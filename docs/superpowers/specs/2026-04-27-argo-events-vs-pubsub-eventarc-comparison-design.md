# Design — Argo Events vs Pub/Sub + Eventarc Comparison Doc Set

**Date:** 2026-04-27
**Status:** Draft (pending user review)
**Owner:** Kaio Fellipe
**Scope:** Documentation only. No code, infra, or chart changes.

## Purpose

Produce a comparison doc set that puts the project's current Argo Events
implementation side-by-side with an equivalent design built on GCP
Pub/Sub + Eventarc, both running on GKE. The doc set is a hybrid
artifact: neutral on macro architectural tradeoffs, opinionated on
operational and resource-burden tradeoffs (resource counts, IAM
sprawl, identity-binding overhead, port allocation). The closing
sections name where each stack wins for which axis but stop short of
prescribing a migration.

## Audience

Engineering team members familiar with the bookinfo project, Kubernetes,
and Helm. Familiarity with Pub/Sub, Eventarc, Crossplane, and GKE
Workload Identity is assumed at the API surface level. Concepts are
not re-explained; the doc set is a reference, not a tutorial.

## Out of scope

- Observability cross-cutting concerns (trace propagation through the
  event bus is its own concern and gets its own doc later).
- Schema registry / contract enforcement beyond what Pub/Sub Schemas
  natively offers.
- Multi-region failover or DR posture.
- Cost modeling beyond a qualitative line in the macro tradeoff matrix.
- Workload Identity Federation (the cross-project / non-GKE flavor).
  GKE Workload Identity (KSA → GSA via annotation) is the only
  identity model the doc covers, by user direction.
- Migration plan, runbooks, or any "how to switch" content. The doc is
  a comparison, not a transition guide.

## Outputs

A docs subtree under `docs/comparisons/argo-events-vs-pubsub-eventarc/`
containing six files:

| File | Purpose | Approx size |
|---|---|---|
| `00-overview.md` | Macro tradeoffs, glossary translation, opinionated resource-burden snapshot | 150-200 lines |
| `01-cqrs.md` | CQRS write-path: today vs GCP equivalent | 250-350 lines |
| `02-events-catalog.md` | `events.exposed` + `events.consumed`: today vs GCP | 250-350 lines |
| `03-dlq.md` | Argo `dlqTrigger` vs Pub/Sub `dead_letter_policy` | 150-200 lines |
| `04-ingestion-producer.md` | Producer-side (ingestion + details/reviews/ratings) | 120-180 lines |
| `05-resource-checklists.md` | Scenario-driven "what do I provision" checklists | 200-280 lines |

Each file uses the same internal pattern:

1. Argo Events approach (with diagram and resource enumeration)
2. Pub/Sub + Eventarc approach (with diagram and resource enumeration)
3. Side-by-side resource table
4. Crossplane YAML snippets where they ground the discussion
5. Tradeoffs (opinionated where ops/resource burden is the axis,
   neutral where macro architecture is the axis)

## Verified current state (source of truth for the doc)

Confirmed via `kubectl --context=k3d-bookinfo-local` on 2026-04-27.

### EventBus (1)

- `kafka` (namespace `bookinfo`) — Kafka-backed, broker
  `bookinfo-kafka-kafka-bootstrap.platform.svc.cluster.local:9092`,
  argo-events internal topic `argo-events`, version `4.2.0`.

### EventSources (9)

**Webhook EventSources (CQRS write-path) — 5:**

| EventSource | Owning service | Endpoint | Port | Method |
|---|---|---|---|---|
| `book-added` | details | `/v1/details` | 12000 | POST |
| `review-submitted` | reviews | `/v1/reviews` | 12000 | POST |
| `review-deleted` | reviews | `/v1/reviews/delete` | 12000 | POST |
| `rating-submitted` | ratings | `/v1/ratings` | 12000 | POST |
| `dlq-event-received` | dlqueue | `/v1/events` | 12000 | POST |

All five render on port 12000 as of the recent consolidation. The
per-service `deploy/<svc>/values-local.yaml` files still encode the
legacy distinct ports 12000-12004; the live cluster diverges from
those values. The doc records the live state and notes the drift in
passing in `01-cqrs.md`.

Each webhook EventSource also produces a `<name>-eventsource-svc`
ClusterIP Service object (so resource accounting per webhook
EventSource = 1 EventSource CR + 1 Deployment + 1 Service).

**Kafka EventSources (`events.exposed`) — 4:**

| EventSource | Owning service | Topic |
|---|---|---|
| `details-events` | details | `bookinfo_details_events` |
| `reviews-events` | reviews | `bookinfo_reviews_events` |
| `ratings-events` | ratings | `bookinfo_ratings_events` |
| `ingestion-raw-books-details` | ingestion | `raw_books_details` |

Producers publish to Kafka via franz-go directly. The Kafka
EventSource is a broker-to-EventBus bridge that downstream Sensors
depend on by `eventSourceName` + `eventName`. There is no GCP
equivalent of this construct — Pub/Sub subscriptions attach directly
to topics. This is called out in `02-events-catalog.md`.

### Sensors (6)

**CQRS sensors — 4:**

| Sensor | Webhook EventSources it depends on |
|---|---|
| `details-sensor` | `book-added` |
| `reviews-sensor` | `review-submitted`, `review-deleted` |
| `ratings-sensor` | `rating-submitted` |
| `dlqueue-sensor` | `dlq-event-received` |

`reviews-sensor` multiplexes two webhook EventSources for one service
(two endpoints in the chart's `cqrs.endpoints` map → one Sensor with
two dependencies + two triggers). This multiplexing is itself a
comparison point: a single Argo Sensor handles N endpoints for one
service cheaply; the GCP equivalent is N independent Subscriptions or
Eventarc Triggers. Surfaced in `01-cqrs.md`.

**Consumer sensors — 2:**

| Sensor | Dependencies (eventSource.eventName, ce_type filter) |
|---|---|
| `details-consumer-sensor` | `ingestion-raw-books-details.raw-books-details` (no ce_type filter) |
| `notification-consumer-sensor` | `details-events.events` (4 ce_type filters: `com.bookinfo.details.book-added`, `com.bookinfo.reviews.review-submitted`, `com.bookinfo.reviews.review-deleted`, `com.bookinfo.ratings.rating-submitted`) |

`notification-consumer-sensor` is the worst-case fan-in example used
throughout the doc. One Argo Sensor with four dependencies vs four
Pub/Sub Subscriptions or four Eventarc Triggers in the GCP design.

### DLQ surface

Single shared destination: webhook EventSource `dlq-event-received`
on port 12000, endpoint `/v1/events`, target service
`dlqueue-write.bookinfo.svc.cluster.local`. Every Sensor trigger in
the chart automatically renders a `dlqTrigger` block (gated by
`sensor.dlq.enabled`, default `true`) that posts a structured payload
including `sensor_name`, `failed_trigger`, `original_payload`, and
CloudEvents context keys. Dedup key on the dlqueue side is
`SHA-256(sensor_name + failed_trigger + payload)`.

## GCP equivalent — design assumptions

These assumptions ground the comparison and must be explicit so the
resource counts in `05-resource-checklists.md` are reproducible.

### Topology

- GKE cluster in the same project that hosts Pub/Sub and Eventarc.
- Crossplane installed cluster-side with the `provider-upjet-gcp`
  family (`Topic`, `Subscription`, `IAMMember`, `ServiceAccount`,
  `Trigger` from the eventarc API group). Compositions are not used
  in the doc — raw provider resources are shown for clarity, with a
  note that a Composition would compress the per-event boilerplate.
- Identity model: GKE Workload Identity only. Each app service has
  one KSA (the existing chart-managed `serviceAccountName`) annotated
  with `iam.gke.io/gcp-service-account=<gsa>@<project>.iam.gserviceaccount.com`,
  and the GSA grants `roles/iam.workloadIdentityUser` to the KSA
  principal. WIF (the cross-project federation flavor) is explicitly
  out of scope.

### Mapping decisions

| Argo Events concept | GCP equivalent the doc uses |
|---|---|
| EventBus (kafka) | None — Pub/Sub is the bus, no separate construct |
| Webhook EventSource (CQRS write) | Either (a) thin in-cluster publisher Go service that accepts POST and calls `Topic.Publish()`, or (b) Eventarc generic HTTP source. Doc shows (a) as the primary design and (b) as alternative |
| Kafka EventSource (`events.exposed` bridge) | None — subscribers attach directly to the Pub/Sub Topic |
| CQRS Sensor | One Pub/Sub push Subscription per `cqrs.endpoints` entry, OR one Eventarc Trigger per endpoint. Push destination = `<svc>-write` ClusterIP via Service URL |
| Consumer Sensor (with ce_type filter) | One push Subscription per ce_type with Subscription `filter` expression on `attributes.ce_type` |
| Sensor `retryStrategy` | Subscription `retryPolicy` (`minimumBackoff`, `maximumBackoff`) |
| Sensor `dlqTrigger` | Subscription `dead_letter_policy` → DLQ Topic → second Subscription that pushes to dlqueue-write |
| Producer service publishes to Kafka | Producer service publishes to Pub/Sub Topic via Pub/Sub Go client; auth via Workload Identity |

### Resource accounting basis

All resource counts in the doc set use these unit definitions so the
"+N vs +M" deltas are apples-to-apples:

- **Argo Events unit:** one CRD instance counts as 1, regardless of
  the Deployment/Service it spawns. Webhook EventSources are
  exceptionally noted as "1 EventSource + 1 Deployment + 1 Service"
  because the Service is reachable inside the cluster and is part of
  the surface area an operator sees.
- **GCP unit:** one Crossplane managed resource (or directly-applied
  GCP resource) counts as 1. IAM bindings count separately. KSA
  annotations are not counted as resources but are flagged when
  required.

## Diagrams

Each pattern file gets two mermaid diagrams (current and GCP) using
the same node shapes so visual comparison is direct:

- Box (`[ ]`) = app Service / Deployment
- Stadium (`( )`) = managed messaging primitive (Kafka topic, Pub/Sub
  topic, EventBus, Subscription)
- Subroutine (`[[ ]]`) = orchestrating CRD / GCP construct (Sensor,
  Trigger, EventSource)
- Cylinder (`[( )]`) = persistent store (Postgres, dead-letter)

`00-overview.md` includes the pair of overall-stack diagrams.
Per-pattern diagrams live in their respective files.

## Style and content rules

- All YAML snippets are illustrative and concrete to a real example
  from the project (e.g. `book-added`, `notification` consumer
  fan-in). No placeholders like `<your-event>`.
- Crossplane YAML uses the upjet-gcp provider API versions
  (`pubsub.gcp.upbound.io/v1beta1`, `eventarc.gcp.upbound.io/v1beta1`,
  `cloudplatform.gcp.upbound.io/v1beta1`, `iam.gcp.upbound.io/v1beta1`).
  When versions are unstable in upjet-gcp, the doc notes the version
  used and provides a footnote.
- Per-section tradeoff tables (in `01`-`04`) use three columns: axis,
  Argo Events, Pub/Sub + Eventarc. The verdict lives in a short
  narrative paragraph that follows the table and leads with the
  conclusion.
- The macro tradeoff matrix in `00-overview.md` is the exception:
  four columns — axis, Argo Events, Pub/Sub + Eventarc, verdict
  ("argo wins" / "GCP wins" / "depends — see notes"). The matrix
  avoids a single-line summary verdict on the entire stack.
- Resource tables use four columns: resource, Argo column, GCP
  column, notes.

## Closing structure (`00-overview.md` macro tradeoff matrix)

The matrix is the doc set's most visible artifact. Axes:

1. Portability across clouds / on-prem
2. Coupling to GKE control plane
3. Day-2 ops surface (CRDs to learn, dashboards to watch, identity
   plumbing)
4. Identity model uniformity (single chart-managed KSA vs per-event
   GSAs and bindings)
5. Schema enforcement (none vs Pub/Sub Schemas)
6. Replay model (Kafka offset / dlqueue replay vs Pub/Sub seek + DLQ
   resubscribe)
7. Latency profile (in-cluster Sensor vs Pub/Sub push round-trip)
8. Cost model (cluster compute for Kafka + sensors vs per-message GCP
   billing)
9. Vendor lock-in
10. Observability story (sensor traces, EventSource span coverage vs
    Cloud Trace + Cloud Logging integration)

Each axis is rated with a short verdict ("argo wins", "GCP wins",
"depends — see notes") in the matrix's fourth column. The matrix
avoids giving a single-line summary verdict on the entire stack.

## Validation gates

Before the doc set is merged:

1. Resource counts in `05-resource-checklists.md` cross-check against
   the kubectl-verified inventory above.
2. Each Crossplane YAML snippet renders against the `provider-upjet-gcp`
   schema (visual check — no actual apply needed since this is a
   doc).
3. All mermaid diagrams render in GitHub's mermaid renderer.
4. Glossary translation table in `00-overview.md` covers every term
   used elsewhere in the doc set (no surprise jargon).

## Risks and notes

- Crossplane provider API versions for `provider-upjet-gcp` evolve
  faster than the doc set. This doc set pins to
  `upbound/provider-gcp@v2.5.0`; each YAML snippet cites that
  pinned version in a comment header. Downstream readers should
  re-validate against newer releases.
- Pub/Sub `filter` expression syntax has limits (no nested attribute
  paths, no string functions); some `ce_type` patterns the project
  uses today (exact match) translate cleanly, but if any future
  filter needs prefix matching the doc must flag the gap. The
  current four ce_types in the notification fan-in are all exact
  matches, so this risk is theoretical for now.
- Eventarc Trigger destination URLs to GKE workloads require either
  the destination to be exposed via a public LB or via Eventarc's
  GKE-internal channels. The doc shows the GKE-internal channel as
  the primary path (matches the project's "no public ingress for
  internal traffic" posture).

## Next steps

After user review of this design:

1. Invoke the `superpowers:writing-plans` skill to produce a phased
   implementation plan covering the six files, file order,
   per-file diagram and table inventory, and review checkpoints.
2. Implement per the plan; commit each file or group of files
   independently with conventional-commit scopes
   (`docs(comparisons): ...`).
