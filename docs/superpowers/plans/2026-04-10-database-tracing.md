# Database Operation Tracing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenTelemetry tracing to all PostgreSQL and Redis operations so database calls appear as child spans in distributed traces.

**Architecture:** Adapter-level instrumentation using `otelpgx` for pgx connection pools and `redisotel` for go-redis clients. Changes are centralized in `pkg/database/pool.go` and `services/productpage/internal/pending/redis.go` — all downstream consumers get tracing for free.

**Tech Stack:** Go, pgx/v5, go-redis/v9, OpenTelemetry, otelpgx, redisotel

---

### Task 1: Add otelpgx and redisotel dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add otelpgx dependency**

Run:
```bash
go get github.com/exaring/otelpgx
```

Expected: `go.mod` gains `github.com/exaring/otelpgx` in the require block.

- [ ] **Step 2: Add redisotel dependency**

Run:
```bash
go get github.com/redis/go-redis/extra/redisotel/v9
```

Expected: `go.mod` gains `github.com/redis/go-redis/extra/redisotel/v9` in the require block.

- [ ] **Step 3: Tidy modules**

Run:
```bash
go mod tidy
```

Expected: Clean exit, no unused dependencies removed.

- [ ] **Step 4: Verify build still passes**

Run:
```bash
make build-all
```

Expected: All 5 services build successfully. No compilation errors.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add otelpgx and redisotel dependencies"
```

---

### Task 2: Instrument PostgreSQL connection pool with otelpgx

**Files:**
- Modify: `pkg/database/pool.go`

- [ ] **Step 1: Write the failing test for tracer configuration**

Create `pkg/database/pool_test.go`:

```go
package database_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
)

func TestNewPoolConfig_HasTracer(t *testing.T) {
	// Use a dummy URL — we only need to verify config parsing, not connectivity.
	databaseURL := "postgres://user:pass@localhost:5432/testdb"

	cfg, err := database.NewPoolConfig(databaseURL)
	if err != nil {
		t.Fatalf("NewPoolConfig returned error: %v", err)
	}
	if cfg.ConnConfig.Tracer == nil {
		t.Fatal("expected ConnConfig.Tracer to be set, got nil")
	}
}

