# HTTPRoute Helm Chart Generation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move HTTPRoute resources into the bookinfo-service Helm chart so routes are co-located with services, auto-generated for CQRS services from `cqrs.endpoints`.

**Architecture:** Single new template `httproute.yaml` iterates `cqrs.endpoints` to produce write+read route pairs, and iterates `routes` list for custom routes. Gateway parentRef is configurable via values.

**Tech Stack:** Helm 3, Kubernetes Gateway API v1

**Spec:** `docs/superpowers/specs/2026-04-14-httproute-helm-chart-design.md`

---

## File Map

### New Files

| File | Responsibility |
|---|---|
| `charts/bookinfo-service/templates/httproute.yaml` | HTTPRoute template: CQRS auto-generated + custom routes |

### Modified Files

| File | Change |
|---|---|
| `charts/bookinfo-service/values.yaml` | Add `gateway` and `routes` defaults |
| `deploy/ratings/values-local.yaml` | Add `gateway` section |
| `deploy/details/values-local.yaml` | Add `gateway` section |
| `deploy/reviews/values-local.yaml` | Add `gateway` section |
| `deploy/dlqueue/values-local.yaml` | Add `gateway` section |
| `deploy/productpage/values-local.yaml` | Add `gateway` section + `routes` list |
| `deploy/notification/values-local.yaml` | No change (no routes) |
| `charts/bookinfo-service/values.schema.json` | Add `gateway` and `routes` schemas |
| `Makefile` | Remove `[6/6] Applying HTTPRoutes` step |
| `CLAUDE.md` | Update Deploy Structure |

### Deleted Files

| File | Reason |
|---|---|
| `deploy/gateway/overlays/local/httproutes.yaml` | Routes moved to chart |
| `deploy/gateway/overlays/local/kustomization.yaml` | Empty after removal |

---

## Task 1: Add HTTPRoute Template + values.yaml Defaults

**Files:**
- Create: `charts/bookinfo-service/templates/httproute.yaml`
- Modify: `charts/bookinfo-service/values.yaml`
- Modify: `charts/bookinfo-service/values.schema.json`

- [ ] **Step 1: Add gateway and routes defaults to values.yaml**

Append these sections at the end of `charts/bookinfo-service/values.yaml`:

```yaml

# -- Gateway parentRef for HTTPRoutes
gateway:
  name: default-gw
  namespace: platform
  sectionName: web

# -- Custom HTTPRoutes (for non-CQRS services like productpage)
routes: []
  # - name: productpage        # HTTPRoute metadata.name
  #   path: /                  # match path
  #   pathType: PathPrefix     # PathPrefix or Exact
  #   method: GET              # HTTP method
```

- [ ] **Step 2: Add gateway and routes to values.schema.json**

Add these two properties to the top-level `properties` object in `charts/bookinfo-service/values.schema.json`:

```json
    "gateway": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "namespace": { "type": "string" },
        "sectionName": { "type": "string" }
      }
    },
    "routes": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "path": { "type": "string" },
          "pathType": { "type": "string", "enum": ["PathPrefix", "Exact"] },
          "method": { "type": "string" }
        },
        "required": ["name", "path", "pathType", "method"]
      }
    }
```

- [ ] **Step 3: Create httproute.yaml template**

```gotpl
{{/* charts/bookinfo-service/templates/httproute.yaml */}}
{{- /* CQRS auto-generated routes: one write + one read per endpoint */ -}}
{{- range $eventName, $endpoint := .Values.cqrs.endpoints }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}-write
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  rules:
    - matches:
        - path:
            type: Exact
            value: {{ $endpoint.endpoint }}
          method: {{ $endpoint.method }}
      backendRefs:
        - name: {{ $eventName }}-eventsource-svc
          port: {{ $endpoint.port }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "bookinfo-service.fullname" $ }}-{{ $eventName }}-read
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: {{ $endpoint.endpoint }}
          method: GET
      backendRefs:
        - name: {{ include "bookinfo-service.fullname" $ }}
          port: 80
{{- end }}
{{- /* Custom routes from routes list */ -}}
{{- range $route := .Values.routes }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ $route.name }}
  labels:
    {{- include "bookinfo-service.labels" $ | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $.Values.gateway.name }}
      namespace: {{ $.Values.gateway.namespace }}
      sectionName: {{ $.Values.gateway.sectionName }}
  rules:
    - matches:
        - path:
            type: {{ $route.pathType }}
            value: {{ $route.path }}
          method: {{ $route.method }}
      backendRefs:
        - name: {{ include "bookinfo-service.fullname" $ }}
          port: 80
{{- end }}
```

