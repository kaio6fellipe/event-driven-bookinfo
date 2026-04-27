# values-generated.yaml Sharpening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate `cqrs.endpoints`, `events.exposed`, and `cqrs.eventBusName` from per-service `values-local.yaml`. specgen emits `topic`; chart applies defaults for port (12000), trigger shape (passthrough), and event-bus name (kafka).

**Architecture:** Three-stage migration: (1) extend specgen to emit `topic` into `values-generated.yaml`. (2) Add chart defaults using `default <new-key> <old-key>` so both shapes work simultaneously. (3) Strip values-local files; drop fallbacks; bump chart version. Final state: chart owns operational defaults; specgen owns spec-derived values; values-local owns service-identity + infra-environment values only.

**Tech Stack:** Go (specgen), Helm/Go-templates (chart), YAML (values files), shell (Makefile + kubectl + helm + gh).

**Spec:** `docs/superpowers/specs/2026-04-27-values-generated-sharpening-design.md`

**Branch:** `feat/values-generated-sharpening`

**Spec deviation discovered during planning:** The spec called for refactoring the reviews delete handler to accept the full request body. Verification of `services/productpage/internal/client/reviews.go:91-94` and `services/reviews/internal/adapter/inbound/http/handler.go:126-143` shows productpage already POSTs `{"review_id": "..."}` and the handler already accepts that exact shape. The sensor's `body.review_id → review_id` transform is a literal no-op. Switching to passthrough is byte-identical. **No handler refactor task in this plan.** Removing the trigger override in step 5 is sufficient.

---

## File Structure

### Created
None. All work modifies existing files.

### Modified

**specgen:**
- `tools/specgen/internal/values/values.go` — emit `topic` under `events.exposed.<key>` from `walker.DescriptorInfo.Topic`. Fail if descriptors sharing an `ExposureKey` disagree on `Topic`.
- `tools/specgen/internal/values/values_test.go` — add table cases (single-descriptor topic, multi-descriptor shared-topic, divergent-topic failure, no-port/no-triggers/no-eventBusName invariant).

**Chart:**
- `charts/bookinfo-service/Chart.yaml` — `version: 0.4.0` → `0.5.0` (final breaking step only).
- `charts/bookinfo-service/values.yaml` — add `cqrs.eventSource.port: 12000`, add `events.busName: kafka`, drop `cqrs.eventBusName: kafka` (final step), update inline example comments.
- `charts/bookinfo-service/templates/eventsource.yaml` — replace `$endpoint.port` and `$.Values.cqrs.eventBusName` with new keys (additive then breaking).
- `charts/bookinfo-service/templates/eventsource-service.yaml` — replace `$endpoint.port` (additive then breaking).
- `charts/bookinfo-service/templates/httproute.yaml` — replace `$endpoint.port` (additive then breaking).
- `charts/bookinfo-service/templates/sensor.yaml` — synthesize default passthrough trigger when `$endpoint.triggers` absent; replace eventBusName + port lookups; loosen `$hasEndpoints` gate.
- `charts/bookinfo-service/templates/kafka-eventsource.yaml` — replace per-event `eventBusName` with `events.busName`.
- `charts/bookinfo-service/templates/consumer-sensor.yaml` — replace `cqrs.eventBusName` with `events.busName`.
- `charts/bookinfo-service/ci/values-details-consumer.yaml` — strip soon-stale keys.
- `charts/bookinfo-service/ci/values-dlqueue-no-dlq.yaml` — strip soon-stale keys.
- `charts/bookinfo-service/ci/values-ingestion-kafka.yaml` — strip soon-stale keys.
- `charts/bookinfo-service/ci/values-ratings-cqrs.yaml` — strip soon-stale keys.
- `charts/bookinfo-service/ci/values-productpage-simple.yaml` — verify (productpage has no cqrs/events).
- `charts/bookinfo-service/ci/values-details-postgres.yaml` — strip soon-stale keys.

**values-local (per service):**
- `deploy/details/values-local.yaml` — drop `cqrs.endpoints`, `events.exposed`. Update dlq url port 12004 → 12000.
- `deploy/ratings/values-local.yaml` — drop `cqrs.endpoints`, `events.exposed`. Update dlq url port 12004 → 12000.
- `deploy/reviews/values-local.yaml` — drop `cqrs.endpoints`, `events.exposed`. Update dlq url port 12004 → 12000.
- `deploy/dlqueue/values-local.yaml` — drop `cqrs.endpoints`. (No `events.exposed`.)
- `deploy/ingestion/values-local.yaml` — drop `events.exposed`. (No `cqrs.endpoints`.)
- `deploy/notification/values-local.yaml` — update 4× dlq urls 12004 → 12000. (No `cqrs.endpoints`, no `events.exposed`.)
- `deploy/productpage/values-local.yaml` — verify; no changes expected.

**Documentation:**
- `CLAUDE.md` — section "Helm Events Configuration" (line 68 area): update example to drop `eventBusName` from `events.exposed`. Mention specgen now emits `topic`. Note that port and trigger shape are chart defaults.
- `services/details/internal/adapter/outbound/kafka/exposed.go` — header comment line 5–9 already accurate; verify wording consistent post-change.
- `services/ratings/internal/adapter/outbound/kafka/exposed.go` — same.
- `services/reviews/internal/adapter/outbound/kafka/exposed.go` — same.
- `services/ingestion/internal/adapter/outbound/kafka/exposed.go` — same.
- `services/details/internal/adapter/inbound/http/endpoints.go` — header comments unchanged.

### Test files (existing, modified)
- `tools/specgen/internal/values/values_test.go` — augmented (above).
- `services/dlqueue/internal/core/service/dlq_service_test.go` — unchanged (uses port 12001 in unit fixtures unrelated to deployment).
- `services/dlqueue/internal/core/domain/dlq_event_test.go` — unchanged (same).

---

## Phase 1 — specgen extension (additive)

### Task 1: Emit `topic` under `events.exposed.<key>`

**Files:**
- Modify: `tools/specgen/internal/values/values.go` (function `buildExposedNode` lines 79–143)
- Test: `tools/specgen/internal/values/values_test.go`

- [ ] **Step 1: Read current `values_test.go` to confirm test pattern**

```bash
cat tools/specgen/internal/values/values_test.go
```

Expected: existing table-driven tests using `walker.DescriptorInfo` and `Build(Input)`. Identify the table struct so new cases follow the same shape.

- [ ] **Step 2: Add failing test case for single-descriptor topic emission**

Add this case to the existing test table in `tools/specgen/internal/values/values_test.go` (preserve surrounding style):

```go
{
    name: "single descriptor emits topic",
    in: Input{
        ServiceName: "details",
        Exposed: []walker.DescriptorInfo{
            {
                Name:        "book-added",
                ExposureKey: "events",
                Topic:       "bookinfo_details_events",
                CEType:      "com.bookinfo.details.book-added",
                ContentType: "application/json",
            },
        },
    },
    want: `# DO NOT EDIT — generated by tools/specgen from
