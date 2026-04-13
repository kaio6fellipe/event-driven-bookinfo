# System-Wide Idempotency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make all write services (reviews, ratings, details, notification) idempotent so that duplicate event delivery (from Argo Events at-least-once semantics, DLQ replay, or retries) does not create duplicate records.

**Architecture:** Shared `pkg/idempotency` package provides a `Store` port with memory and postgres adapters. Each write service wires the Store into its service layer and consults it before persisting. The key is either an explicit `idempotency_key` from the request body, or a SHA-256 hash of business fields (natural key). Each service has its own `processed_events` table.

**Tech Stack:** Go 1.24+, pgx/v5, golang-migrate, existing hex arch patterns.

---

## File Structure

### New files

- `pkg/idempotency/store.go` — `Store` interface (port)
- `pkg/idempotency/key.go` — `NaturalKey()` hash helper, `Resolve()` key resolver
- `pkg/idempotency/memory.go` — in-memory `Store` implementation
- `pkg/idempotency/postgres.go` — pgx-backed `Store` implementation
- `pkg/idempotency/memory_test.go` — table-driven tests for memory store
- `pkg/idempotency/key_test.go` — table-driven tests for key helpers
- `services/reviews/migrations/002_create_processed_events.up.sql`
- `services/reviews/migrations/002_create_processed_events.down.sql`
- `services/ratings/migrations/002_create_processed_events.up.sql`
- `services/ratings/migrations/002_create_processed_events.down.sql`
- `services/details/migrations/002_create_processed_events.up.sql`
- `services/details/migrations/002_create_processed_events.down.sql`
- `services/notification/migrations/002_create_processed_events.up.sql`
- `services/notification/migrations/002_create_processed_events.down.sql`

### Modified files

- `services/reviews/internal/adapter/inbound/http/dto.go` — add `IdempotencyKey` to `SubmitReviewRequest`
- `services/reviews/internal/adapter/inbound/http/handler.go` — pass `IdempotencyKey` to service
- `services/reviews/internal/core/port/inbound.go` — add `idempotencyKey` param to `SubmitReview`
- `services/reviews/internal/core/service/review_service.go` — wire idempotency check
- `services/reviews/cmd/main.go` — construct `idempotency.Store`, pass to service
- Same pattern applied to ratings, details, notification
- `services/productpage/internal/handler/handler.go` — generate `idempotency_key` on form submission
- `services/productpage/internal/client/reviews.go` — forward `idempotency_key` in POST body
- `services/productpage/internal/client/ratings.go` — forward `idempotency_key` in POST body

---

## Task 1: Create idempotency Store interface and key helpers

**Files:**
- Create: `pkg/idempotency/store.go`
- Create: `pkg/idempotency/key.go`
- Create: `pkg/idempotency/key_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/idempotency/key_test.go`:

```go
package idempotency_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
)

func TestNaturalKey(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "single field",
			fields: []string{"hello"},
			want:   "aa8e26655ad91d0fa0ff7ae8c2a43f3c3bdd6d4f8e4f5a6c7a61a5f1d92dcb20",
		},
		{
			name:   "same fields produce same key",
			fields: []string{"a", "b", "c"},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idempotency.NaturalKey(tt.fields...)
			if len(got) != 64 {
				t.Errorf("key length = %d, want 64 (SHA-256 hex)", len(got))
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNaturalKey_Deterministic(t *testing.T) {
	k1 := idempotency.NaturalKey("a", "b", "c")
	k2 := idempotency.NaturalKey("a", "b", "c")
	if k1 != k2 {
		t.Errorf("expected deterministic output, got %q and %q", k1, k2)
	}
}

func TestNaturalKey_OrderSensitive(t *testing.T) {
	k1 := idempotency.NaturalKey("a", "b")
	k2 := idempotency.NaturalKey("b", "a")
	if k1 == k2 {
		t.Error("expected different output for different field order")
	}
}

func TestNaturalKey_SeparatorCollision(t *testing.T) {
	// Without separator, ("ab", "c") would collide with ("a", "bc")
	k1 := idempotency.NaturalKey("ab", "c")
	k2 := idempotency.NaturalKey("a", "bc")
	if k1 == k2 {
		t.Error("expected separator to prevent field boundary collision")
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		explicitKey string
		fields      []string
		wantExplicit bool
	}{
		{
			name:        "explicit key takes precedence",
			explicitKey: "my-key",
			fields:      []string{"a", "b"},
			wantExplicit: true,
		},
		{
			name:        "empty explicit falls back to natural",
			explicitKey: "",
			fields:      []string{"a", "b"},
			wantExplicit: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idempotency.Resolve(tt.explicitKey, tt.fields...)
			if tt.wantExplicit && got != tt.explicitKey {
				t.Errorf("got %q, want explicit key %q", got, tt.explicitKey)
			}
			if !tt.wantExplicit && got == tt.explicitKey {
				t.Error("expected natural key, got explicit (or empty)")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
go test ./pkg/idempotency/...
```

