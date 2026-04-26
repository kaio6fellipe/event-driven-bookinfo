# Productpage HTMX Partials — Proper HTTP Status Codes on Error

**Status:** Design approved · Implementation pending
**Date:** 2026-04-26
**Issue:** [#40](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/40)
**Scope:** `services/productpage/internal/handler/handler.go` — `partialRatingSubmit` only

## Problem

The `POST /partials/rating` handler in `productpage` returns HTTP **200** on every error path. The error message is rendered in the HTML body (`Failed to submit: <error>`) but the status code is always 200. This breaks:

- **Load testing.** k6 (`test/k6/bookinfo-traffic.js`) status checks always pass even when the rating service is unreachable, hiding regressions.
- **Monitoring/alerting.** No status-code signal for the alerting pipeline; everything looks healthy from Prometheus' perspective even during sustained failures.
- **HTMX error UX.** Without proper non-2xx codes, `hx-target-error` / `hx-target-4xx` / `hx-target-5xx` patterns can't be used for error-specific UI behavior.
- **Silent partial success.** When the rating succeeds but the review service fails (path D below), the user sees "Rating submitted successfully" with no indication their review was lost.

## Goals

- Return semantically correct HTTP status codes from every error path in `partialRatingSubmit`.
- Preserve the current user experience: error messages still render in the HTML form section after a swap.
- Distinguish "everything failed" from "rating succeeded but review failed" in both wire status and UI.
- Add unit tests covering each new error path.
- Verify end-to-end on local k3d (clean stop+start cycle, scale-down upstream, browser swap behavior).

## Non-goals

- Fixing `partialDeleteReview`'s similar silent-failure pattern (different handler, different UX assumptions; tracked separately if needed).
- Refactoring the broader productpage handler structure (`partialRatingSubmit` is fine architecturally; only its status codes are wrong).
- Changing the k6 test — `test/k6/bookinfo-traffic.js:106-107` already includes a `r.body.includes('submitted successfully')` check landed in a prior PR; the new status codes work with the existing checks.

## Status code mapping

Four error paths in `partialRatingSubmit`. Each line below replaces a `w.WriteHeader(http.StatusOK)` call:

| Source line | Trigger | Current | New | Rationale |
|-------------|---------|---------|-----|-----------|
| 271 | `r.ParseForm()` returns error (malformed body, MaxBytesReader exceeded) | 200 | **422** | Validation: client sent unprocessable form |
| 285 | `strconv.Atoi(starsStr)` returns error | 200 | **422** | Validation: stars must be 1–5 integer |
| 297 | `h.ratingsClient.SubmitRating(...)` returns error | 200 | **502** | Upstream ratings service failed/unreachable |
| 308 | `h.reviewsClient.SubmitReview(...)` returns error (rating already succeeded) | 200 (silent) | **200** with partial-success template | Rating IS in the system; review path degraded |

Path D's `pendingStore.StorePending` failure stays silent — the cache is best-effort and a miss does not change correctness.

The success path (line 318) keeps its 200.

## Template changes

`services/productpage/templates/partials/rating-form.html` gains an optional `ReviewWarning` field on the success branch:

```html
{{if .Success}}
<div class="notification-banner">
    <strong>Rating submitted successfully!</strong> You gave {{.Stars}} star{{if gt .Stars 1}}s{{end}}.
</div>
{{if .ReviewWarning}}
<div class="warning-banner" style="background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 8px; padding: 0.75rem 1rem; margin-top: 0.5rem; color: #fbbf24;">
    {{.ReviewWarning}}
</div>
{{end}}
{{if .ProductID}}
<div hx-get="/partials/reviews/{{.ProductID}}"
     hx-trigger="load"
     hx-target="#reviews-section"
     hx-swap="innerHTML">
</div>
{{end}}
{{else}}
<!-- error branch unchanged -->
<div class="error-banner" style="...">
    Failed to submit: {{.Error}}
</div>
<form hx-post="/partials/rating"
      hx-target="#rating-form-section"
      hx-swap="innerHTML">
    ...
</form>
{{end}}
```

The handler sets `ReviewWarning` only on path D:

```go
"ReviewWarning": "Your rating was submitted; the reviews service is unavailable — please retry the review separately.",
```

## HTMX swap configuration

`services/productpage/templates/layout.html` — add a `<script>` block immediately after the `htmx.org@2.0.4` script tag:

```html
<script>
    htmx.config.responseHandling = [
        {code: '204', swap: false},
        {code: '[23]..', swap: true},
        {code: '422', swap: true},   // form validation errors — render error template
        {code: '502', swap: true},   // upstream-service errors — render error template
        {code: '[45]..', swap: false, error: true},
        {code: '...', swap: false}
    ];
</script>
```

This makes the form's existing `hx-target="#rating-form-section"` swap the response body for status codes 422 and 502 too. Other 4xx/5xx codes still fire the default error event without swapping. No per-form `hx-target-*` attributes are needed.

## Tests

Four new tests in `services/productpage/internal/handler/handler_test.go` covering the four newly-specified error paths:

| Test | Setup | Assertion |
|------|-------|-----------|
| `TestPartialRatingSubmit_ParseFormFails` | POST a body larger than 1MB so `MaxBytesReader` rejects it | `rec.Code == 422` and body contains `Failed to submit:` |
| `TestPartialRatingSubmit_InvalidStars` | POST with `stars=abc` | `rec.Code == 422` and body contains `Invalid stars value` |
| `TestPartialRatingSubmit_RatingsServiceFails` | Mock ratings server returns 500 | `rec.Code == 502` and body contains the form re-render |
| `TestPartialRatingSubmit_ReviewSubmitFails` | Mock ratings returns 200, mock reviews returns 500, `text=...` is provided | `rec.Code == 200`, body contains `Rating submitted successfully` AND `ReviewWarning` text |

Each test follows the existing `setupMockServers` pattern. ~80–100 lines total.

The three existing happy-path tests (`TestPartialRatingSubmit`, `TestPartialRatingSubmitAsync`, `TestPartialRatingSubmitAsyncWithReview`) keep asserting `rec.Code == http.StatusOK` and continue to pass.

## End-to-end verification

The plan's verification step (clean k3d cycle):

1. `make stop-k8s && make run-k8s` — fresh cluster, all 11 deployments `Available`.
2. **Happy path**: `curl -fsS -X POST -F 'product_id=product-x' -F 'reviewer=alice' -F 'stars=5' http://localhost:8080/partials/rating | grep submitted` → exit 0.
3. **Validation error (B)**: `curl -i -X POST -F 'product_id=product-x' -F 'reviewer=alice' -F 'stars=abc' http://localhost:8080/partials/rating | head -1` → first line shows `HTTP/1.1 422`.
4. **Upstream failure (C)**:
   - `kubectl --context=k3d-bookinfo-local scale deploy/ratings-write --replicas=0`
   - Submit a rating; expect `HTTP/1.1 502` and the error template in the body.
   - `kubectl scale deploy/ratings-write --replicas=1` → restore.
5. **Partial success (D)**:
   - `kubectl scale deploy/reviews-write --replicas=0`
   - Submit a rating WITH review text; expect `HTTP/1.1 200`, body contains both the success banner AND the warning banner ("reviews service is unavailable").
   - `GET /v1/ratings/<product>` → confirms the rating IS in the system (`count: 1`).
   - `kubectl scale deploy/reviews-write --replicas=1` → restore.
6. **Browser swap behavior** (manual, single-step): open `http://localhost:8080/products/product-x` in a browser; submit an invalid form; confirm the error template visibly replaces the form section (not just console errors). This step is the only non-automatable part — call out in the plan but flag it as manual.
7. **Pre-existing tests**: `go test ./services/productpage/... -race -count=1` plus `make e2e` — both pass.

A future PR can promote the scripted parts of this verification (steps 2–5) into a `test/smoke/k8s-smoke.sh` runner — tracked under [#62](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/62) (CI k3d validation).

## Risks & open questions

- **k6 backwards compatibility**: the existing `body.includes('submitted successfully')` check passes for 200 responses with the success template. With the new 422/502 paths, that check correctly fails — load tests will now report failures they previously missed. Acceptable: this is the bug being fixed.
- **HTMX 2.x `responseHandling` API stability**: HTMX 2.0.4 is in use; the `responseHandling` config has been stable since 2.0.0. No deprecation risk for the immediate term.
- **`partialDeleteReview`** has the same silent-failure pattern (line 331 — DeleteReview error logged, response is a refresh template at 200). This PR does NOT touch it. Open as a follow-up issue if the alerting team needs it.
- **`ReviewWarning` template field is optional**: the existing happy-path tests don't pass it; they continue to render without the warning. Tested explicitly.

## File summary

```
services/productpage/internal/handler/handler.go         # modify partialRatingSubmit (4 status codes + ReviewWarning on D)
services/productpage/internal/handler/handler_test.go    # +4 tests
services/productpage/templates/partials/rating-form.html # +ReviewWarning rendering branch
services/productpage/templates/layout.html               # +htmx.config.responseHandling block
```

Net: 4 files changed, ~150 lines added, ~12 lines removed.