#   services/details/internal/adapter/outbound/kafka/exposed.go
# Run ` + "`make generate-specs`" + ` to refresh.
events:
  exposed:
    events:
      topic: bookinfo_details_events
      contentType: application/json
      eventTypes:
        - com.bookinfo.details.book-added
`,
},
```

- [ ] **Step 3: Run test, verify it fails**

```bash
go test ./tools/specgen/internal/values/...
```

Expected: FAIL — actual output is missing `topic: bookinfo_details_events` line.

- [ ] **Step 4: Add `topic` to `exposedGroup` and emit it in `buildExposedNode`**

Edit `tools/specgen/internal/values/values.go`:

Change the `exposedGroup` struct (lines 38–42):

```go
type exposedGroup struct {
    contentType string   // ContentType of the first descriptor (sorted by Name)
    topic       string   // Topic of the first descriptor (sorted by Name); descriptors sharing an ExposureKey must agree
    ceTypes     []string // Union of all CETypes in the group, sorted alphabetically
}
```

In `buildExposedNode`, after the `g.contentType = descs[0].ContentType` line (line 118) add agreement enforcement and topic capture:

```go
if len(descs) > 0 {
    g.contentType = descs[0].ContentType
    g.topic = descs[0].Topic
    for _, d := range descs[1:] {
        if d.Topic != g.topic {
            // Will be returned via Build() — buildExposedNode signature must change.
        }
    }
}
```

The check above flags a problem the function can't currently report because it returns `*yaml.Node` only. Update `buildExposedNode` to return `(*yaml.Node, error)`:

```go
func buildExposedNode(exposed []walker.DescriptorInfo) (*yaml.Node, error) {
    if len(exposed) == 0 {
        return nil, nil
    }
    // ... existing grouping code ...
    for key, g := range groupMap {
        var descs []walker.DescriptorInfo
        for _, d := range exposed {
            dk := d.ExposureKey
            if dk == "" {
                dk = d.Name
            }
            if dk == key {
                descs = append(descs, d)
            }
        }
        sort.Slice(descs, func(i, j int) bool {
            return descs[i].Name < descs[j].Name
        })
        if len(descs) > 0 {
            g.contentType = descs[0].ContentType
            g.topic = descs[0].Topic
            for _, d := range descs[1:] {
                if d.Topic != g.topic {
                    return nil, fmt.Errorf("events.exposed.%s: descriptors disagree on Topic (%q vs %q)", key, g.topic, d.Topic)
                }
            }
        }
        sort.Strings(g.ceTypes)
    }
    // ... existing emit code ...
}
```

In the emit loop (around line 128–138), add `topic` BEFORE `contentType` so the YAML output ordering is `topic, contentType, eventTypes`:

```go
exposedNode := yamlutil.Mapping()
for _, key := range groupOrder {
    g := groupMap[key]
    entryNode := yamlutil.Mapping()
    yamlutil.AddScalar(entryNode, "topic", g.topic)
    yamlutil.AddScalar(entryNode, "contentType", g.contentType)
    // ... eventTypes ...
}
```

In `Build` (line 161), update the call site:

```go
eventsNode, err := buildExposedNode(in.Exposed)
if err != nil {
    return nil, err
}
if eventsNode != nil {
    yamlutil.AddMapping(docNode, "events", eventsNode)
}
```

- [ ] **Step 5: Run test, verify it passes**

```bash
go test ./tools/specgen/internal/values/...
```

Expected: PASS for the new case and all existing cases.

- [ ] **Step 6: Add divergent-topic failure case to test table**

```go
{
    name: "divergent topic same exposure key fails",
    in: Input{
        ServiceName: "reviews",
        Exposed: []walker.DescriptorInfo{
            {Name: "review-submitted", ExposureKey: "events", Topic: "topic_a", CEType: "com.bookinfo.reviews.review-submitted", ContentType: "application/json"},
            {Name: "review-deleted",   ExposureKey: "events", Topic: "topic_b", CEType: "com.bookinfo.reviews.review-deleted",   ContentType: "application/json"},
        },
    },
    wantErr: `events.exposed.events: descriptors disagree on Topic`,
},
```

If the existing test struct doesn't already have a `wantErr` field, add one. Update the test driver to assert error substring when `wantErr != ""`:

```go
got, err := Build(tt.in)
if tt.wantErr != "" {
    if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
        t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
    }
    return
}
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
```

- [ ] **Step 7: Add multi-descriptor shared-topic case (reviews)**

```go
{
    name: "multi descriptor shared topic and exposure key",
    in: Input{
        ServiceName: "reviews",
        Exposed: []walker.DescriptorInfo{
            {Name: "review-submitted", ExposureKey: "events", Topic: "bookinfo_reviews_events", CEType: "com.bookinfo.reviews.review-submitted", ContentType: "application/json"},
            {Name: "review-deleted",   ExposureKey: "events", Topic: "bookinfo_reviews_events", CEType: "com.bookinfo.reviews.review-deleted",   ContentType: "application/json"},
        },
    },
    want: `# DO NOT EDIT — generated by tools/specgen from
#   services/reviews/internal/adapter/outbound/kafka/exposed.go
# Run ` + "`make generate-specs`" + ` to refresh.
events:
  exposed:
    events:
      topic: bookinfo_reviews_events
      contentType: application/json
      eventTypes:
        - com.bookinfo.reviews.review-deleted
        - com.bookinfo.reviews.review-submitted
`,
},
```

- [ ] **Step 8: Add invariant-assertion case (no port / triggers / eventBusName ever emitted)**

The existing tests already implicitly assert this by comparing exact YAML output, but add an explicit case to make the invariant unmistakable:

```go
{
    name: "cqrs endpoint never emits port or triggers",
    in: Input{
        ServiceName: "details",
        Endpoints: []walker.EndpointInfo{
            {Method: "POST", Path: "/v1/details", EventName: "book-added"},
        },
    },
    want: `# DO NOT EDIT — generated by tools/specgen from
#   services/details/internal/adapter/inbound/http/endpoints.go
# Run ` + "`make generate-specs`" + ` to refresh.
cqrs:
  endpoints:
    book-added:
      method: POST
      endpoint: /v1/details
`,
},
```

- [ ] **Step 9: Run all values tests, verify all pass**

```bash
go test ./tools/specgen/internal/values/... -v
```

Expected: all cases PASS.

- [ ] **Step 10: Run full specgen test suite to ensure no regression**

```bash
go test ./tools/specgen/...
```

Expected: PASS.

- [ ] **Step 11: Regenerate values-generated.yaml files via specgen**

```bash
make generate-specs
```

- [ ] **Step 12: Diff the regenerated values files**

```bash
git diff -- 'deploy/*/values-generated.yaml'
```

Expected: each of `deploy/{details,ratings,reviews,ingestion}/values-generated.yaml` gains a `topic: <name>` line under `events.exposed.<key>`. `deploy/{dlqueue,notification}/values-generated.yaml` unchanged (no Exposed descriptors). `deploy/productpage/values-generated.yaml` does not exist.

- [ ] **Step 13: Verify chart still installs cleanly with new generated files (regression check)**

```bash
make helm-lint
```

Expected: PASS for all per-service values combinations. The chart still reads `events.exposed.<key>.topic` from values-local (which still has it); the new line in values-generated is additive and merges harmlessly.