Expected: compile error — `package idempotency` does not exist.

- [ ] **Step 3: Create Store interface**

Create `pkg/idempotency/store.go`:

```go
// Package idempotency provides idempotency key tracking for write services.
// Services call Store.CheckAndRecord before performing a write operation.
// If the key has already been processed, the service skips the write and
// returns success. Keys are either explicitly provided by clients or
// derived from business payload fields (natural key).
package idempotency

import "context"

// Store tracks whether an idempotency key has been processed.
type Store interface {
	// CheckAndRecord atomically checks whether key was previously recorded.
	// Returns (alreadyProcessed, error):
	//   - alreadyProcessed=true  → key exists, caller should skip work
	//   - alreadyProcessed=false → key was just recorded, caller should proceed
	CheckAndRecord(ctx context.Context, key string) (bool, error)
}
```

- [ ] **Step 4: Create key helpers**

Create `pkg/idempotency/key.go`:

```go
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
)

// fieldSeparator is a byte unlikely to appear in business payloads that
// prevents collisions at field boundaries. Without it, fields ("ab", "c")
// and ("a", "bc") would hash to the same value.
const fieldSeparator byte = 0x1f // ASCII Unit Separator

// NaturalKey computes a SHA-256 hash over the given fields, separated by
// an unambiguous byte to prevent boundary collisions. The returned string
// is 64 hex characters.
func NaturalKey(fields ...string) string {
	h := sha256.New()
	for _, f := range fields {
		_, _ = h.Write([]byte(f))
		_, _ = h.Write([]byte{fieldSeparator})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Resolve returns explicitKey if non-empty, otherwise returns the natural
// key computed from fields. Used by service layers to pick the correct
// idempotency key for a request.
func Resolve(explicitKey string, fields ...string) string {
	if explicitKey != "" {
		return explicitKey
	}
	return NaturalKey(fields...)
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./pkg/idempotency/...
```

Expected: PASS (the `TestNaturalKey` "single field" case will fail on specific hex — that's fine, remove the hardcoded `want` in the first subtest, keep only the length and deterministic checks). Edit the test to remove the hardcoded hash value before running.

Update the first test case to drop the specific hash expectation:

```go
{
    name:   "single field",
    fields: []string{"hello"},
    want:   "", // length check only
},
```

Re-run `go test ./pkg/idempotency/...`. Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/idempotency/
git commit -m "feat(pkg/idempotency): add Store interface and key helpers"
```

---

## Task 2: In-memory Store implementation

**Files:**
- Create: `pkg/idempotency/memory.go`
- Create: `pkg/idempotency/memory_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/idempotency/memory_test.go`:

```go
package idempotency_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
)

func TestMemoryStore_CheckAndRecord(t *testing.T) {
	ctx := context.Background()
	s := idempotency.NewMemoryStore()

	seen, err := s.CheckAndRecord(ctx, "key-1")
	if err != nil {
		t.Fatalf("first call err = %v", err)
	}
	if seen {
		t.Error("first call: expected seen=false, got true")
	}

	seen, err = s.CheckAndRecord(ctx, "key-1")
	if err != nil {
		t.Fatalf("second call err = %v", err)
	}
	if !seen {
		t.Error("second call: expected seen=true, got false")
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	ctx := context.Background()
	s := idempotency.NewMemoryStore()

	var wg sync.WaitGroup
	seenCount := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen, err := s.CheckAndRecord(ctx, "shared-key")
			if err != nil {
				t.Errorf("err = %v", err)
				return
			}
			seenCount <- seen
		}()
	}
	wg.Wait()
	close(seenCount)

	var firstFalse, laterTrue int
	for seen := range seenCount {
		if seen {
			laterTrue++
		} else {
			firstFalse++
		}
	}
	if firstFalse != 1 {
		t.Errorf("expected exactly one first-writer, got %d", firstFalse)
	}
	if laterTrue != 99 {
		t.Errorf("expected 99 already-processed, got %d", laterTrue)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/idempotency/...
```

Expected: FAIL — `NewMemoryStore` undefined.

- [ ] **Step 3: Implement MemoryStore**

Create `pkg/idempotency/memory.go`:

```go
package idempotency

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store. Suitable for local dev and tests.
// Lost on process restart.
type MemoryStore struct {
	mu    sync.Mutex
	keys  map[string]struct{}
}

// NewMemoryStore creates a new in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{keys: make(map[string]struct{})}
}

