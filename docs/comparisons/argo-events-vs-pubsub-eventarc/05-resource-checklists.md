# Resource Checklists by Scenario

For each common change (expose a new event, consume one, add a new CQRS service, enable DLQ, bootstrap a fresh producer), the exact list of resources to provision in each stack and the resulting delta count.

## Reading conventions

Each scenario lists the operator-side artifacts to add for each stack. **K8s objects** count = CRs + the Deployments and Services they auto-create. **GCP-side objects** count = Crossplane managed resources. **IAM bindings** count separately. **KSA annotations** are flagged but not counted.

Counts assume the relevant shared infrastructure is already in place (EventBus / DLQ Topic / chart KSA pattern).

## Scenario A — Service exposes a new event

Example: ratings adds a `rating-deleted` event on its existing topic.

### Argo Events

- Add an entry under `events.exposed` in `deploy/ratings/values-local.yaml`:

  ```yaml
  events:
    exposed:
      events:                 # existing
        topic: bookinfo_ratings_events
  ```

  No new entry needed if the topic and Kafka EventSource already exist (the new event is a new ce_type on the existing topic).

- The producer code emits the new ce_type via franz-go.

**K8s objects added: 0.** No new CRs.

### Pub/Sub + Eventarc

If reusing the existing `bookinfo-ratings-events` Topic:

- Producer code emits with `attributes.ce_type = "com.bookinfo.ratings.rating-deleted"`.

**GCP-side objects added: 0.** **IAM bindings added: 0.**

If you split per ce_type into its own Topic instead:

- 1 new `Topic` (Crossplane).
- 1 new `TopicIAMMember` granting the existing `ratings-publisher` GSA `roles/pubsub.publisher` on the new Topic.

**GCP-side objects added: 2.** **IAM bindings added: 1.**

**Delta:** argo +0 / GCP +0 (per-stream model) or +2 GCP-side + 1 IAM (per-ce_type model).

## Scenario B — Service consumes a new event with a ce_type filter

Example: notification adds a 5th ce_type, `com.bookinfo.reviews.review-edited`, sourced from the existing `reviews-events` stream.

### Argo Events

- Add to `deploy/notification/values-local.yaml` under `events.consumed`:

  ```yaml
  review-edited:
    eventSourceName: reviews-events
    eventName: events
    filter:
      ceType: com.bookinfo.reviews.review-edited
    triggers:
      - name: notify-review-edited
        url: self
        path: /v1/notifications
        method: POST
        payload:
          - src: { dependencyName: review-edited-dep, dataKey: body.review_id }
            dest: subject
          - src: { value: "Review edited" }
            dest: body
          - src: { value: "system@bookinfo" }
            dest: recipient
          - src: { value: "email" }
            dest: channel
    dlq:
      enabled: true
      url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12000/v1/events"
  ```

The chart re-renders the existing `notification-consumer-sensor` with one more dependency and one more trigger. **K8s objects added: 0.**

### Pub/Sub + Eventarc

- 1 new `Subscription` on `bookinfo-reviews-events` with filter `attributes.ce_type = "com.bookinfo.reviews.review-edited"` + `pushConfig` to `notification-write` + `deadLetterPolicy` to the shared DLQ Topic.
- 1 new `SubscriptionIAMMember` granting `roles/pubsub.subscriber` to the existing `notification-subscriber` GSA.

**GCP-side objects added: 1.** **IAM bindings added: 1.**

**Delta:** argo +0 / GCP +1 + 1 IAM.

## Scenario C — New CQRS write-path service with 1 endpoint

Example: a new `wishlist` service with one CQRS endpoint `/v1/wishlists` listening for `wishlist-added`.

### Argo Events

Helm values changes:

```yaml
cqrs:
  enabled: true
  endpoints:
    wishlist-added:
      port: 12000
      triggers:
        - name: create-wishlist
          url: self
          payload: [passthrough]
```

Chart-rendered cluster resources:

- 1 webhook `EventSource` (`wishlist-added`) + the auto-created Deployment + ClusterIP Service (`wishlist-added-eventsource-svc`).
- 1 `Sensor` (`wishlist-sensor`) with one dependency on `wishlist-added` and one HTTP trigger to `wishlist-write`.
- 1 `HTTPRoute` matching `POST /v1/wishlists` and forwarding to the EventSource Service.