- [ ] **Step 14: Commit**

```bash
git add tools/specgen/internal/values/values.go \
        tools/specgen/internal/values/values_test.go \
        deploy/details/values-generated.yaml \
        deploy/ratings/values-generated.yaml \
        deploy/reviews/values-generated.yaml \
        deploy/ingestion/values-generated.yaml
git commit -s -m "feat(specgen): emit topic under events.exposed.<key>

Captures Topic field from kafka.Exposed descriptors into the generated
helm values. Asserts descriptors sharing an ExposureKey agree on Topic
(unique per group). values-local still authoritative for now; chart
reads merged result. Migration-safe additive change."
```

---

## Phase 2 — Chart additive defaults (backward-compatible)

### Task 2: Add chart-default keys

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`

- [ ] **Step 1: Read current chart values.yaml structure**

```bash
sed -n '85,165p' charts/bookinfo-service/values.yaml
```

Expected: see existing `cqrs:` and `events:` sections, line numbers match the spec.

- [ ] **Step 2: Add `cqrs.eventSource.port: 12000` to chart values.yaml**

Edit `charts/bookinfo-service/values.yaml`. Inside the `cqrs:` block (between `eventBusName: kafka` line ~94 and `endpoints: {}` line ~95), add:

```yaml
  # -- Default port for webhook EventSource pods (Argo Events allows multiple
  # webhooks on the same port within a single EventSource pod, and pods are
  # isolated, so 12000 is reused across every CQRS endpoint).
  eventSource:
    port: 12000
```

- [ ] **Step 3: Add `events.busName: kafka` to chart values.yaml**

Inside the existing `events:` block (line ~152, before `kafka:`), add:

```yaml
events:
  # -- Default Argo Events EventBus name. Used by EventSource (CQRS webhook),
  # kafka EventSource (exposed events), and Sensor (CQRS + consumer).
  busName: kafka
  kafka:
    broker: ""
  exposed: {}
  consumed: {}
```

(Note: `cqrs.eventBusName: kafka` stays for now — chart is in dual-key mode during phase 2.)

- [ ] **Step 4: Update example comment block under `events.exposed`**

Edit the comment lines ~155–161 of `charts/bookinfo-service/values.yaml` to drop the per-event `eventBusName` example:

```yaml
  exposed: {}
    # event-name:
    #   topic: kafka_topic_name
    #   contentType: application/json
    #   eventTypes:                        # optional, rendered as bookinfo.io/emitted-ce-types annotation on the EventSource
    #     - com.bookinfo.<service>.<event>
```

- [ ] **Step 5: Lint chart**

```bash
make helm-lint
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -s -m "feat(chart): add cqrs.eventSource.port and events.busName defaults

Introduces the new keys without removing the old ones. Templates do not
yet read them (next task)."
```

### Task 3: Templates read new keys with `default <new> <old>` fallback

**Files:**
- Modify: `charts/bookinfo-service/templates/eventsource.yaml`
- Modify: `charts/bookinfo-service/templates/eventsource-service.yaml`
- Modify: `charts/bookinfo-service/templates/httproute.yaml`
- Modify: `charts/bookinfo-service/templates/sensor.yaml`
- Modify: `charts/bookinfo-service/templates/kafka-eventsource.yaml`
- Modify: `charts/bookinfo-service/templates/consumer-sensor.yaml`

- [ ] **Step 1: Update `templates/eventsource.yaml`**

Replace line 11:

```yaml
  eventBusName: {{ $.Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ default $.Values.events.busName $.Values.cqrs.eventBusName }}
```

Replace line 23:

```yaml
      port: {{ $endpoint.port | quote }}
```

with:

```yaml
      port: {{ default $.Values.cqrs.eventSource.port $endpoint.port | quote }}
```

Note: `default A B` returns B when B is non-empty, A otherwise. So when values-local provides `$endpoint.port`, that wins; when absent, falls back to chart default. Same pattern below.

- [ ] **Step 2: Update `templates/eventsource-service.yaml`**

Replace lines 14–15:

```yaml
    - port: {{ $endpoint.port }}
      targetPort: {{ $endpoint.port }}
```

with:

```yaml
    - port: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
      targetPort: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
```

- [ ] **Step 3: Update `templates/httproute.yaml`**

Replace line 27:

```yaml
          port: {{ $endpoint.port }}
```

with:

```yaml
          port: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
```

- [ ] **Step 4: Update `templates/sensor.yaml`**

Replace line 18:

```yaml
  eventBusName: {{ .Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ default .Values.events.busName .Values.cqrs.eventBusName }}
```

Replace line 93 inside the DLQ url builder:

```yaml
      {{- $esURL := printf "http://%s-eventsource-svc.%s.svc.cluster.local:%v%s" $eventName $.Release.Namespace $endpoint.port $endpoint.endpoint }}
```

with:

```yaml
      {{- $esPort := default $.Values.cqrs.eventSource.port $endpoint.port }}
      {{- $esURL := printf "http://%s-eventsource-svc.%s.svc.cluster.local:%v%s" $eventName $.Release.Namespace $esPort $endpoint.endpoint }}
```

- [ ] **Step 5: Update `templates/kafka-eventsource.yaml`**

Replace line 15:

```yaml
  eventBusName: {{ default "kafka" $event.eventBusName }}
```

with:

```yaml
  eventBusName: {{ default $.Values.events.busName $event.eventBusName }}
```

- [ ] **Step 6: Update `templates/consumer-sensor.yaml`**

Replace line 20:

```yaml
  eventBusName: {{ .Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ default .Values.events.busName .Values.cqrs.eventBusName }}
```

- [ ] **Step 7: Lint chart**

```bash
make helm-lint
```

Expected: PASS for all per-service values files.

- [ ] **Step 8: Verify rendered manifests are byte-identical to main for each service**

```bash
for svc in details ratings reviews dlqueue ingestion notification productpage; do
  echo "=== $svc ==="
  helm template "$svc" charts/bookinfo-service \
    -f "deploy/$svc/values-local.yaml" \
    -f "deploy/$svc/values-generated.yaml" 2>/dev/null > "/tmp/render-$svc-new.yaml" || \
    helm template "$svc" charts/bookinfo-service \
      -f "deploy/$svc/values-local.yaml" 2>/dev/null > "/tmp/render-$svc-new.yaml"
done

# Compare against main
git stash
for svc in details ratings reviews dlqueue ingestion notification productpage; do
  helm template "$svc" charts/bookinfo-service \
    -f "deploy/$svc/values-local.yaml" \
    -f "deploy/$svc/values-generated.yaml" 2>/dev/null > "/tmp/render-$svc-main.yaml" || \
    helm template "$svc" charts/bookinfo-service \
      -f "deploy/$svc/values-local.yaml" 2>/dev/null > "/tmp/render-$svc-main.yaml"
done
git stash pop

for svc in details ratings reviews dlqueue ingestion notification productpage; do
  diff -u "/tmp/render-$svc-main.yaml" "/tmp/render-$svc-new.yaml" || echo "DRIFT in $svc"