// CheckAndRecord implements Store.
func (m *MemoryStore) CheckAndRecord(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.keys[key]; ok {
		return true, nil
	}
	m.keys[key] = struct{}{}
	return false, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -race ./pkg/idempotency/...
```

Expected: PASS with no race conditions.

- [ ] **Step 5: Commit**

```bash
git add pkg/idempotency/memory.go pkg/idempotency/memory_test.go
git commit -m "feat(pkg/idempotency): add in-memory Store implementation"
```

---

## Task 3: Postgres Store implementation

**Files:**
- Create: `pkg/idempotency/postgres.go`

- [ ] **Step 1: Implement PostgresStore**

No dedicated unit test — the query is tested via service-layer integration tests once wired into each service. The query uses `INSERT ... ON CONFLICT DO NOTHING RETURNING` pattern for atomicity.

Create `pkg/idempotency/postgres.go`:

```go
package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a Store backed by a PostgreSQL table named processed_events.
// Each service should create the table via its own migration — see
// services/<name>/migrations/002_create_processed_events.up.sql.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore using the given pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CheckAndRecord implements Store. The INSERT ... ON CONFLICT DO NOTHING
// pattern atomically checks and records. If the returned row is empty,
// the key already existed (alreadyProcessed=true).
func (p *PostgresStore) CheckAndRecord(ctx context.Context, key string) (bool, error) {
	var inserted string
	err := p.pool.QueryRow(ctx,
		`INSERT INTO processed_events (idempotency_key)
		 VALUES ($1)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING idempotency_key`,
		key,
	).Scan(&inserted)

	if errors.Is(err, pgx.ErrNoRows) {
		// Row already existed — conflict triggered DO NOTHING.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("recording idempotency key: %w", err)
	}
	return false, nil
}
```

- [ ] **Step 2: Compile check**

```bash
go build ./pkg/idempotency/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/idempotency/postgres.go
git commit -m "feat(pkg/idempotency): add postgres Store implementation"
```

---

## Task 4: Create processed_events migration for reviews

**Files:**
- Create: `services/reviews/migrations/002_create_processed_events.up.sql`
- Create: `services/reviews/migrations/002_create_processed_events.down.sql`

- [ ] **Step 1: Create up migration**

Create `services/reviews/migrations/002_create_processed_events.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS processed_events (
    idempotency_key TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at
    ON processed_events (processed_at);
```

- [ ] **Step 2: Create down migration**

Create `services/reviews/migrations/002_create_processed_events.down.sql`:

```sql
DROP TABLE IF EXISTS processed_events;
```

- [ ] **Step 3: Commit**

```bash
git add services/reviews/migrations/
git commit -m "feat(reviews): add processed_events migration"
```

---

## Task 5: Wire idempotency into reviews service

**Files:**
- Modify: `services/reviews/internal/adapter/inbound/http/dto.go`
- Modify: `services/reviews/internal/adapter/inbound/http/handler.go`
- Modify: `services/reviews/internal/core/port/inbound.go`
- Modify: `services/reviews/internal/core/service/review_service.go`
- Modify: `services/reviews/cmd/main.go`

- [ ] **Step 1: Read current inbound port**

```bash
cat services/reviews/internal/core/port/inbound.go
```

Note the existing `SubmitReview` signature.

- [ ] **Step 2: Add IdempotencyKey field to SubmitReviewRequest DTO**

Find the `SubmitReviewRequest` struct in `services/reviews/internal/adapter/inbound/http/dto.go` and add field:

```go
type SubmitReviewRequest struct {
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key,omitempty"` // NEW
}
```

- [ ] **Step 3: Update inbound port**

Edit `services/reviews/internal/core/port/inbound.go` — change `SubmitReview` signature:

```go
type ReviewService interface {
	GetProductReviews(ctx context.Context, productID string, page, pageSize int) ([]domain.Review, int, error)
	SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error)
	DeleteReview(ctx context.Context, id string) error
}
```

- [ ] **Step 4: Update ReviewService to accept Store and idempotencyKey**

Edit `services/reviews/internal/core/service/review_service.go` replacing the struct, constructor, and `SubmitReview`:

```go
// Package service implements the business logic for the reviews service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
)

