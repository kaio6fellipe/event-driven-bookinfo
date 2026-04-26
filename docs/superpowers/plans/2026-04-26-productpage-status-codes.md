# Productpage Status Codes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix issue [#40](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/40) — make `productpage`'s `partialRatingSubmit` return semantically correct HTTP status codes (422 for validation, 502 for upstream failure, 200-with-warning for partial success) instead of always 200.

**Architecture:** Five focused changes — handler error paths get correct status codes, the success template gains an optional `ReviewWarning` field for partial-success rendering, layout.html configures HTMX 2.x to swap on 422/502, four new unit tests cover the new error paths, and verification runs against a fresh k3d cluster.

**Tech Stack:** Go (stdlib `net/http` + `html/template`), HTMX 2.0.4 (`htmx.config.responseHandling`), `httptest` for unit tests, k3d for end-to-end verification.

**Spec reference:** `docs/superpowers/specs/2026-04-26-productpage-status-codes-design.md`

**Repo:** `/Users/kaio.fellipe/Documents/git/others/go-http-server`
**Worktree:** `.worktrees/feat-api-spec-generation-foundation/`
**Branch:** `fix/productpage-status-codes` (already created off post-#59 main; HEAD is the spec doc commit `06c9e5a`)

---

## File Structure

Four files modified, no new files.

```text
services/productpage/internal/handler/handler.go              # 4 lines change in partialRatingSubmit
services/productpage/internal/handler/handler_test.go         # +4 tests (~120 lines)
services/productpage/templates/partials/rating-form.html      # +ReviewWarning rendering branch (~3 lines)
services/productpage/templates/layout.html                    # +htmx.config.responseHandling block (~10 lines)
```

Net: ~140 lines added, ~12 lines removed.

---

## Task 1: Configure HTMX response handling in layout

**Files:**

- Modify: `services/productpage/templates/layout.html` (after line 7)

This task lands first because the template + handler tests don't depend on it, but the end-to-end k3d verification does. Putting it first means later tasks can be verified in the browser as they go.

- [ ] **Step 1: Read the current head section**

```bash
sed -n '1,10p' services/productpage/templates/layout.html
```

Expected first 10 lines (verified):

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Bookinfo — Product Page</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <style>
        :root {
            --bg: #0f1117;
```

- [ ] **Step 2: Insert the HTMX config block after the htmx script tag**

In `services/productpage/templates/layout.html`, after the line `<script src="https://unpkg.com/htmx.org@2.0.4"></script>` (line 7), insert this block:

```html
    <script>
        // Treat 422 (validation errors) and 502 (upstream failure) as
        // swap-eligible so the partialRatingSubmit handler's error
        // template visibly replaces the form section. Other 4xx/5xx
        // responses keep the default error behavior.
        htmx.config.responseHandling = [
            { code: '204', swap: false },
            { code: '[23]..', swap: true },
            { code: '422', swap: true },
            { code: '502', swap: true },
            { code: '[45]..', swap: false, error: true },
            { code: '...', swap: false }
        ];
    </script>
```

The script runs immediately after `htmx.org@2.0.4` loads (both are synchronous script tags), so `htmx.config` is defined and mutable before HTMX scans the DOM for elements. The new entries for `422` and `502` come BEFORE the catch-all `[45]..` entry — order matters; HTMX uses the first-matching rule.

- [ ] **Step 3: Verify the layout still renders**

Run:

```bash
go test ./services/productpage/... -race -count=1 -run TestPartialRatingSubmit$
```

Expected: PASS — the existing happy-path test still loads the layout. (Template parse errors would manifest as test failures.)

- [ ] **Step 4: Commit**

```bash
git add services/productpage/templates/layout.html
git commit -s -m "feat(productpage/layout): treat 422 and 502 as HTMX swap-eligible

Adds an htmx.config.responseHandling override so the upcoming
partialRatingSubmit error responses (422 for validation, 502 for
upstream) actually swap the response template into the form section.
Other 4xx/5xx responses keep the default error behavior (no swap)."
```

---

## Task 2: Render ReviewWarning in the success template

**Files:**

- Modify: `services/productpage/templates/partials/rating-form.html` (success branch, around line 5)

The template currently has a binary `{{if .Success}} ... {{else}} ...`. Add a third optional element inside the success branch: a warning banner shown when `ReviewWarning` is set.

- [ ] **Step 1: Read the current success branch**

```bash
sed -n '1,15p' services/productpage/templates/partials/rating-form.html
```

Expected (verified):

```html
{{if .Success}}
<div class="notification-banner">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="M22 4L12 14.01l-3-3"/></svg>
    <div>
        <strong>Review submitted successfully!</strong> You gave {{.Stars}} star{{if gt .Stars 1}}s{{end}}.
    </div>
</div>
{{if .ProductID}}
<div hx-get="/partials/reviews/{{.ProductID}}"
     hx-trigger="load"
     hx-target="#reviews-section"
     hx-swap="innerHTML">
</div>
{{end}}
{{else}}
```

- [ ] **Step 2: Insert the ReviewWarning block between the success banner and the reviews-refresh trigger**

In `services/productpage/templates/partials/rating-form.html`, replace lines 1–14 (the success branch up to but not including `{{else}}`) with:

```html
{{if .Success}}
<div class="notification-banner">
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="M22 4L12 14.01l-3-3"/></svg>
    <div>
        <strong>Review submitted successfully!</strong> You gave {{.Stars}} star{{if gt .Stars 1}}s{{end}}.
    </div>
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
```

The `{{else}}` and the failure branch below it are unchanged.

- [ ] **Step 3: Verify the existing happy-path tests still pass**

Run:

```bash
go test ./services/productpage/... -race -count=1
```

Expected: existing 6 productpage tests all PASS. The new `{{if .ReviewWarning}}` block is gated by the absent field, so the existing tests (which don't set `ReviewWarning`) render exactly the same body as before.

- [ ] **Step 4: Commit**

```bash
git add services/productpage/templates/partials/rating-form.html
git commit -s -m "feat(productpage/templates): optional ReviewWarning on success branch

Adds a third optional rendering element to the rating-form success
branch: a warning banner shown when ReviewWarning is set on the
template data. Used by partialRatingSubmit to surface a degraded
reviews-service path while still confirming that the rating itself
succeeded."
```

---

## Task 3: Update partialRatingSubmit to return correct status codes

**Files:**

- Modify: `services/productpage/internal/handler/handler.go` (lines 267–324)

The handler has 4 error paths each calling `w.WriteHeader(http.StatusOK)`. Replace each with the right code per the spec:

| Source line | Trigger | New status |
|-------------|---------|------------|
| 272 | `r.ParseForm()` returns error | 422 |
| 287 | `strconv.Atoi(starsStr)` returns error | 422 |
| 300 | `h.ratingsClient.SubmitRating(...)` returns error | 502 |
| 308–315 | Review submit OR pending-store path | 200 with `ReviewWarning` |

- [ ] **Step 1: Read the existing partialRatingSubmit body**

```bash
sed -n '267,324p' services/productpage/internal/handler/handler.go
```

Expected: 58 lines starting with the function declaration. Confirms the four `w.WriteHeader(http.StatusOK)` calls live at lines 272, 287, 300, and (implicitly absent) for path D where the success branch fires regardless of review failure.

- [ ] **Step 2: Replace lines 267–324 with the corrected handler**

In `services/productpage/internal/handler/handler.go`, replace the entire `partialRatingSubmit` function with:

```go
func (h *Handler) partialRatingSubmit(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid form data",
		})
		return
	}

	productID := r.FormValue("product_id")
	reviewer := r.FormValue("reviewer")
	starsStr := r.FormValue("stars")
	reviewText := r.FormValue("text")

	stars, err := strconv.Atoi(starsStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   "Invalid stars value",
		})
		return
	}

	idempotencyKey := uuid.NewString()

	_, err = h.ratingsClient.SubmitRating(r.Context(), productID, reviewer, stars, idempotencyKey)
	if err != nil {
		logger.Warn("failed to submit rating", "error", err)
		w.WriteHeader(http.StatusBadGateway)
		_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
			"Success": false,
			"Error":   err.Error(),
		})
		return
	}

	var reviewWarning string
	if reviewText != "" {
		if err := h.reviewsClient.SubmitReview(r.Context(), productID, reviewer, reviewText, idempotencyKey); err != nil {
			logger.Warn("failed to submit review", "error", err)
			reviewWarning = "Your rating was submitted; the reviews service is unavailable — please retry the review separately."
		}

		if err := h.pendingStore.StorePending(r.Context(), productID, pending.NewReview(reviewer, reviewText, stars)); err != nil {
			logger.Warn("failed to store pending review", "error", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.templates.ExecuteTemplate(w, "rating-form.html", map[string]any{
		"Success":       true,
		"Stars":         stars,
		"ProductID":     productID,
		"ReviewWarning": reviewWarning,
	})
}
```

Notes for the diff:

- Lines 272, 287, 300: change `http.StatusOK` to `http.StatusUnprocessableEntity` (422), `http.StatusUnprocessableEntity` (422), and `http.StatusBadGateway` (502) respectively.
- The success block (last 6 lines) gains a `"ReviewWarning": reviewWarning` map entry — empty string when review path was healthy (or `reviewText` was empty), populated when SubmitReview failed.
- `pendingStore.StorePending` failure remains silent — the cache is best-effort; a miss does not change correctness for the user. The log warning is the only signal.

- [ ] **Step 3: Compile**

Run:

```bash
go build ./services/productpage/...
```

Expected: clean build, no output.

- [ ] **Step 4: Run existing tests**

Run:

```bash
go test ./services/productpage/... -race -count=1
```

Expected: all 6 existing tests PASS. The 3 happy-path tests for `partialRatingSubmit` (`TestPartialRatingSubmit`, `TestPartialRatingSubmitAsync`, `TestPartialRatingSubmitAsyncWithReview`) still hit the 200 success path with empty `ReviewWarning` — unchanged behavior.

- [ ] **Step 5: Commit**

```bash
git add services/productpage/internal/handler/handler.go
git commit -s -m "fix(productpage): proper HTTP status codes in partialRatingSubmit

Closes #40. Replaces unconditional 200 OK on every error path:

- ParseForm error           -> 422 Unprocessable Entity
- Invalid stars value       -> 422 Unprocessable Entity
- Ratings service failure   -> 502 Bad Gateway
- Review path failure       -> 200 with ReviewWarning (rating succeeded;
  review service degraded). Distinguishes 'everything failed' from
  'rating ok, review degraded' for downstream alerting.

The pendingStore.StorePending path stays silent — best-effort cache
miss doesn't change correctness."
```

---

## Task 4: Add unit tests for the four new error paths

**Files:**

- Modify: `services/productpage/internal/handler/handler_test.go` (append to end)

Four new tests, each following the existing `setupMockServers` / `httptest.NewServer` pattern. They cover the new status codes and (for path D) the `ReviewWarning` body content.

- [ ] **Step 1: Verify the existing helpers exist**

```bash
grep -n "setupMockServers\|templateDir\|projectRoot" services/productpage/internal/handler/handler_test.go | head -5
```

Expected: `setupMockServers` at line ~46, `templateDir` at line ~41, `projectRoot` at line ~23. The new tests reuse these.

- [ ] **Step 2: Append the four tests to handler_test.go**

Append the following to the end of `services/productpage/internal/handler/handler_test.go`:

```go
func TestPartialRatingSubmit_ParseFormFails(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Body larger than the 1MB MaxBytesReader limit.
	body := strings.Repeat("x", (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	if !strings.Contains(rec.Body.String(), "Invalid form data") {
		t.Errorf("expected 'Invalid form data' in body, got:\n%s", rec.Body.String())
	}
}

func TestPartialRatingSubmit_InvalidStars(t *testing.T) {
	detailsURL, reviewsURL, ratingsURL := setupMockServers(t)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(ratingsURL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=abc"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	if !strings.Contains(rec.Body.String(), "Invalid stars value") {
		t.Errorf("expected 'Invalid stars value' in body, got:\n%s", rec.Body.String())
	}
}

func TestPartialRatingSubmit_RatingsServiceFails(t *testing.T) {
	detailsURL, reviewsURL, _ := setupMockServers(t)

	// Override the ratings server with one that always errors.
	failingRatingsMux := http.NewServeMux()
	failingRatingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"ratings unavailable"}`))
	})
	failingRatingsServer := httptest.NewServer(failingRatingsMux)
	t.Cleanup(failingRatingsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(reviewsURL)
	ratingsClient := client.NewRatingsClient(failingRatingsServer.URL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Failed to submit:") {
		t.Errorf("expected 'Failed to submit:' in body, got:\n%s", body)
	}
}

func TestPartialRatingSubmit_ReviewSubmitFails(t *testing.T) {
	detailsURL, _, _ := setupMockServers(t)

	// Ratings succeeds.
	okRatingsMux := http.NewServeMux()
	okRatingsMux.HandleFunc("POST /v1/ratings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "rating-1",
			"product_id": "product-1",
			"reviewer":   "bob",
			"stars":      5,
		})
	})
	okRatingsServer := httptest.NewServer(okRatingsMux)
	t.Cleanup(okRatingsServer.Close)

	// Reviews POST fails.
	failingReviewsMux := http.NewServeMux()
	failingReviewsMux.HandleFunc("GET /v1/reviews/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"product_id": r.PathValue("id"),
			"reviews":    []any{},
			"pagination": map[string]any{"page": 1, "page_size": 10, "total_items": 0, "total_pages": 0},
		})
	})
	failingReviewsMux.HandleFunc("POST /v1/reviews", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"reviews unavailable"}`))
	})
	failingReviewsServer := httptest.NewServer(failingReviewsMux)
	t.Cleanup(failingReviewsServer.Close)

	detailsClient := client.NewDetailsClient(detailsURL)
	reviewsClient := client.NewReviewsClient(failingReviewsServer.URL)
	ratingsClient := client.NewRatingsClient(okRatingsServer.URL)

	h := handler.NewHandler(detailsClient, reviewsClient, ratingsClient, pending.NoopStore{}, templateDir(t))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	formData := "product_id=product-1&reviewer=bob&stars=5&text=Great+book"
	req := httptest.NewRequest(http.MethodPost, "/partials/rating", strings.NewReader(formData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Review submitted successfully!") {
		t.Errorf("expected success banner in body, got:\n%s", body)
	}
	if !strings.Contains(body, "reviews service is unavailable") {
		t.Errorf("expected ReviewWarning in body, got:\n%s", body)
	}
}
```

The test bodies use only the existing imports already in the file (`net/http`, `net/http/httptest`, `strings`, `testing`, plus `encoding/json` for the mock ratings response and `client`/`handler`/`pending` packages).

- [ ] **Step 3: Run the new tests**

Run:

```bash
go test ./services/productpage/... -race -count=1 -run TestPartialRatingSubmit_ -v
```

Expected: 4 PASS — the 4 new error-path tests. Times: well under 1s each.

- [ ] **Step 4: Run the full productpage suite to confirm no regressions**

Run:

```bash
go test ./services/productpage/... -race -count=1 -v
```

Expected: all 10 tests pass — the 6 pre-existing plus the 4 new.

- [ ] **Step 5: Commit**

```bash
git add services/productpage/internal/handler/handler_test.go
git commit -s -m "test(productpage): cover new partialRatingSubmit status codes