done
```

Expected: no diff for any service. Old keys still win via `default`.

- [ ] **Step 9: Commit**

```bash
git add charts/bookinfo-service/templates/
git commit -s -m "feat(chart): templates read new keys with fallback to old keys

cqrs.eventSource.port, events.busName, and per-event topic now flow
through chart defaults; existing values-local files still override.
Rendered manifests byte-identical to main."
```

### Task 4: Sensor synthesizes default passthrough trigger

**Files:**
- Modify: `charts/bookinfo-service/templates/sensor.yaml`

- [ ] **Step 1: Loosen `$hasEndpoints` gate (lines 2–9)**

Today the gate requires endpoints to have `$ep.triggers`. After this task, missing triggers are synthesized, so the gate must accept any endpoint:

Replace lines 2–9:

```yaml
{{- $hasEndpoints := false }}
{{- $endpointCount := 0 }}
{{- range $_, $ep := .Values.cqrs.endpoints }}
{{- if $ep.triggers }}
{{- $hasEndpoints = true }}
{{- $endpointCount = add $endpointCount 1 }}
{{- end }}
{{- end }}
```

with:

```yaml
{{- $endpointCount := len .Values.cqrs.endpoints }}
{{- $hasEndpoints := gt $endpointCount 0 }}
```

- [ ] **Step 2: Synthesize default trigger when `$endpoint.triggers` absent**

Inside the trigger range loop. Replace line 35:

```yaml
    {{- range $eventName, $endpoint := $.Values.cqrs.endpoints }}
    {{- range $trigger := $endpoint.triggers }}
```

with:

```yaml
    {{- range $eventName, $endpoint := $.Values.cqrs.endpoints }}
    {{- $triggers := $endpoint.triggers }}
    {{- if not $triggers }}
    {{- $triggers = list (dict "name" $eventName "url" "self" "payload" (list "passthrough")) }}
    {{- end }}
    {{- range $trigger := $triggers }}
```

- [ ] **Step 3: Lint chart**

```bash
make helm-lint
```

Expected: PASS.

- [ ] **Step 4: Verify rendered manifests still byte-identical to main**

Repeat the diff harness from Task 3 Step 8.

Expected: no diff. Existing values-local still provides `triggers`, so synthesis path doesn't activate.

- [ ] **Step 5: Render with a synthetic empty-triggers value to prove synthesis works**

```bash
cat > /tmp/synth-test.yaml <<'EOF'
serviceName: details
fullnameOverride: details
cqrs:
  enabled: true
  endpoints:
    book-added:
      method: POST
      endpoint: /v1/details
      # no port, no triggers — chart must default both
sensor:
  dlq:
    enabled: false
gateway:
  name: default-gw
  namespace: platform
  sectionName: web
events:
  kafka:
    broker: "kafka:9092"
EOF

helm template details charts/bookinfo-service -f /tmp/synth-test.yaml \
  | grep -A 8 'kind: Sensor' | head -30
```

Expected: rendered Sensor includes a trigger `name: book-added` with `payload` array containing one entry that maps `dataKey: body` to dest `""` (the passthrough form).

- [ ] **Step 6: Commit**

```bash
git add charts/bookinfo-service/templates/sensor.yaml
git commit -s -m "feat(chart): synthesize default passthrough trigger when absent

Endpoints declared in values-generated.yaml without triggers now produce
a sensor trigger {name: <eventName>, url: self, payload: [passthrough]}.
Existing values-local-authored triggers continue to win."
```

---

## Phase 3 — Strip values-local files (per-service)

> Each task in this phase: drop `cqrs.endpoints` block, drop `events.exposed` block, drop `cqrs.eventBusName` if present, update any dlq url ports 12001–12004 → 12000. After each task, render manifests and verify byte-identical to main (manifests should not change because chart defaults match prior literal values).

### Task 5: details — strip values-local

**Files:**
- Modify: `deploy/details/values-local.yaml`

- [ ] **Step 1: Read current file**

```bash
cat deploy/details/values-local.yaml
```

Identify lines 22–43 (`cqrs.endpoints` block) and 56–60 (`events.exposed` block) and dlq url at line 47.

- [ ] **Step 2: Remove `cqrs.endpoints` block under `cqrs:`**

Delete the `endpoints:` subtree (lines 36–43 in the current file):

```yaml
  endpoints:
    book-added:
      port: 12000
      triggers:
        - name: create-detail
          url: self
          payload:
            - passthrough
```

`cqrs:` block keeps `enabled`, `read`, `write`.

- [ ] **Step 3: Remove `events.exposed` block under `events:`**

Delete:

```yaml
  exposed:
    events:
      topic: bookinfo_details_events
      eventBusName: kafka
```

`events:` keeps `kafka.broker` and `consumed`.

- [ ] **Step 4: Update `sensor.dlq.url` port 12004 → 12000**

```yaml
sensor:
  dlq:
    url: "http://dlq-event-received-eventsource-svc.bookinfo.svc.cluster.local:12000/v1/events"
```

Also update the inner `events.consumed.raw-books-details.dlq.url` at line 74 (same change 12004 → 12000).

- [ ] **Step 5: Lint chart against new values**

```bash
make helm-lint
```

Expected: PASS.

- [ ] **Step 6: Diff rendered manifest vs main**

```bash
git stash
helm template details charts/bookinfo-service \
  -f deploy/details/values-local.yaml \
  -f deploy/details/values-generated.yaml > /tmp/render-details-main.yaml
git stash pop
helm template details charts/bookinfo-service \
  -f deploy/details/values-local.yaml \
  -f deploy/details/values-generated.yaml > /tmp/render-details-new.yaml

diff -u /tmp/render-details-main.yaml /tmp/render-details-new.yaml
```

Expected: only diff is the dlq url port `12004 → 12000`, which is a deliberate dlqueue port change (dlqueue endpoint moves to 12000 in Task 8). Nothing else.

- [ ] **Step 7: Commit**

```bash
git add deploy/details/values-local.yaml
git commit -s -m "chore(details): drop cqrs.endpoints and events.exposed from values-local

Both blocks now sourced from values-generated.yaml (specgen) plus chart
defaults. dlq url updated for upcoming dlqueue port change."
```

### Task 6: ratings — strip values-local

**Files:**
- Modify: `deploy/ratings/values-local.yaml`

- [ ] **Step 1: Remove `cqrs.endpoints` block** (lines ~44–51, the `endpoints: rating-submitted: { port: 12002, triggers: ... }` subtree).
- [ ] **Step 2: Remove `events.exposed` block** (the `topic: bookinfo_ratings_events / eventBusName: kafka` subtree).
- [ ] **Step 3: Update `sensor.dlq.url` port 12004 → 12000** (line 55).
- [ ] **Step 4: Lint**

```bash
make helm-lint
```

- [ ] **Step 5: Diff manifest vs main**

```bash
git stash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  -f deploy/ratings/values-generated.yaml > /tmp/render-ratings-main.yaml
git stash pop
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  -f deploy/ratings/values-generated.yaml > /tmp/render-ratings-new.yaml