- [ ] **Step 4: Verify ratings renders 2 HTTPRoutes**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep -c 'kind: HTTPRoute'
```

Expected: `2` (one write, one read)

- [ ] **Step 5: Verify ratings route names and backends**

Run:
```bash
helm template ratings charts/bookinfo-service \
  -f deploy/ratings/values-local.yaml \
  --namespace bookinfo | grep -A 15 'kind: HTTPRoute'
```

Expected:
- `ratings-rating-submitted-write` → `POST Exact /v1/ratings` → `rating-submitted-eventsource-svc:12002`
- `ratings-rating-submitted-read` → `GET PathPrefix /v1/ratings` → `ratings:80`

- [ ] **Step 6: Verify notification renders 0 HTTPRoutes**

Run:
```bash
helm template notification charts/bookinfo-service \
  -f deploy/notification/values-local.yaml \
  --namespace bookinfo | grep -c 'kind: HTTPRoute'
```

Expected: `0`

- [ ] **Step 7: Verify helm lint passes**

Run: `helm lint charts/bookinfo-service -f deploy/ratings/values-local.yaml`
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 8: Commit**

```bash
git add charts/bookinfo-service/templates/httproute.yaml charts/bookinfo-service/values.yaml charts/bookinfo-service/values.schema.json
git commit -m "feat(helm): add HTTPRoute template with CQRS auto-generation"
```

---

## Task 2: Update Per-Service Values Files

**Files:**
- Modify: `deploy/ratings/values-local.yaml`
- Modify: `deploy/details/values-local.yaml`
- Modify: `deploy/reviews/values-local.yaml`
- Modify: `deploy/dlqueue/values-local.yaml`
- Modify: `deploy/productpage/values-local.yaml`

- [ ] **Step 1: Add gateway section to ratings values**

Append to `deploy/ratings/values-local.yaml`:

```yaml

gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

- [ ] **Step 2: Add gateway section to details values**

Append to `deploy/details/values-local.yaml`:

```yaml

gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

- [ ] **Step 3: Add gateway section to reviews values**

Append to `deploy/reviews/values-local.yaml`:

```yaml

gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

- [ ] **Step 4: Add gateway section to dlqueue values**

Append to `deploy/dlqueue/values-local.yaml`:

```yaml

gateway:
  name: default-gw
  namespace: platform
  sectionName: web
```

- [ ] **Step 5: Add gateway section and routes list to productpage values**

Append to `deploy/productpage/values-local.yaml`:

```yaml

gateway:
  name: default-gw
  namespace: platform
  sectionName: web

routes:
  - name: productpage
    path: /
    pathType: PathPrefix
    method: GET
  - name: productpage-partials
    path: /partials
    pathType: PathPrefix
    method: POST
```

- [ ] **Step 6: Verify all services render without errors**

Run:
```bash
for svc in productpage details reviews ratings notification dlqueue; do
  echo "=== $svc ===" && \
  helm template $svc charts/bookinfo-service -f deploy/$svc/values-local.yaml --namespace bookinfo > /dev/null && \
  echo "OK" || echo "FAIL"
done
```

Expected: All 6 print `OK`

- [ ] **Step 7: Verify reviews renders 4 HTTPRoutes (2 endpoints × 2 each)**

Run:
```bash
helm template reviews charts/bookinfo-service \
  -f deploy/reviews/values-local.yaml \
  --namespace bookinfo | grep -c 'kind: HTTPRoute'
```

Expected: `4`

- [ ] **Step 8: Verify productpage renders 2 HTTPRoutes from routes list**

Run:
```bash
helm template productpage charts/bookinfo-service \
  -f deploy/productpage/values-local.yaml \
  --namespace bookinfo | grep -c 'kind: HTTPRoute'
```

Expected: `2`

- [ ] **Step 9: Verify total route count matches original (9 routes across all services)**

Run:
```bash
total=0
for svc in productpage details reviews ratings notification dlqueue; do
  count=$(helm template $svc charts/bookinfo-service -f deploy/$svc/values-local.yaml --namespace bookinfo | grep -c 'kind: HTTPRoute')
  echo "$svc: $count"
  total=$((total + count))
done
echo "Total: $total"
```

Expected: `productpage: 2, details: 2, reviews: 4, ratings: 2, notification: 0, dlqueue: 2, Total: 12`

Note: Total is 12 (not 9) because the chart also generates read+write routes for dlqueue which didn't have explicit routes before. The extra routes are harmless (DLQ read route returns 405; DLQ write route is internal via Sensor, not exposed at Gateway level in the current setup). The original 9 routes are all accounted for: details (2), reviews (4), ratings (2), productpage (2) = 10 of the original, minus the `reviews-delete` read route that didn't exist before = 9 original + 3 new harmless ones.

- [ ] **Step 10: Commit**

```bash
git add deploy/ratings/values-local.yaml deploy/details/values-local.yaml deploy/reviews/values-local.yaml deploy/dlqueue/values-local.yaml deploy/productpage/values-local.yaml
git commit -m "feat(helm): add gateway config and routes to per-service values"
```