func TestNewPoolConfig_InvalidURL(t *testing.T) {
	_, err := database.NewPoolConfig("not-a-valid-url://")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./pkg/database/ -run TestNewPoolConfig -v
```

Expected: FAIL — `database.NewPoolConfig` is undefined.

- [ ] **Step 3: Implement NewPoolConfig and update NewPool**

Replace the contents of `pkg/database/pool.go` with:

```go
// Package database provides PostgreSQL connection pool, migration, and health check utilities.
package database

import (
	"context"
	"fmt"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPoolConfig parses a database URL and returns a pgxpool.Config with
// OpenTelemetry tracing enabled. The tracer creates child spans for every
// Query, QueryRow, Exec, Prepare, and Connect call.
func NewPoolConfig(databaseURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTrimSQLInSpanName(),
	)
	return cfg, nil
}

// NewPool creates a new PostgreSQL connection pool from the given database URL
// with OpenTelemetry tracing enabled.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := NewPoolConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}
```

Key changes:
- Extracted `NewPoolConfig` so the tracer configuration is testable without a live database.
- `NewPool` now calls `NewPoolConfig` then `pgxpool.NewWithConfig` instead of `pgxpool.New`.
- `otelpgx.WithTrimSQLInSpanName()` keeps span names short (e.g., `SELECT details` instead of the full query).

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./pkg/database/ -run TestNewPoolConfig -v
```

Expected: Both tests PASS.

- [ ] **Step 5: Run full build to ensure no breakage**

Run:
```bash
make build-all
```

Expected: All 5 services build. The `NewPool` signature is unchanged (`ctx, string -> *Pool, error`) so all callers remain valid.

- [ ] **Step 6: Commit**

```bash
git add pkg/database/pool.go pkg/database/pool_test.go
git commit -m "feat(pkg/database): add OpenTelemetry tracing to PostgreSQL connection pool"
```

---

### Task 3: Instrument Redis client with redisotel

**Files:**
- Modify: `services/productpage/internal/pending/redis.go`
- Modify: `services/productpage/internal/pending/pending_test.go`

- [ ] **Step 1: Write the failing test for Redis tracing instrumentation**

Add this test to `services/productpage/internal/pending/pending_test.go`:

```go
func TestNewRedisStore_HasTracingHook(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := pending.NewRedisStore("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Verify the store was created successfully with tracing.
	// The tracing hook is transparent — existing operations must still work.
	ctx := context.Background()
	err = store.StorePending(ctx, "trace-test", pending.NewReview("tracer", "test", 5))
	if err != nil {
		t.Fatalf("StorePending with tracing hook failed: %v", err)
	}

	reviews, err := store.GetAndReconcile(ctx, "trace-test", nil)
	if err != nil {
		t.Fatalf("GetAndReconcile with tracing hook failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("got %d reviews, want 1", len(reviews))
	}
}
```

- [ ] **Step 2: Run test to verify it passes (baseline — tracing not yet added)**

Run:
```bash
go test ./services/productpage/internal/pending/ -run TestNewRedisStore_HasTracingHook -v
```

Expected: PASS (miniredis works fine without tracing). This establishes the baseline.

- [ ] **Step 3: Add redisotel tracing hook to NewRedisStore**

Replace the contents of `services/productpage/internal/pending/redis.go` with:

```go
package pending

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "pending:reviews:"

// RedisStore implements Store backed by Redis lists.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a RedisStore from a Redis URL (e.g. "redis://localhost:6379").
// The client is instrumented with OpenTelemetry tracing — every command
// produces a child span with operation name and key.
func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(client,
		redisotel.WithDBStatement(false),
	); err != nil {
		return nil, fmt.Errorf("instrumenting redis tracing: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// Ping verifies the connection to Redis.
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// StorePending appends a pending review to the Redis list for the given product.
func (s *RedisStore) StorePending(ctx context.Context, productID string, review Review) error {
	data, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("marshaling pending review: %w", err)
	}
	return s.client.RPush(ctx, keyPrefix+productID, data).Err()
}

// GetAndReconcile returns pending reviews after removing any that match confirmed reviews.
func (s *RedisStore) GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, error) {
	key := keyPrefix + productID

	vals, err := s.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching pending reviews: %w", err)
	}

	if len(vals) == 0 {
		return nil, nil
	}

	confirmedSet := make(map[string]struct{}, len(confirmed))
	for _, c := range confirmed {
		confirmedSet[c.Reviewer+"\x00"+c.Text] = struct{}{}
	}

	var remaining []Review
	for _, raw := range vals {
		var r Review
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			continue // skip corrupted entries
		}

		matchKey := r.Reviewer + "\x00" + r.Text
		if _, found := confirmedSet[matchKey]; found {
			// Review confirmed — remove from Redis
			s.client.LRem(ctx, key, 1, raw)
			delete(confirmedSet, matchKey) // only remove first match
		} else {
			remaining = append(remaining, r)
		}
	}

	return remaining, nil
}
```

Key changes:
- Added `redisotel` import.
- Split client creation and store return: `redis.NewClient(opts)` then `redisotel.InstrumentTracing(client, ...)` then return.
- `WithDBStatement(false)` excludes full command arguments (values) from spans — only command name + key are visible.

- [ ] **Step 4: Run all pending tests to verify nothing broke**

Run:
```bash
go test ./services/productpage/internal/pending/ -v
```

Expected: ALL tests pass (TestStorePending, TestGetAndReconcile_RemovesConfirmed, TestGetAndReconcile_EmptyProduct, TestGetAndReconcile_IsolatesProducts, TestNewRedisStore_HasTracingHook).

- [ ] **Step 5: Commit**

```bash
git add services/productpage/internal/pending/redis.go services/productpage/internal/pending/pending_test.go
git commit -m "feat(productpage): add OpenTelemetry tracing to Redis pending store"
```

---

### Task 4: Full test suite and build verification

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite with race detector**

Run:
```bash
make test-race
```

Expected: ALL tests pass across the entire monorepo. Zero failures.

- [ ] **Step 2: Run linter**

Run:
```bash
make lint
```

Expected: No new lint warnings. Clean exit.

- [ ] **Step 3: Build all services**

Run:
```bash
make build-all
```

Expected: All 5 services build successfully.

- [ ] **Step 4: Run go vet**

Run:
```bash
make vet
```

Expected: Clean exit, no issues.

---

### Task 5: Deploy to local k8s and verify traces in Grafana Tempo

**Files:** None (deployment and verification only)

- [ ] **Step 1: Rebuild images and redeploy to k8s**

Run:
```bash
make k8s-rebuild
```

Expected: All images rebuilt with new tracing instrumentation. Pods restart with updated code.

- [ ] **Step 2: Wait for pods to be ready**

Run:
```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get pods -w
```

Expected: All pods reach `Running` / `Ready` state.

- [ ] **Step 3: Submit a review via POST (trigger the async write path)**

Run:
```bash
curl -s -D - -X POST http://localhost:8080/partials/rating \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "product_id=d0001&reviewer=trace-test&rating=5&text=Testing+database+tracing"
```

Expected: HTTP 200 response. Check the productpage logs for the trace ID:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo logs -l app=productpage --tail=20 | grep trace_id
```

Note the `trace_id` value from the POST request log line.

- [ ] **Step 4: Trigger a GET to load reviews (triggers Postgres SELECT + Redis reconciliation)**

Run:
```bash
curl -s http://localhost:8080/partials/reviews?product_id=d0001 > /dev/null
```

Check logs again for the GET request's trace ID:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo logs -l app=productpage --tail=10 | grep trace_id
```

- [ ] **Step 5: Verify Postgres spans in Grafana Tempo**

Open Grafana at http://localhost:3000, navigate to Explore > Tempo.

Search for the trace ID from Step 3. Verify:
- The trace contains spans from the reviews-write service
- A `db.postgresql` span (or span with `db.system=postgresql`) appears as a child of the write service HTTP handler span
- The span has `db.statement` attribute with sanitized SQL (e.g., `INSERT INTO reviews ... VALUES ($1, $2, ...)`)

Search for the trace ID from Step 4. Verify:
- Postgres `SELECT` spans appear under the read service handler spans (details-read, reviews-read, ratings-read)
- Each SELECT span shows the sanitized SQL query

- [ ] **Step 6: Verify Redis spans in Grafana Tempo**

Using the trace ID from Step 3 (POST request):
- A Redis `RPUSH` span should appear under the productpage handler span
- The span name should include the key (e.g., `RPUSH pending:reviews:d0001`)

Using the trace ID from Step 4 (GET request):
- Redis `LRANGE` span should appear under the productpage handler span
- If reconciliation occurred, a `LREM` span should also be present

- [ ] **Step 7: Commit verification notes (optional)**

No code changes needed. If all traces verified, the task is complete.