diff -u /tmp/render-ratings-main.yaml /tmp/render-ratings-new.yaml
```

Expected: only diff is dlq url port `12004 → 12000`, AND the EventSource for rating-submitted moves from port 12002 to port 12000 (its Service, EventSource webhook port, and HTTPRoute backend port). All other manifests unchanged.

- [ ] **Step 6: Commit**

```bash
git add deploy/ratings/values-local.yaml
git commit -s -m "chore(ratings): drop cqrs.endpoints and events.exposed from values-local

Endpoint port shifts 12002 → 12000 (chart default). dlq url updated for
upcoming dlqueue port change."
```

### Task 7: reviews — strip values-local

**Files:**
- Modify: `deploy/reviews/values-local.yaml`

- [ ] **Step 1: Remove `cqrs.endpoints` block** (the entire `endpoints:` subtree with both `review-submitted` (port 12001) and `review-deleted` (port 12003) entries, including the verbose `review-deleted` payload-transform that was a no-op).

- [ ] **Step 2: Remove `events.exposed` block** (`topic: bookinfo_reviews_events / eventBusName: kafka`).

- [ ] **Step 3: Update `sensor.dlq.url` port 12004 → 12000** (line 58 area).

- [ ] **Step 4: Lint**

```bash
make helm-lint
```

- [ ] **Step 5: Diff manifest vs main**

```bash
git stash
helm template reviews charts/bookinfo-service \
  -f deploy/reviews/values-local.yaml \
  -f deploy/reviews/values-generated.yaml > /tmp/render-reviews-main.yaml
git stash pop
helm template reviews charts/bookinfo-service \
  -f deploy/reviews/values-local.yaml \
  -f deploy/reviews/values-generated.yaml > /tmp/render-reviews-new.yaml

diff -u /tmp/render-reviews-main.yaml /tmp/render-reviews-new.yaml
```

Expected diffs:
- review-submitted EventSource/Service/HTTPRoute port 12001 → 12000.
- review-deleted EventSource/Service/HTTPRoute port 12003 → 12000.
- review-deleted Sensor trigger payload simplifies from `[{src: {dependencyName, dataKey: body.review_id}, dest: review_id}]` to `[{src: {dependencyName, dataKey: body}, dest: ""}]` (passthrough). Wire-effect identical because productpage already POSTs `{review_id}` and the reviews handler already accepts that exact shape (see plan preamble).
- Trigger names change `create-review → review-submitted` and `delete-review-write → review-deleted`. Internal Argo Events labels only; no functional impact.
- dlq url port 12004 → 12000.

- [ ] **Step 6: Commit**

```bash
git add deploy/reviews/values-local.yaml
git commit -s -m "chore(reviews): drop cqrs.endpoints and events.exposed from values-local

Endpoints move to chart-default port 12000. The review-deleted trigger
payload becomes plain passthrough — productpage already POSTs the wire
shape the reviews handler expects, so the prior body.review_id transform
was a no-op. dlq url updated for upcoming dlqueue port change."
```

### Task 8: dlqueue — strip values-local

**Files:**
- Modify: `deploy/dlqueue/values-local.yaml`

- [ ] **Step 1: Remove `cqrs.endpoints` block** (the `endpoints: dlq-event-received: { port: 12004, triggers: ... }` subtree, lines ~36–45).

- [ ] **Step 2: dlqueue has no `events.exposed` block — verify nothing to remove.**

```bash
grep -n 'events:' deploy/dlqueue/values-local.yaml
```

Expected: no `events.exposed:` key, only possibly a `cqrs.eventBusName` to drop in this task if present:

```bash
grep -n 'eventBusName' deploy/dlqueue/values-local.yaml
```

If `cqrs.eventBusName: kafka` is present, remove that line.

- [ ] **Step 3: Lint**

```bash
make helm-lint
```

- [ ] **Step 4: Diff manifest vs main**

```bash
git stash
helm template dlqueue charts/bookinfo-service \
  -f deploy/dlqueue/values-local.yaml \
  -f deploy/dlqueue/values-generated.yaml > /tmp/render-dlqueue-main.yaml
git stash pop
helm template dlqueue charts/bookinfo-service \
  -f deploy/dlqueue/values-local.yaml \
  -f deploy/dlqueue/values-generated.yaml > /tmp/render-dlqueue-new.yaml

diff -u /tmp/render-dlqueue-main.yaml /tmp/render-dlqueue-new.yaml
```

Expected: dlq-event-received EventSource/Service/HTTPRoute port `12004 → 12000`. Trigger name changes `ingest-dlq-event → dlq-event-received` (label only).

- [ ] **Step 5: Commit**

```bash
git add deploy/dlqueue/values-local.yaml
git commit -s -m "chore(dlqueue): drop cqrs.endpoints from values-local

Endpoint moves to chart-default port 12000."
```

### Task 9: ingestion — strip values-local

**Files:**
- Modify: `deploy/ingestion/values-local.yaml`

- [ ] **Step 1: Remove `events.exposed` block** (the `raw-books-details: { topic: raw_books_details, eventBusName: kafka }` subtree).

- [ ] **Step 2: Verify no `cqrs.endpoints` (ingestion is producer-only).**

```bash
grep -n 'endpoints:\|eventBusName:' deploy/ingestion/values-local.yaml
```

Remove any `cqrs.eventBusName` if present.

- [ ] **Step 3: Lint**

```bash
make helm-lint
```

- [ ] **Step 4: Diff manifest vs main**

```bash
git stash
helm template ingestion charts/bookinfo-service \
  -f deploy/ingestion/values-local.yaml \
  -f deploy/ingestion/values-generated.yaml > /tmp/render-ingestion-main.yaml
git stash pop
helm template ingestion charts/bookinfo-service \
  -f deploy/ingestion/values-local.yaml \
  -f deploy/ingestion/values-generated.yaml > /tmp/render-ingestion-new.yaml

diff -u /tmp/render-ingestion-main.yaml /tmp/render-ingestion-new.yaml
```

Expected: no diff. The Kafka EventSource still reads `topic` (now from values-generated) and `eventBusName` (now from chart default `kafka`).

- [ ] **Step 5: Commit**

```bash
git add deploy/ingestion/values-local.yaml
git commit -s -m "chore(ingestion): drop events.exposed from values-local

topic now sourced from values-generated.yaml; eventBusName from chart
default."
```

### Task 10: notification — update dlq urls

**Files:**
- Modify: `deploy/notification/values-local.yaml`

- [ ] **Step 1: Update 4× dlq urls 12004 → 12000**

The file has four occurrences (lines 50, 77, 104, 131). Update all of them.

```bash
sed -i.bak 's|:12004/v1/events|:12000/v1/events|g' deploy/notification/values-local.yaml
rm deploy/notification/values-local.yaml.bak
```

- [ ] **Step 2: Verify no `cqrs.endpoints` or `events.exposed` to drop**

```bash
grep -n 'endpoints:\|exposed:\|eventBusName:' deploy/notification/values-local.yaml
```

Expected: no `cqrs.endpoints` (notification has no CQRS write endpoint), no `events.exposed` (notification doesn't publish). If `cqrs.eventBusName` is set, remove it.

- [ ] **Step 3: Lint**

```bash
make helm-lint
```

- [ ] **Step 4: Diff manifest vs main**

```bash
git stash
helm template notification charts/bookinfo-service \
  -f deploy/notification/values-local.yaml \
  -f deploy/notification/values-generated.yaml > /tmp/render-notification-main.yaml