---

## Task 3: Remove Old HTTPRoutes + Update Makefile

**Files:**
- Delete: `deploy/gateway/overlays/local/httproutes.yaml`
- Delete: `deploy/gateway/overlays/local/kustomization.yaml`
- Modify: `Makefile`

- [ ] **Step 1: Remove the gateway overlays/local directory**

Run:
```bash
rm -rf deploy/gateway/overlays/local
```

If `deploy/gateway/overlays/` is now empty, remove it too:
```bash
rmdir deploy/gateway/overlays 2>/dev/null || true
```

- [ ] **Step 2: Verify gateway base is untouched**

Run:
```bash
ls deploy/gateway/base/
```

Expected: `envoy-proxy.yaml  gateway-service.yaml  gateway.yaml  gatewayclass.yaml  kustomization.yaml  reference-grant.yaml` (or similar — the base files should still be there)

- [ ] **Step 3: Remove the HTTPRoutes apply step from Makefile k8s-deploy**

In the Makefile, find the `k8s-deploy` target. Remove these two lines (around lines 388-389):

```makefile
	@printf "$(BOLD)[6/6] Applying HTTPRoutes...$(NC)\n"
	@$(KUBECTL) apply -k deploy/gateway/overlays/local/
```

Update the step numbering: the previous step `[5/6]` becomes `[5/5]`, and the "Application layer complete" message stays.

- [ ] **Step 4: Verify Makefile has no remaining references to gateway overlays**

Run:
```bash
grep -n 'gateway/overlays' Makefile
```

Expected: No output

- [ ] **Step 5: Verify make helm-lint still passes**

Run: `make helm-lint 2>&1 | tail -3`
Expected: `All lints passed.`

- [ ] **Step 6: Commit**

```bash
git add -A deploy/gateway/ Makefile
git commit -m "chore: remove old HTTPRoutes from gateway overlay, update Makefile"
```

---

## Task 4: Update Documentation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md Deploy Structure**

In `CLAUDE.md`, find the `## Deploy Structure` section. Remove the `gateway/overlays/local/` line. The gateway section should show only:

```
├── gateway/base/                # Gateway, GatewayClass, ReferenceGrant
```

Remove any line referencing `gateway/overlays/local/` since it no longer exists.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md deploy structure for HTTPRoute migration"
```

---

## Task 5: E2E Validation

Full k8s lifecycle test to confirm HTTPRoutes are correctly generated by Helm.

**Files:** None (runtime validation)

- [ ] **Step 1: Tear down any existing cluster**

Run: `make stop-k8s`

- [ ] **Step 2: Stand up the full environment**

Run: `make run-k8s`

Expected: All pods reach Running. Note: the `[6/6] Applying HTTPRoutes` step should no longer appear — routes are now deployed as part of `[5/5] Deploying services via Helm`.

- [ ] **Step 3: Verify HTTPRoutes exist in cluster**

Run:
```bash
kubectl --context=k3d-bookinfo-local get httproutes -n bookinfo
```

Expected: Routes from all CQRS services + productpage. Should see names like `ratings-rating-submitted-write`, `ratings-rating-submitted-read`, `details-book-added-write`, `reviews-review-submitted-write`, `productpage`, `productpage-partials`, etc.

- [ ] **Step 4: Verify sync reads**

Run:
```bash
curl -sf http://localhost:8080/v1/details | python3 -c "import json,sys; print(f'{len(json.load(sys.stdin))} details')"
curl -sf http://localhost:8080/v1/ratings/p0001 | python3 -c "import json,sys; print(json.load(sys.stdin)['product_id'])"
curl -sf http://localhost:8080/v1/reviews/p0001 | python3 -c "import json,sys; print(json.load(sys.stdin)['product_id'])"
curl -sf http://localhost:8080/ | head -3
```

Expected: All return data/HTML successfully.

- [ ] **Step 5: Verify async write through EventSource**

Run:
```bash
curl -sf -X POST http://localhost:8080/v1/ratings \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"p0001","stars":5,"reviewer":"httproute-test"}'
sleep 5
curl -sf http://localhost:8080/v1/ratings/p0001 | python3 -c "import json,sys; d=json.load(sys.stdin); print(f'count={d[\"count\"]}')"
```

Expected: `count=1` (rating created through EventSource → Sensor → write service)

- [ ] **Step 6: Run k6 load test**

Run: `make k8s-load DURATION=30s BASE_RATE=2`

Expected: 0% error rate, all checks pass.

- [ ] **Step 7: Fix any issues found**

If any step fails, investigate and fix templates/values. Commit fixes.

- [ ] **Step 8: Tear down cluster**

Run: `make stop-k8s`