Adds four tests for the error paths corrected in the previous commit:

- ParseFormFails -> 422 + 'Invalid form data' in body
- InvalidStars   -> 422 + 'Invalid stars value' in body
- RatingsServiceFails -> 502 + form re-render
- ReviewSubmitFails   -> 200 + success banner + ReviewWarning text

The three pre-existing happy-path tests still pass unchanged because
ReviewWarning defaults to the empty string and is template-gated."
```

---

## Task 5: End-to-end verification on local k3d

**Files:** none modified — this task is verification-only. If anything fails here, return to the relevant earlier task to fix, then re-verify.

The smoke script will become reusable in a future PR (tracked under [#62](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/62) — CI k3d validation). For now we run it ad-hoc.

- [ ] **Step 1: Bring up a clean cluster**

Run:

```bash
make stop-k8s
make run-k8s
```

Expected: `make stop-k8s` exits 0 quickly. `make run-k8s` takes ~5 minutes; output ends with `Application layer complete.` and `make k8s-status` lists 11 deployments.

- [ ] **Step 2: Verify all deployments are Available**

Run:

```bash
kubectl --context=k3d-bookinfo-local rollout status deployment/productpage -n bookinfo --timeout=60s
kubectl --context=k3d-bookinfo-local rollout status deployment/ratings-write -n bookinfo --timeout=60s
kubectl --context=k3d-bookinfo-local rollout status deployment/reviews-write -n bookinfo --timeout=60s
```

Expected: all three report `successfully rolled out`.

- [ ] **Step 3: Happy path smoke**

Run:

```bash
curl -fsS -i -X POST \
  --data-urlencode 'product_id=product-1' \
  --data-urlencode 'reviewer=plan-step-3' \
  --data-urlencode 'stars=5' \
  http://localhost:8080/partials/rating | head -1