git stash pop
helm template notification charts/bookinfo-service \
  -f deploy/notification/values-local.yaml \
  -f deploy/notification/values-generated.yaml > /tmp/render-notification-new.yaml

diff -u /tmp/render-notification-main.yaml /tmp/render-notification-new.yaml
```

Expected: only diff is the four dlq url ports `12004 → 12000`.

- [ ] **Step 5: Commit**

```bash
git add deploy/notification/values-local.yaml
git commit -s -m "chore(notification): point dlq urls at new dlqueue port 12000"
```

### Task 11: productpage — verify unchanged

**Files:**
- Read-only: `deploy/productpage/values-local.yaml`

- [ ] **Step 1: Confirm no soon-stale keys**

```bash
grep -n 'cqrs.endpoints\|events.exposed\|eventBusName' deploy/productpage/values-local.yaml
```

Expected: no matches. Productpage has no CQRS writes, no exposed events.

- [ ] **Step 2: Diff manifest vs main**

```bash
git stash
helm template productpage charts/bookinfo-service \
  -f deploy/productpage/values-local.yaml > /tmp/render-productpage-main.yaml
git stash pop
helm template productpage charts/bookinfo-service \
  -f deploy/productpage/values-local.yaml > /tmp/render-productpage-new.yaml

diff -u /tmp/render-productpage-main.yaml /tmp/render-productpage-new.yaml
```

Expected: no diff.

No commit (no changes).

---

## Phase 4 — Chart breaking cleanup

### Task 12: Remove fallback expressions from chart templates

**Files:**
- Modify: `charts/bookinfo-service/templates/eventsource.yaml`
- Modify: `charts/bookinfo-service/templates/eventsource-service.yaml`
- Modify: `charts/bookinfo-service/templates/httproute.yaml`
- Modify: `charts/bookinfo-service/templates/sensor.yaml`
- Modify: `charts/bookinfo-service/templates/kafka-eventsource.yaml`
- Modify: `charts/bookinfo-service/templates/consumer-sensor.yaml`

- [ ] **Step 1: `eventsource.yaml`**

Replace:

```yaml
  eventBusName: {{ default $.Values.events.busName $.Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ $.Values.events.busName }}
```

Replace:

```yaml
      port: {{ default $.Values.cqrs.eventSource.port $endpoint.port | quote }}
```

with:

```yaml
      port: {{ $.Values.cqrs.eventSource.port | quote }}
```

- [ ] **Step 2: `eventsource-service.yaml`**

Replace:

```yaml
    - port: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
      targetPort: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
```

with:

```yaml
    - port: {{ $.Values.cqrs.eventSource.port }}
      targetPort: {{ $.Values.cqrs.eventSource.port }}
```

- [ ] **Step 3: `httproute.yaml`**

Replace:

```yaml
          port: {{ default $.Values.cqrs.eventSource.port $endpoint.port }}
```

with:

```yaml
          port: {{ $.Values.cqrs.eventSource.port }}
```

- [ ] **Step 4: `sensor.yaml`**

Replace:

```yaml
  eventBusName: {{ default .Values.events.busName .Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ .Values.events.busName }}
```

Replace the DLQ url block:

```yaml
      {{- $esPort := default $.Values.cqrs.eventSource.port $endpoint.port }}
      {{- $esURL := printf "http://%s-eventsource-svc.%s.svc.cluster.local:%v%s" $eventName $.Release.Namespace $esPort $endpoint.endpoint }}
```

with:

```yaml
      {{- $esURL := printf "http://%s-eventsource-svc.%s.svc.cluster.local:%v%s" $eventName $.Release.Namespace $.Values.cqrs.eventSource.port $endpoint.endpoint }}
```

- [ ] **Step 5: `kafka-eventsource.yaml`**

Replace:

```yaml
  eventBusName: {{ default $.Values.events.busName $event.eventBusName }}
```

with:

```yaml
  eventBusName: {{ $.Values.events.busName }}
```

- [ ] **Step 6: `consumer-sensor.yaml`**

Replace:

```yaml
  eventBusName: {{ default .Values.events.busName .Values.cqrs.eventBusName }}
```

with:

```yaml
  eventBusName: {{ .Values.events.busName }}
```

- [ ] **Step 7: Lint chart**

```bash
make helm-lint
```

Expected: PASS.

- [ ] **Step 8: Diff manifest vs main (full sweep)**

```bash
for svc in details ratings reviews dlqueue ingestion notification productpage; do
  helm template "$svc" charts/bookinfo-service \
    -f "deploy/$svc/values-local.yaml" \
    -f "deploy/$svc/values-generated.yaml" 2>/dev/null > "/tmp/render-$svc-clean.yaml" || \
    helm template "$svc" charts/bookinfo-service \
      -f "deploy/$svc/values-local.yaml" 2>/dev/null > "/tmp/render-$svc-clean.yaml"
  diff -u "/tmp/render-$svc-new.yaml" "/tmp/render-$svc-clean.yaml" || echo "DRIFT in $svc"
done
```

Expected: no drift between phase-3 and phase-4 outputs (values-local already strips the old keys, so removing fallback has no effect on the rendered manifests).

- [ ] **Step 9: Commit**

```bash
git add charts/bookinfo-service/templates/
git commit -s -m "feat(chart)!: drop legacy cqrs.eventBusName / per-endpoint port keys

Templates now read only events.busName and cqrs.eventSource.port. The
default <new> <old> fallbacks added in phase 2 are removed. All
values-local files have been migrated.

BREAKING CHANGE: external chart consumers must rename cqrs.eventBusName
to events.busName, drop per-endpoint cqrs.endpoints[].port (chart now
hardcodes 12000 via cqrs.eventSource.port), and drop per-event
events.exposed[].eventBusName."
```

### Task 13: Drop `cqrs.eventBusName` from chart values.yaml

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`

- [ ] **Step 1: Remove the line `eventBusName: kafka` under `cqrs:`**

After this change, the `cqrs:` block in chart values.yaml looks like:

```yaml
cqrs:
  enabled: false
  read:
    replicas: 1
    resources: {}
  write:
    replicas: 1
    resources: {}
  eventSource:
    port: 12000
  endpoints: {}
```

- [ ] **Step 2: Lint**

```bash
make helm-lint
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -s -m "feat(chart)!: drop cqrs.eventBusName default

Replaced by events.busName earlier in this PR."
```

### Task 14: Update chart-testing fixtures

**Files:**
- Modify: `charts/bookinfo-service/ci/values-details-consumer.yaml`
- Modify: `charts/bookinfo-service/ci/values-details-postgres.yaml`
- Modify: `charts/bookinfo-service/ci/values-dlqueue-no-dlq.yaml`
- Modify: `charts/bookinfo-service/ci/values-ingestion-kafka.yaml`
- Modify: `charts/bookinfo-service/ci/values-ratings-cqrs.yaml`
- Modify: `charts/bookinfo-service/ci/values-productpage-simple.yaml`