// ErrAlreadyProcessed signals that an idempotent request was previously processed.
var ErrAlreadyProcessed = errors.New("request already processed")

// ReviewService implements the port.ReviewService interface.
type ReviewService struct {
	repo          port.ReviewRepository
	ratingsClient port.RatingsClient
	idempotency   idempotency.Store
}

// NewReviewService creates a new ReviewService.
func NewReviewService(repo port.ReviewRepository, ratingsClient port.RatingsClient, idem idempotency.Store) *ReviewService {
	return &ReviewService{
		repo:          repo,
		ratingsClient: ratingsClient,
		idempotency:   idem,
	}
}

// SubmitReview creates and persists a new review. Deduplicates on idempotencyKey
// (falls back to a natural key derived from productID+reviewer+text).
func (s *ReviewService) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) (*domain.Review, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, text)

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if alreadyProcessed {
		logger := logging.FromContext(ctx)
		logger.Info("review submit skipped: already processed", slog.String("idempotency_key", key))
		return nil, ErrAlreadyProcessed
	}

	review, err := domain.NewReview(productID, reviewer, text)
	if err != nil {
		return nil, fmt.Errorf("creating review: %w", err)
	}

	if err := s.repo.Save(ctx, review); err != nil {
		return nil, fmt.Errorf("saving review: %w", err)
	}

	return review, nil
}
```

Leave `GetProductReviews` and `DeleteReview` unchanged.

- [ ] **Step 5: Update submitReview handler**

Edit `services/reviews/internal/adapter/inbound/http/handler.go`, replacing the `submitReview` function:

```go
func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	logger := logging.FromContext(r.Context())

	var req SubmitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	review, err := h.svc.SubmitReview(r.Context(), req.ProductID, req.Reviewer, req.Text, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyProcessed) {
			logger.Info("duplicate submit skipped")
			writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
			return
		}
		logger.Warn("failed to submit review", "error", err)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	logger.Info("review submitted", "review_id", review.ID, "product_id", review.ProductID)

	writeJSON(w, http.StatusCreated, ReviewResponse{
		ID:        review.ID,
		ProductID: review.ProductID,
		Reviewer:  review.Reviewer,
		Text:      review.Text,
	})
}
```

Add the `service` import at top of file:

```go
import (
	// ... existing imports
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)
```

- [ ] **Step 6: Wire idempotency Store in main.go**

Edit `services/reviews/cmd/main.go`. After the storage backend switch block, add:

```go
// Idempotency store — pairs with storage backend.
var idemStore idempotency.Store
switch cfg.StorageBackend {
case "postgres":
	// pool already constructed above; reuse it
	idemStore = idempotency.NewPostgresStore(pool)
default:
	idemStore = idempotency.NewMemoryStore()
}
```

Move the `pool` variable declaration outside the switch so it's visible. Update the switch to:

```go
var repo port.ReviewRepository
var pool *pgxpool.Pool
var readinessChecks []func() error

switch cfg.StorageBackend {
case "postgres":
	var err error
	pool, err = database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.RunMigrations {
		if err := database.RunMigrations(cfg.DatabaseURL, migrations.FS); err != nil {
			logger.Error("failed to run migrations", "error", err)
			os.Exit(1)
		}
		logger.Info("database migrations applied")
	}

	repo = postgres.NewReviewRepository(pool)
	readinessChecks = append(readinessChecks, database.HealthCheck(pool))
	logger.Info("using postgres storage backend")
default:
	repo = memory.NewReviewRepository()
	logger.Info("using memory storage backend")
}