```

Expected first line: `HTTP/1.1 200 OK` (or HTTP/2). The body (omitted by `head -1`) should contain `Review submitted successfully!`.

- [ ] **Step 4: Validation error (path B) — invalid stars**

Run:

```bash
curl -i -X POST \
  --data-urlencode 'product_id=product-1' \
  --data-urlencode 'reviewer=plan-step-4' \
  --data-urlencode 'stars=abc' \
  http://localhost:8080/partials/rating 2>&1 | head -1
```

Expected first line: `HTTP/1.1 422 Unprocessable Entity` (or HTTP/2 422). Without `-f` so the non-2xx status doesn't kill curl.

- [ ] **Step 5: Upstream failure (path C) — ratings-write scaled down**

Run:

```bash
kubectl --context=k3d-bookinfo-local scale deploy/ratings-write -n bookinfo --replicas=0
kubectl --context=k3d-bookinfo-local rollout status deploy/ratings-write -n bookinfo --timeout=30s

# Give the gateway/sensor a moment to register the scale-down.
sleep 5

curl -i -X POST \
  --data-urlencode 'product_id=product-1' \
  --data-urlencode 'reviewer=plan-step-5' \
  --data-urlencode 'stars=4' \
  http://localhost:8080/partials/rating 2>&1 | head -1