- [ ] **Step 1: Read each ci fixture and identify keys to strip**

```bash
for f in charts/bookinfo-service/ci/*.yaml; do
  echo "=== $f ==="
  grep -nE 'cqrs\.endpoints|cqrs\.eventBusName|events\.exposed|port:.*1200[0-4]|eventBusName' "$f" || echo "(clean)"
done
```

- [ ] **Step 2: For each fixture, remove `cqrs.endpoints.<name>.port`, `cqrs.endpoints.<name>.triggers`, `cqrs.eventBusName`, `events.exposed.<key>.eventBusName`, `events.exposed.<key>.topic` (if generated keys are not separately mocked) — keep only the keys the fixture genuinely tests.**

(Open each file individually; preserve the test's intent. If a fixture exists specifically to test a chart feature that no longer takes those keys, the fixture's expected behavior shifts to chart defaults.)

- [ ] **Step 3: Run chart-testing locally if installed**

```bash
ct lint --chart-dirs charts --target-branch main 2>/dev/null || make helm-lint
```

Expected: PASS for every fixture.

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/ci/
git commit -s -m "test(chart): update ci fixtures for new key shape"
```

### Task 15: Bump chart version

**Files:**
- Modify: `charts/bookinfo-service/Chart.yaml`

- [ ] **Step 1: Increment chart version 0.4.0 → 0.5.0**

```yaml
version: 0.5.0
```

- [ ] **Step 2: Lint**

```bash
make helm-lint
```

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/Chart.yaml
git commit -s -m "chore(chart): bump version 0.4.0 → 0.5.0

Breaking change: cqrs.eventBusName and per-endpoint port keys removed."
```

---

## Phase 5 — Documentation

### Task 16: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Find the "Helm Events Configuration" section**

```bash
grep -n 'Helm Events Configuration\|cqrs.endpoints\|events.exposed' CLAUDE.md
```

- [ ] **Step 2: Update the section text**

The section starting at line 68 currently describes:
```
Services can declare Kafka event pipelines independent of CQRS:

- **`events.exposed`**: creates a Kafka-type EventSource (used by producer services like ingestion)
- **`events.consumed`**: creates a Consumer Sensor with triggers and DLQ (used by consuming services like details)
- **`events.kafka.broker`**: injects `KAFKA_BROKERS` env var into ConfigMap
```

After this change, expand the section to clarify what is auto-generated vs hand-authored:

```markdown
## Helm Events Configuration

Services declare Kafka event pipelines and CQRS write endpoints, with most
boilerplate auto-generated by tools/specgen into `deploy/<svc>/values-generated.yaml`:

**Auto-generated by specgen (in `values-generated.yaml`):**
- `cqrs.endpoints.<eventName>.{method, endpoint}` — derived from `services/<svc>/internal/adapter/inbound/http/endpoints.go` for endpoints with an `EventName`.
- `events.exposed.<exposureKey>.{topic, contentType, eventTypes}` — derived from `services/<svc>/internal/adapter/outbound/kafka/exposed.go`.

**Chart defaults (no values-file declaration needed):**
- `cqrs.eventSource.port: 12000` — port for every CQRS webhook EventSource.
- Default sensor trigger: `{name: <eventName>, url: self, payload: [passthrough]}` — synthesized when `cqrs.endpoints.<name>.triggers` is absent.
- `events.busName: kafka` — EventBus name for both CQRS sensor and kafka EventSource.

**Hand-authored in `values-local.yaml`:**
- `cqrs.{enabled, read, write}` — service-specific deployment shape.
- `events.kafka.broker` — cluster bootstrap URL.
- `events.consumed.<event>.triggers` — kept hand-authored because consumer triggers may need payload transforms (notification, details).
- `sensor.dlq.url` — points at the dlqueue's EventSource webhook URL.

These coexist with productpage which uses neither cqrs.endpoints nor events.exposed.
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -s -m "docs: clarify auto-generated vs hand-authored helm values"
```

### Task 17: Verify kafka exposed.go header comments are still accurate

**Files:**
- Read-only: `services/details/internal/adapter/outbound/kafka/exposed.go`
- Read-only: `services/ratings/internal/adapter/outbound/kafka/exposed.go`
- Read-only: `services/reviews/internal/adapter/outbound/kafka/exposed.go`
- Read-only: `services/ingestion/internal/adapter/outbound/kafka/exposed.go`

- [ ] **Step 1: Read each header comment**

```bash
for f in services/{details,ratings,reviews,ingestion}/internal/adapter/outbound/kafka/exposed.go; do
  echo "=== $f ==="
  head -15 "$f"
done
```

Expected: each header comment says "tools/specgen reads the same slice to derive ... values-generated.yaml". This is still accurate after the change. No edits needed.

- [ ] **Step 2: If any header text suggests `topic` is hand-authored in values-local, update it to say "values-generated"**

(Currently none do; this step is a guard. If a comment says `events.exposed.<key>.topic` in `deploy/<svc>/values-local.yaml`, change it to `values-generated.yaml`.)

- [ ] **Step 3: If any edits were needed, commit**

```bash
git diff -- 'services/*/internal/adapter/outbound/kafka/exposed.go'
# If non-empty:
git add services/*/internal/adapter/outbound/kafka/exposed.go
git commit -s -m "docs(kafka): align header comments with new generated topic field"
```

If no edits, skip the commit.

---

## Phase 6 — Validation

### Task 18: Full local k3d validation

**Files:**
- Read-only: all of the above.

- [ ] **Step 1: Tear down and rebuild**

```bash
make stop-k8s && make run-k8s
```

Expected: cluster comes up cleanly. Total time ~3-5 min for the first run after kafka/postgres images are present.

- [ ] **Step 2: Confirm pod status**

```bash
make k8s-status
kubectl --context=k3d-bookinfo-local get pods -n bookinfo
```

Expected: all backend services show `Running 1/1`. EventSource pods (one per endpoint) all `Running`. Sensor pods all `Running`. ingestion service `Running`.

Confirm pod count: 9 EventSource pods (one per CR — same as before, see brainstorming).

- [ ] **Step 3: Smoke flow — productpage GET**

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/productpage?id=0
```

Expected: `200`.

- [ ] **Step 4: Smoke flow — submit review**

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"0","reviewer":"smoke-test","text":"sharpening migration check"}' \
  http://localhost:8080/v1/reviews
```

Expected: HTTP 202 (or 200 from the gateway). Within ~5s, GETing productpage shows the new review (kafka path: gateway → review-submitted EventSource → sensor → reviews-write).

- [ ] **Step 5: Smoke flow — delete review**

```bash
REVIEW_ID=$(curl -s 'http://localhost:8080/v1/reviews/0' | jq -r '.reviews[0].id')
curl -s -X POST -H 'Content-Type: application/json' \
  -d "{\"review_id\":\"$REVIEW_ID\"}" \
  http://localhost:8080/v1/reviews/delete
```

Expected: HTTP 202. Within ~5s, GETing productpage no longer shows that review.