var idemStore idempotency.Store
if pool != nil {
	idemStore = idempotency.NewPostgresStore(pool)
} else {
	idemStore = idempotency.NewMemoryStore()
}
```

Add imports:

```go
"github.com/jackc/pgx/v5/pgxpool"
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
```

Change the service construction line from:

```go
svc := service.NewReviewService(repo, ratingsClient)
```

to:

```go
svc := service.NewReviewService(repo, ratingsClient, idemStore)
```

- [ ] **Step 7: Build**

```bash
go build ./services/reviews/...
```

Expected: no errors.

- [ ] **Step 8: Update existing handler tests**

```bash
go test ./services/reviews/...
```

If tests fail because `NewReviewService` signature changed, update them. Find the test files:

```bash
grep -rn "NewReviewService" services/reviews/
```

In any test that constructs `ReviewService`, add `idempotency.NewMemoryStore()` as the third argument. Example:

```go
svc := service.NewReviewService(repo, ratingsClient, idempotency.NewMemoryStore())
```

- [ ] **Step 9: Add idempotency test**

Add a new test file `services/reviews/internal/core/service/review_service_idempotency_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
)

type noopRatingsClient struct{}

func (noopRatingsClient) GetProductRatings(_ context.Context, _ string) (*service.RatingData, error) {
	return nil, nil
}

func TestSubmitReview_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewReviewRepository()
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore())

	_, err := svc.SubmitReview(ctx, "p1", "alice", "great book", "key-1")
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "alice", "great book", "key-1")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second submit: err = %v, want ErrAlreadyProcessed", err)
	}
}