# Restore.
kubectl --context=k3d-bookinfo-local scale deploy/ratings-write -n bookinfo --replicas=1
kubectl --context=k3d-bookinfo-local rollout status deploy/ratings-write -n bookinfo --timeout=60s
```

Expected first line of the curl: `HTTP/1.1 502 Bad Gateway` (or HTTP/2 502).

NOTE: ratings goes through the gateway → EventSource → Sensor path — scaling `ratings-write` only blocks the sensor target. The gateway POST itself may return 200 (the EventSource accepted the publish to Kafka). The path C error in `productpage` requires the **client** call to fail. productpage's `ratingsClient.SubmitRating` calls the gateway directly; the gateway publishes to Kafka and returns 200 to productpage.

**Implication:** path C cannot be triggered cleanly in k3d via scaling alone — the gateway answers regardless of whether the write Deployment is up. The end-to-end test for C is the unit test (Task 4 step 3); the k3d step here verifies the **happy path still works** even when ratings-write is scaled down (the rating flows through Kafka and waits for ratings-write to come back).

If you want to force a real 502 from productpage, add an env var like `RATINGS_SERVICE_URL=http://nonexistent.svc.cluster.local` to productpage and restart it — that produces a connect error in `ratingsClient`. Then submit a rating and confirm 502.