**K8s objects added: 3 CRs (EventSource, Sensor, HTTPRoute) + 1 Deployment + 1 Service (auto-created).**

### Pub/Sub + Eventarc

- 1 `Topic` (`cqrs-wishlist-added`).
- 1 `ServiceAccount` (GSA) for the wishlist-write push delivery identity.
- 1 `ServiceAccountIAMMember` (WI binding) for the new GSA + KSA pair.
- 1 `Trigger` (Eventarc) targeting the GKE destination `wishlist-write` at path `/v1/wishlists`.
- 1 publisher-adapter Service (in-cluster) OR Eventarc generic HTTP source for the inbound POST.
- 1 KSA annotation on the chart-managed `wishlist` ServiceAccount.

**GCP-side objects added: 4** (Topic, GSA, WI binding, Trigger). **IAM bindings added: 0** beyond what the WI binding covers (Eventarc Trigger spec ties the GSA to the destination implicitly). **K8s objects added in-cluster: 1** (publisher adapter Service).

**Delta:** argo: 3 CRs + 1 Deployment + 1 Service / GCP: 4 GCP-side + 1 in-cluster Service + KSA annotation.

## Scenario D — Enable DLQ on a new consumer

Assume the consumer exists and the shared DLQ destination (Argo `dlq-event-received` EventSource / Pub/Sub `bookinfo-dlq` Topic) is already provisioned.

### Argo Events

Helm values changes:

```yaml
events:
  consumed:
    <event-name>:
      dlq:
        enabled: true              # already default true
        url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12000/v1/events"
```

Or simply rely on `sensor.dlq.enabled: true` (default) — the chart auto-renders the `dlqTrigger` block on every Sensor trigger. **K8s objects added: 0.**

### Pub/Sub + Eventarc

Append to the consumer's `Subscription` manifest:

```yaml
spec:
  forProvider:
    deadLetterPolicy:
      deadLetterTopicRef:
        name: bookinfo-dlq
      maxDeliveryAttempts: 5
```

Plus a one-time IAM binding granting the project's Pub/Sub service agent `roles/pubsub.publisher` on `bookinfo-dlq` (already in place if any other consumer DLQ uses the same Topic).

**GCP-side objects added: 0.** **IAM bindings added: 0** (assuming the one-time service-agent binding is in place).

**Delta:** argo +0 / GCP +0 (after one-time setup). Both stacks are equally cheap once the DLQ destination exists; argo wins on the very first DLQ enablement (no service-agent IAM dance).

## Scenario E — Bootstrap a brand-new producer service

Example: a new `inventory` service that polls an external API and publishes to a new event stream `inventory-events`.

### Argo Events

Helm values + chart-rendered cluster resources:

- 1 Kafka `EventSource` (`inventory-events`).
- The service Deployment + Service (auto-created by the chart).
- 1 chart-managed KSA (auto-created).

The Kafka topic `bookinfo_inventory_events` is **not** provisioned as a CR — Strimzi's broker auto-creates it on first publish from franz-go.

**K8s objects added: 1 EventSource + 1 Deployment + 1 Service + 1 ServiceAccount = 4.**

### Pub/Sub + Eventarc

GCP-side, all via Crossplane:

- 1 `Topic` (`bookinfo-inventory-events`).
- 1 `ServiceAccount` (GSA, e.g. `inventory-publisher`).
- 1 `TopicIAMMember` (`roles/pubsub.publisher`).
- 1 `ServiceAccountIAMMember` (WI binding).

In-cluster:

- The service Deployment + Service (chart).
- 1 chart-managed KSA + 1 KSA annotation pointing at the GSA.

**GCP-side objects added: 4.** **IAM bindings added: 1.** **K8s objects added: 3** (Deployment, Service, KSA — same as before, plus the annotation).

**Delta:** argo: 4 K8s objects (1 messaging-related: EventSource only) / GCP: 4 GCP-side + 1 IAM + 3 K8s + KSA annotation.

The headline number — **5 messaging-related objects on GCP (4 GCP-side + 1 IAM binding) vs 1 on argo for a fresh producer** — is the resource-burden gap that scales with producer count. For the four producers in the project today (details, reviews, ratings, ingestion), GCP would have provisioned **+12 extra GCP-side artifacts and +4 extra IAM bindings** that argo collapses into in-cluster RBAC, against the **4 Kafka EventSources** that already exist.