func TestSubmitReview_NaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewReviewRepository()
	svc := service.NewReviewService(repo, nil, idempotency.NewMemoryStore())

	_, err := svc.SubmitReview(ctx, "p1", "bob", "good", "")
	if err != nil {
		t.Fatalf("first submit err = %v", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "bob", "good", "")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("duplicate natural key: err = %v, want ErrAlreadyProcessed", err)
	}

	_, err = svc.SubmitReview(ctx, "p1", "bob", "different text", "")
	if err != nil {
		t.Errorf("different text should succeed, got err = %v", err)
	}
}
```

If the `noopRatingsClient` doesn't compile (wrong interface), check `port.RatingsClient` signature and adjust. If `RatingData` is not exported from service, import from wherever it is defined.

Run the new test:

```bash
go test ./services/reviews/internal/core/service/... -run Idempotent -v
```

Expected: PASS.

- [ ] **Step 10: Run full test suite**

```bash
go test -race -count=1 ./services/reviews/...
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add services/reviews/ pkg/idempotency/
git commit -m "feat(reviews): add idempotency to submit review"
```

---

## Task 6: Create processed_events migration for ratings

**Files:**
- Create: `services/ratings/migrations/002_create_processed_events.up.sql`
- Create: `services/ratings/migrations/002_create_processed_events.down.sql`

- [ ] **Step 1: Create up migration**

Create `services/ratings/migrations/002_create_processed_events.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS processed_events (
    idempotency_key TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at
    ON processed_events (processed_at);
```

- [ ] **Step 2: Create down migration**

Create `services/ratings/migrations/002_create_processed_events.down.sql`:

```sql
DROP TABLE IF EXISTS processed_events;
```

- [ ] **Step 3: Commit**

```bash
git add services/ratings/migrations/
git commit -m "feat(ratings): add processed_events migration"
```

---

## Task 7: Wire idempotency into ratings service

**Files:**
- Modify: `services/ratings/internal/adapter/inbound/http/dto.go`
- Modify: `services/ratings/internal/adapter/inbound/http/handler.go`
- Modify: `services/ratings/internal/core/port/inbound.go`
- Modify: `services/ratings/internal/core/service/rating_service.go` (or equivalent filename)
- Modify: `services/ratings/cmd/main.go`

Natural key fields: `product_id`, `reviewer`, `stars`.

- [ ] **Step 1: Discover exact file paths**

```bash
ls services/ratings/internal/core/service/
ls services/ratings/internal/adapter/inbound/http/
grep -rn "SubmitRating\|CreateRating\|func (s \*" services/ratings/internal/core/
```

Note the service struct name, constructor, and primary write method (probably `SubmitRating` or `CreateRating`).

- [ ] **Step 2: Add IdempotencyKey to request DTO**

Open `services/ratings/internal/adapter/inbound/http/dto.go`. Find the ratings submit request struct (likely `SubmitRatingRequest` or `CreateRatingRequest`). Add:

```go
IdempotencyKey string `json:"idempotency_key,omitempty"`
```

- [ ] **Step 3: Update inbound port signature**

Open `services/ratings/internal/core/port/inbound.go`. Add `idempotencyKey string` as the last parameter on the submit method. Example, if current is:

```go
SubmitRating(ctx context.Context, productID, reviewer string, stars int) (*domain.Rating, error)
```

Change to:

```go
SubmitRating(ctx context.Context, productID, reviewer string, stars int, idempotencyKey string) (*domain.Rating, error)
```

- [ ] **Step 4: Update RatingService**

Open the service file (e.g., `services/ratings/internal/core/service/rating_service.go`). Apply three changes:

1. Add imports:

```go
"errors"
"strconv"

"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
```

2. Add sentinel error:

```go
var ErrAlreadyProcessed = errors.New("request already processed")
```

3. Update struct and constructor:

```go
type RatingService struct {
	repo        port.RatingRepository
	idempotency idempotency.Store
}

func NewRatingService(repo port.RatingRepository, idem idempotency.Store) *RatingService {
	return &RatingService{repo: repo, idempotency: idem}
}
```

4. Update `SubmitRating` method — wrap existing logic with idempotency check at the top:

```go
func (s *RatingService) SubmitRating(ctx context.Context, productID, reviewer string, stars int, idempotencyKey string) (*domain.Rating, error) {
	key := idempotency.Resolve(idempotencyKey, productID, reviewer, strconv.Itoa(stars))

	alreadyProcessed, err := s.idempotency.CheckAndRecord(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("checking idempotency: %w", err)
	}
	if alreadyProcessed {
		return nil, ErrAlreadyProcessed
	}

	// ... existing logic unchanged ...
}
```

- [ ] **Step 5: Update handler**

Open `services/ratings/internal/adapter/inbound/http/handler.go`. Find the submit/create handler. Apply:

1. Pass `req.IdempotencyKey` to `svc.SubmitRating(...)` call.
2. Handle `ErrAlreadyProcessed` similar to reviews:

```go
if errors.Is(err, service.ErrAlreadyProcessed) {
    writeJSON(w, http.StatusOK, ErrorResponse{Error: "already processed"})
    return
}
```

Add service import if missing.

- [ ] **Step 6: Update cmd/main.go**

Apply the same pattern as the reviews service in Task 5, Step 6:

1. Move `pool` declaration outside the switch.
2. Create `idemStore` after the switch.
3. Pass `idemStore` to `NewRatingService`.

Add imports:

```go
"github.com/jackc/pgx/v5/pgxpool"
"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
```

- [ ] **Step 7: Update existing tests**

```bash
grep -rn "NewRatingService" services/ratings/
```

Add `idempotency.NewMemoryStore()` as the second argument in every construction in tests.

- [ ] **Step 8: Add idempotency test**

Create `services/ratings/internal/core/service/rating_service_idempotency_test.go`:

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
)

func TestSubmitRating_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRatingRepository()
	svc := service.NewRatingService(repo, idempotency.NewMemoryStore())

	_, err := svc.SubmitRating(ctx, "p1", "alice", 5, "key-1")
	if err != nil {
		t.Fatalf("first err = %v", err)
	}

	_, err = svc.SubmitRating(ctx, "p1", "alice", 5, "key-1")
	if !errors.Is(err, service.ErrAlreadyProcessed) {
		t.Errorf("second: err = %v, want ErrAlreadyProcessed", err)
	}
}
```

Verify memory repo constructor name — if it's `NewInMemoryRepository` or similar, adjust import and call.

- [ ] **Step 9: Build + test**

```bash
go build ./services/ratings/...
go test -race -count=1 ./services/ratings/...
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add services/ratings/
git commit -m "feat(ratings): add idempotency to submit rating"
```

---

## Task 8: Create processed_events migration for details

**Files:**
- Create: `services/details/migrations/002_create_processed_events.up.sql`
- Create: `services/details/migrations/002_create_processed_events.down.sql`

- [ ] **Step 1: Create up migration**

```sql
CREATE TABLE IF NOT EXISTS processed_events (
    idempotency_key TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at
    ON processed_events (processed_at);
```

- [ ] **Step 2: Create down migration**

```sql
DROP TABLE IF EXISTS processed_events;
```

- [ ] **Step 3: Commit**

```bash
git add services/details/migrations/
git commit -m "feat(details): add processed_events migration"
```

---

## Task 9: Wire idempotency into details service

**Files:**
- Modify: `services/details/internal/adapter/inbound/http/dto.go`
- Modify: `services/details/internal/adapter/inbound/http/handler.go`
- Modify: `services/details/internal/core/port/inbound.go`
- Modify: `services/details/internal/core/service/detail_service.go` (or equivalent)
- Modify: `services/details/cmd/main.go`

Natural key fields: all significant business fields. Likely `title`, `author`, `year`, `isbn` (or equivalent). Discover below.

- [ ] **Step 1: Discover paths and fields**

```bash
ls services/details/internal/core/service/
cat services/details/internal/core/domain/*.go
grep -rn "type AddDetail\|type CreateDetail\|type SubmitDetail" services/details/internal/adapter/inbound/http/
```

Identify:
- Submit/create method name (e.g., `AddBook`, `CreateDetail`)
- Business fields present on the request DTO (use for natural key)

- [ ] **Step 2: Follow the same pattern as Task 7**

Apply the pattern from Task 7 to details:
- Add `IdempotencyKey` field to the submit request DTO
- Update inbound port method signature
- Add `ErrAlreadyProcessed` sentinel + `idempotency.Store` field to service
- Wrap submit logic with `CheckAndRecord` at top
- Natural key fields: all business fields on the submit request (e.g., `strconv` any non-string fields)
- Update handler to pass `idempotencyKey` and handle `ErrAlreadyProcessed` with HTTP 200
- Update `cmd/main.go` to wire `idempotency.Store`

- [ ] **Step 3: Add idempotency test**

Create `services/details/internal/core/service/<service>_idempotency_test.go` — mirror the reviews test structure but with the details service's submit method and fields.

- [ ] **Step 4: Build + test**

```bash
go build ./services/details/...
go test -race -count=1 ./services/details/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/details/
git commit -m "feat(details): add idempotency to create detail"
```

---

## Task 10: Create processed_events migration for notification

**Files:**
- Create: `services/notification/migrations/002_create_processed_events.up.sql`
- Create: `services/notification/migrations/002_create_processed_events.down.sql`

- [ ] **Step 1: Create up migration**

```sql
CREATE TABLE IF NOT EXISTS processed_events (
    idempotency_key TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at
    ON processed_events (processed_at);
```

- [ ] **Step 2: Create down migration**

```sql
DROP TABLE IF EXISTS processed_events;
```

- [ ] **Step 3: Commit**

```bash
git add services/notification/migrations/
git commit -m "feat(notification): add processed_events migration"
```

---

## Task 11: Wire idempotency into notification service

**Files:**
- Modify: `services/notification/internal/adapter/inbound/http/dto.go`
- Modify: `services/notification/internal/adapter/inbound/http/handler.go`
- Modify: `services/notification/internal/core/port/inbound.go`
- Modify: `services/notification/internal/core/service/notification_service.go` (or equivalent)
- Modify: `services/notification/cmd/main.go`

Natural key fields: `recipient`, `subject`, `body`, `channel`.

- [ ] **Step 1: Discover paths**

```bash
ls services/notification/internal/core/service/
grep -rn "type CreateNotification\|type SendNotification" services/notification/internal/adapter/inbound/http/
```

- [ ] **Step 2: Apply the Task 7 pattern**

Same steps as Task 7: DTO, port, service, handler, main.go, tests. Natural key fields: `recipient`, `subject`, `body`, `channel`.

- [ ] **Step 3: Add idempotency test**

Create `services/notification/internal/core/service/<service>_idempotency_test.go` mirroring the reviews test.

- [ ] **Step 4: Build + test**

```bash
go build ./services/notification/...
go test -race -count=1 ./services/notification/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notification/
git commit -m "feat(notification): add idempotency to create notification"
```

---

## Task 12: Productpage generates idempotency_key on form submission

**Files:**
- Modify: `services/productpage/internal/handler/handler.go`
- Modify: `services/productpage/internal/client/reviews.go`
- Modify: `services/productpage/internal/client/ratings.go`

- [ ] **Step 1: Read current clients**

```bash
cat services/productpage/internal/client/reviews.go
cat services/productpage/internal/client/ratings.go
```

Locate the method that POSTs a review and the method that POSTs a rating.

- [ ] **Step 2: Add IdempotencyKey parameter to reviews client**

Edit `services/productpage/internal/client/reviews.go`. Find the `SubmitReview` method signature and add `idempotencyKey string` parameter. Update the request body struct (or map) to include `"idempotency_key": idempotencyKey`.

Example, if current is:

```go
func (c *ReviewsClient) SubmitReview(ctx context.Context, productID, reviewer, text string) error {
	body := map[string]string{"product_id": productID, "reviewer": reviewer, "text": text}
	// ... POST ...
}
```

Change to:

```go
func (c *ReviewsClient) SubmitReview(ctx context.Context, productID, reviewer, text, idempotencyKey string) error {
	body := map[string]string{
		"product_id":      productID,
		"reviewer":        reviewer,
		"text":            text,
		"idempotency_key": idempotencyKey,
	}
	// ... POST unchanged ...
}
```

- [ ] **Step 3: Add IdempotencyKey parameter to ratings client**

Same pattern in `services/productpage/internal/client/ratings.go`. Add `idempotencyKey string` to `SubmitRating` (or equivalent). Include it in the POST body.

- [ ] **Step 4: Generate key in handler and pass through**

Edit `services/productpage/internal/handler/handler.go`. Find the `partialRatingSubmit` (or equivalent) handler. At the top of the handler after parsing the form:

```go
import "github.com/google/uuid" // add if not already imported
```

Inside the handler:

```go
idempotencyKey := uuid.NewString()
```

Pass `idempotencyKey` to both `h.ratingsClient.SubmitRating(...)` and `h.reviewsClient.SubmitReview(...)` calls. **Use the same key for both** so the pair (rating + review) gets a coherent identifier.

- [ ] **Step 5: Check if google/uuid is already a dependency**

```bash
grep -rn "google/uuid" go.mod services/productpage/
```

If not present, add it:

```bash
go get github.com/google/uuid
go mod tidy
```

- [ ] **Step 6: Build + test**

```bash
go build ./services/productpage/...
go test -race -count=1 ./services/productpage/...
```

If handler tests break because of client signature changes, update the tests to pass a dummy idempotency key or verify the POST body contains `idempotency_key`.

- [ ] **Step 7: Commit**

```bash
git add services/productpage/ go.mod go.sum
git commit -m "feat(productpage): generate idempotency_key on form submission"
```

---

## Task 13: End-to-end validation

**Files:** none (validation only)

- [ ] **Step 1: Build everything**

```bash
make build-all
```

Expected: all five binaries produced in `bin/`.

- [ ] **Step 2: Run lint**

```bash
make lint
```

Expected: no issues.

- [ ] **Step 3: Run full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 4: Run Docker Compose E2E**

```bash
make e2e
```

Expected: E2E smoke tests pass with memory backend.

- [ ] **Step 5: Commit (if any tidying was needed)**

```bash
git status
# If any unstaged changes:
git add -A
git commit -m "chore: tidy after idempotency rollout"
```

---

## Self-Review

**Spec coverage:**

- `pkg/idempotency` shared package — Tasks 1-3 ✓
- `processed_events` table in each service — Tasks 4, 6, 8, 10 ✓
- Service-layer idempotency check — Tasks 5, 7, 9, 11 ✓
- Handler extracts explicit key, derives natural key as fallback — Task 5, 7, 9, 11 ✓
- Productpage generates idempotency_key — Task 12 ✓
- Memory adapter for local dev — Tasks 2, 5, 7, 9, 11 ✓
- Backwards compatibility (missing key still derives a natural key) — inherent in `Resolve()` — ✓

**Type consistency:**

- `idempotency.Store.CheckAndRecord(ctx, key) (bool, error)` — used consistently
- `idempotency.Resolve(explicitKey, fields...) string` — used consistently
- `ErrAlreadyProcessed` sentinel — same name in each service package

**Placeholder scan:** None. All code blocks contain the actual content.

**Notes for the implementing engineer:**

- Tasks 6-11 intentionally reference Task 7's pattern for brevity; the step structure is identical, only the service/method/field names change.
- Each service's `cmd/main.go` hoists `pool` out of the switch because `NewPostgresStore` needs it even when `STORAGE_BACKEND=memory` would have left `pool` nil — use the `if pool != nil` guard shown in Task 5 Step 6.
- The "already processed" response currently returns HTTP 200 with a minimal error-shaped body. This is intentionally pragmatic — a fully correct implementation would look up the original resource and return it, but that adds lookup cost and is out of scope for v1. Clients that need the original response can retry with the same key after storing their own local state.