- [ ] **Step 6: Smoke flow — submit rating**

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"0","stars":5}' \
  http://localhost:8080/v1/ratings
```

Expected: HTTP 202. Productpage reflects updated rating.

- [ ] **Step 7: Smoke flow — ingestion → details consumer**

ingestion polls every `POLL_INTERVAL`. Wait for one cycle, then:

```bash
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l app=ingestion --tail=50 | grep -i 'published\|book-added'
kubectl --context=k3d-bookinfo-local logs -n bookinfo -l app=details-write --tail=50 | grep -i 'create\|received'
```

Expected: ingestion publishes book-added events; details-write receives them via the consumer-sensor.

- [ ] **Step 8: Smoke flow — DLQ retry exhaustion**

Force a sensor retry exhaustion to verify dlqueue still works at port 12000:

```bash
# Scale reviews-write to 0
kubectl --context=k3d-bookinfo-local scale deployment reviews-write --replicas=0 -n bookinfo

# Submit a review (will fail at write, exhaust retries, hit DLQ)
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"product_id":"0","reviewer":"dlq-test","text":"force a dlq event"}' \
  http://localhost:8080/v1/reviews

# Wait ~30s for retries to exhaust
sleep 30

# Check dlqueue
curl -s http://localhost:8080/v1/events | jq '.events[] | select(.eventsource_url | contains(":12000"))'
```

Expected: a dlq event present whose `eventsource_url` field contains `:12000` (the new dlqueue port). Replay it:

```bash
DLQ_ID=$(curl -s http://localhost:8080/v1/events | jq -r '.events[0].id')

# Restore reviews-write
kubectl --context=k3d-bookinfo-local scale deployment reviews-write --replicas=1 -n bookinfo
# Wait for ready
kubectl --context=k3d-bookinfo-local rollout status deployment reviews-write -n bookinfo

curl -s -X POST http://localhost:8080/v1/events/$DLQ_ID/replay
```

Expected: HTTP 200. The replayed event lands at reviews-write; review appears on productpage.

- [ ] **Step 9: Tempo trace check**

Open Grafana at `http://localhost:3000` (default password `admin/admin`). Navigate to **Explore → Tempo**.

Search by service name and confirm a connected trace for each of the following flows. Each trace should span the producer → gateway → eventsource → sensor → write-pod chain.

```
service.name = productpage   → at least one POST /partials/reviews/* span linked to /v1/reviews/*
service.name = reviews-write → spans for both review-submitted and review-deleted
service.name = ratings-write → spans for rating-submitted
service.name = details-write → spans for both book-added (from raw-books-details consumer) and any direct POST /v1/details
service.name = notification  → spans for all 4 consumed events
service.name = ingestion     → kafka producer span on the raw_books_details topic
service.name = dlqueue-write → spans from the DLQ retry exhaustion test in step 8
```

Expected: every flow shows a connected, multi-service trace with no orphaned spans.

- [ ] **Step 10: Document validation outcomes inline (no commit)**

Note any unexpected behavior in this turn's chat for the user. If everything passes, proceed to Task 19. If any flow fails, debug, fix, and repeat from Step 1.

### Task 19: Open PR and gate on green CI

**Files:** N/A (git + gh).

- [ ] **Step 1: Push branch**

```bash
git push -u origin feat/values-generated-sharpening
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat: sharpen values-generated.yaml (drop cqrs.endpoints + events.exposed from values-local)" \
  --body "$(cat <<'EOF'
## Summary

- specgen now emits `topic` under `events.exposed.<key>` in `values-generated.yaml`.
- Chart hardcodes `cqrs.eventSource.port: 12000` and synthesizes a default passthrough trigger when `cqrs.endpoints.<name>.triggers` is absent.
- `events.busName: kafka` replaces both `cqrs.eventBusName` and per-event `events.exposed.<key>.eventBusName`.
- All `deploy/<svc>/values-local.yaml` files drop `cqrs.endpoints`, `events.exposed`, and `cqrs.eventBusName`.
- dlqueue endpoint moves to port 12000 (chart default); all DLQ urls updated.
- Chart bumped 0.4.0 → 0.5.0 (BREAKING for external consumers).

Spec: `docs/superpowers/specs/2026-04-27-values-generated-sharpening-design.md`
Plan: `docs/superpowers/plans/2026-04-27-values-generated-sharpening.md`

## Test plan

- [x] specgen unit tests pass (single descriptor, multi descriptor shared topic, divergent topic failure, no-port/no-triggers invariant)
- [x] `make helm-lint` passes for all per-service values
- [x] `helm template` byte-identical to main during phase 2 (additive)
- [x] `helm template` shows expected migrations during phase 3 (port shifts, trigger label renames, dlq url updates)
- [x] Local k3d full bring-up clean — all pods Running
- [x] Smoke: GET productpage, submit/delete review, submit rating, ingestion → details consumer
- [x] DLQ retry exhaustion → entry at port 12000 → replay restores event
- [x] Grafana Tempo: connected traces for every flow per service
EOF
)"
```

- [ ] **Step 3: Wait for CI checks to complete**

```bash
gh pr checks --watch
```

Expected: every required check turns green: build, unit tests (race), golangci-lint, helm-lint, helm-test (chart-testing), e2e, spec-validation. If any check fails, fetch the failing log, fix the underlying issue, push the fix, and re-run `gh pr checks --watch`.

- [ ] **Step 4: List final status**

```bash
gh pr checks
gh pr view --json statusCheckRollup --jq '.statusCheckRollup[] | {name, status, conclusion}'
```

Expected: every entry shows `status: COMPLETED, conclusion: SUCCESS`. No `FAILURE`, no `PENDING`.

- [ ] **Step 5: Report PR url to user**

The PR is ready for review. Report the URL to the user. Do not merge — merge is the user's call.

---

## Self-review notes (verified during writing)

Spec coverage:
- ✓ Goal 1 (no cqrs.endpoints/events.exposed/cqrs.eventBusName in values-local) — Tasks 5–10.
- ✓ Goal 2 (values-generated as source of truth for endpoint+event metadata) — Task 1 + chart defaults in Tasks 2–4.
- ✓ Goal 3 (chart defaults for port/eventBusName/trigger) — Tasks 2–4 + 12–13.
- ✓ Goal 4 (specgen extension narrow) — Task 1.
- ✓ Migration order (docs audit, specgen first, chart additive, refactor — collapsed, values-local cleanup, chart breaking) — Tasks 1, 2–4, 5–11, 12–15, 16–17.
- ✓ Validation (k3d, smoke, Tempo, gh pr checks) — Tasks 18, 19.
- ✓ Risks (dlq port drift) — addressed in Tasks 5–10 (atomic per-service updates).

Type / signature consistency:
- `buildExposedNode` signature `(*yaml.Node, error)` consistent across Step 4 of Task 1 and the call site in `Build`.
- chart key `cqrs.eventSource.port` consistent across all six template tasks.
- `events.busName` consistent across all six template tasks.
- Trigger synthesis `{name: <eventName>, url: self, payload: [passthrough]}` consistent in chart values default + sensor.yaml synthesis.

No placeholders. No "TODO" or "TBD" in any step.