For this plan, the unit test in Task 4 covers the wire behavior; the k3d step verifies the system survives scale-down.

- [ ] **Step 6: Partial success (path D) — reviews-write scaled down**

Run:

```bash
kubectl --context=k3d-bookinfo-local scale deploy/reviews-write -n bookinfo --replicas=0
kubectl --context=k3d-bookinfo-local rollout status deploy/reviews-write -n bookinfo --timeout=30s

sleep 5

# Submit a rating WITH review text. Capture both status and body.
RESP=$(curl -i -X POST \
  --data-urlencode 'product_id=product-1' \
  --data-urlencode 'reviewer=plan-step-6' \
  --data-urlencode 'stars=5' \
  --data-urlencode 'text=k3d partial success probe' \
  http://localhost:8080/partials/rating 2>&1)

echo "$RESP" | head -1
echo "---"
echo "$RESP" | grep -E "Review submitted successfully|reviews service is unavailable" || echo "(no expected banner / warning found)"

# Restore.
kubectl --context=k3d-bookinfo-local scale deploy/reviews-write -n bookinfo --replicas=1
kubectl --context=k3d-bookinfo-local rollout status deploy/reviews-write -n bookinfo --timeout=60s
```

Expected:

- First line of response: `HTTP/1.1 200 OK` — rating succeeded.
- The grep block shows BOTH `Review submitted successfully!` AND `reviews service is unavailable`.

NOTE: same caveat as Step 5 — productpage calls reviews via the gateway. If the gateway accepts and returns 200 even with `reviews-write` down, the `reviewsClient.SubmitReview` call in productpage will not error and `ReviewWarning` won't fire. In that case the k3d step shows only the success banner, no warning — which is correct: from productpage's perspective the call did succeed.

To force a real D-path failure in k3d, point reviews to a nonexistent service via env var as described in Step 5.

The unit test in Task 4 step 4 (`TestPartialRatingSubmit_ReviewSubmitFails`) is the authoritative verification that the warning renders correctly when the client errors.

- [ ] **Step 7: Confirm rating IS in the system after a partial-success scenario**

Run (regardless of whether Step 6 surfaced the warning):

```bash
curl -fsS http://localhost:8080/v1/ratings/product-1 | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'count={d[\"count\"]}, average={d[\"average\"]}')"
```

Expected: a `count` ≥ the number of successful POSTs we made above (3+ in steps 3, 5, 6). The rating data flowed through.

- [ ] **Step 8: Browser sanity check (manual, single-step)**

Open `http://localhost:8080/products/product-1` in a browser. Submit the rating form with a non-numeric stars value (force-edit the dropdown via DevTools, or submit empty). Confirm:

- The form section visibly REPLACES with the error template (red banner reading `Failed to submit: ...`).
- HTMX did the swap (new content visible) — not just a console error.

This is the ONE non-automatable step. If the swap doesn't visibly render, Task 1 (HTMX config) didn't take effect — re-check the layout.html change.

- [ ] **Step 9: Run the compose-mode e2e suite**

Run:

```bash
make e2e
```

Expected: PASS. The compose mode doesn't exercise productpage's HTMX form at all (only HTTP-level acceptance checks against the JSON APIs), but a green run confirms no other regressions.

- [ ] **Step 10: Final lint + test sweep**

Run:

```bash
make lint
go test ./... -race -count=1 -short
```

Expected: 0 issues, all tests pass.

- [ ] **Step 11: No commit**

Verification only. If everything above passes, the implementation is complete and ready for PR.

---

## Self-Review

**1. Spec coverage**

| Spec section | Implementing task |
|--------------|-------------------|
| Status code mapping (4 paths: 422/422/502/200-with-warning) | Task 3 |
| Template `ReviewWarning` field rendering | Task 2 |
| HTMX `responseHandling` config | Task 1 |
| Unit tests for the 4 error paths | Task 4 |
| End-to-end k3d verification (clean cycle, scale-down, browser swap) | Task 5 |
| `partialDeleteReview` explicitly out of scope | Not implemented (correct) |
| k6 test already correct (no change needed) | Not implemented (correct) |

**2. Placeholder scan**

No `TBD`, `TODO`, "implement later", or "fill in details" present. Every code block contains exact code; every command shows the expected output (status line, test count, etc.).

**3. Type / signature consistency**

- `ReviewWarning` is consistently a `string` (Go) and a `{{if .ReviewWarning}}` template guard.
- Status code names use the stdlib constants: `http.StatusUnprocessableEntity` (422), `http.StatusBadGateway` (502), `http.StatusOK` (200) — checked across Tasks 3 and 4.
- The new tests reuse `setupMockServers`, `templateDir`, and the existing `client.New*Client` constructors — no new helpers introduced.

**4. Scope check**

Single PR territory. 4 files modified, ~140 lines added, ~12 removed. Verifiable end-to-end on local k3d in 5 minutes plus the cluster bring-up.

No issues found in self-review.

---

## Notes on path C and D in k3d

The k3d verification steps for paths C and D (Steps 5 and 6) are weakened because productpage's clients call the **gateway**, not the write Deployments directly. The gateway accepts the POST, publishes to Kafka, and returns 200 regardless of whether the downstream write Deployment is healthy. So scaling write Deployments down does not surface a 502 to productpage in the way the spec describes.

The wire-level `502` and `200-with-warning` paths ARE properly verified by the unit tests in Task 4 (they use mock servers that error directly). The k3d steps add value by confirming the system survives scale-down without crashing or hanging — the gateway+Kafka path is resilient.

To trigger a real productpage→502 in k3d, override `RATINGS_SERVICE_URL` to point at a nonexistent service (causes a TCP connect error in `ratingsClient`). This is a follow-up enhancement to the k3d smoke and is captured in [#62](https://github.com/kaio6fellipe/event-driven-bookinfo/issues/62).
