# PostgreSQL Storage Adapters — Design Spec

> Phase 7 of the [bookinfo monorepo design](2026-04-07-bookinfo-monorepo-design.md)

## Context

All 4 backend services (details, reviews, ratings, notification) currently use in-memory adapters behind hexagonal outbound port interfaces. The `STORAGE_BACKEND` env var and `DATABASE_URL` validation already exist in `pkg/config` but no postgres adapters or wiring exist yet.

This spec adds the PostgreSQL storage layer: adapters, migrations, shared database package, composition root wiring, and e2e testing infrastructure.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| PostgreSQL driver | `jackc/pgx/v5` with `pgxpool` | Pure Go, high-performance, community standard. No need for `database/sql` abstraction since adapters are behind port interfaces |
| Migration tool | `golang-migrate/migrate/v4` | Spec-specified, most popular Go migration library, supports embedded SQL via `iofs` source driver |
| Migration file location | `services/<name>/migrations/` per service | Aligns with hex arch boundary — each service owns its schema. Independent evolution |
| Migration execution | Startup by default (`RUN_MIGRATIONS=true`), disableable for production | Convenient for dev/e2e, production runs migrations separately via CI/CD Job |
| Database isolation | Separate database per service | Strongest isolation, true data ownership. `bookinfo_ratings`, `bookinfo_details`, `bookinfo_reviews`, `bookinfo_notification` |
| ID strategy | Application-generated UUIDs (TEXT columns) | Domain types already generate UUIDs via `google/uuid`. No Postgres-side generation |
| Query style | Hand-written SQL via pgx | Simple CRUD (2-3 queries per service). Code generation (sqlc) would be overkill |

---

## New Dependencies

| Package | Purpose |
|---|---|
| `github.com/jackc/pgx/v5` | PostgreSQL driver + `pgxpool` connection pool |
| `github.com/golang-migrate/migrate/v4` | Schema migration runner |
| `github.com/golang-migrate/migrate/v4/database/pgx/v5` | pgx v5 driver for migrate |
| `github.com/golang-migrate/migrate/v4/source/iofs` | Reads embedded migration files via `io/fs` |

---

## Shared Database Package (`pkg/database`)

New package with three files:

### `pkg/database/pool.go`

- `NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)` — creates a connection pool from `DATABASE_URL` with sensible defaults
- Returns the pool for injection into postgres adapters

### `pkg/database/migrate.go`

- `RunMigrations(databaseURL string, migrations fs.FS) error` — runs pending up-migrations using `golang-migrate`
- Uses `iofs` source driver (reads from `embed.FS`) + pgx v5 database driver
- Each service passes its own embedded `migrations.FS`

### `pkg/database/health.go`

- `HealthCheck(pool *pgxpool.Pool) func() error` — returns a check function for `pkg/health.ReadinessHandler`
- Calls `pool.Ping(ctx)` with a short timeout

---

## Database Schemas

Each service gets its own database and migration files.

### ratings — database `bookinfo_ratings`

```sql
-- services/ratings/migrations/001_create_ratings.up.sql
CREATE TABLE IF NOT EXISTS ratings (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    stars      INTEGER NOT NULL CHECK (stars >= 1 AND stars <= 5)
);
CREATE INDEX idx_ratings_product_id ON ratings (product_id);

-- services/ratings/migrations/001_create_ratings.down.sql
DROP INDEX IF EXISTS idx_ratings_product_id;
DROP TABLE IF EXISTS ratings;
```

### details — database `bookinfo_details`

```sql
-- services/details/migrations/001_create_details.up.sql
CREATE TABLE IF NOT EXISTS details (
    id        TEXT PRIMARY KEY,
    title     TEXT NOT NULL,
    author    TEXT NOT NULL,
    year      INTEGER NOT NULL,
    type      TEXT NOT NULL DEFAULT '',
    pages     INTEGER NOT NULL,
    publisher TEXT NOT NULL DEFAULT '',
    language  TEXT NOT NULL DEFAULT '',
    isbn10    TEXT NOT NULL DEFAULT '',
    isbn13    TEXT NOT NULL DEFAULT ''
);

-- services/details/migrations/001_create_details.down.sql
DROP TABLE IF EXISTS details;
```

### reviews — database `bookinfo_reviews`

```sql
-- services/reviews/migrations/001_create_reviews.up.sql
CREATE TABLE IF NOT EXISTS reviews (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    text       TEXT NOT NULL
);
CREATE INDEX idx_reviews_product_id ON reviews (product_id);

-- services/reviews/migrations/001_create_reviews.down.sql
DROP INDEX IF EXISTS idx_reviews_product_id;
DROP TABLE IF EXISTS reviews;
```

### notification — database `bookinfo_notification`

```sql
-- services/notification/migrations/001_create_notifications.up.sql
CREATE TABLE IF NOT EXISTS notifications (
    id        TEXT PRIMARY KEY,
    recipient TEXT NOT NULL,
    channel   TEXT NOT NULL,
    subject   TEXT NOT NULL,
    body      TEXT NOT NULL,
    status    TEXT NOT NULL DEFAULT 'queued',
    sent_at   TIMESTAMPTZ
);
CREATE INDEX idx_notifications_recipient ON notifications (recipient);

-- services/notification/migrations/001_create_notifications.down.sql
DROP INDEX IF EXISTS idx_notifications_recipient;
DROP TABLE IF EXISTS notifications;
```

### Embedded migrations

Each service gets `services/<name>/migrations/embed.go`:

```go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

---

## Postgres Adapter Implementations

Each adapter lives at `services/<name>/internal/adapter/outbound/postgres/<entity>_repository.go`, implements the existing outbound port interface, and receives `*pgxpool.Pool` via constructor.

### Query Mapping

| Service | Method | SQL |
|---|---|---|
| **ratings** | `FindByProductID(ctx, productID)` | `SELECT id, product_id, reviewer, stars FROM ratings WHERE product_id = $1` |
| **ratings** | `Save(ctx, rating)` | `INSERT INTO ratings (id, product_id, reviewer, stars) VALUES ($1, $2, $3, $4)` |
| **details** | `FindByID(ctx, id)` | `SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details WHERE id = $1` |
| **details** | `Save(ctx, detail)` | `INSERT INTO details (id, title, author, year, type, pages, publisher, language, isbn10, isbn13) VALUES ($1, ..., $10)` |
| **details** | `FindAll(ctx)` | `SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details` |
| **reviews** | `FindByProductID(ctx, productID)` | `SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1` |
| **reviews** | `Save(ctx, review)` | `INSERT INTO reviews (id, product_id, reviewer, text) VALUES ($1, $2, $3, $4)` |
| **notification** | `Save(ctx, notification)` | `INSERT INTO notifications (id, recipient, channel, subject, body, status, sent_at) VALUES ($1, ..., $7)` |
| **notification** | `FindByID(ctx, id)` | `SELECT id, recipient, channel, subject, body, status, sent_at FROM notifications WHERE id = $1` |
| **notification** | `FindByRecipient(ctx, recipient)` | `SELECT ... FROM notifications WHERE recipient = $1` |

### Error Handling

- `pgx.ErrNoRows` on `FindByID` → return `nil, nil` (matches memory adapter behavior; callers already handle nil)
- All other errors wrapped with context: `fmt.Errorf("finding rating by product %s: %w", productID, err)`
- `Save` uses plain `INSERT` — no upsert, since IDs are always new UUIDs

---

## Composition Root Wiring

Each service's `cmd/main.go` gets a switch on `cfg.StorageBackend`:

```go
var repo port.RatingRepository
switch cfg.StorageBackend {
case "postgres":
    pool, err := database.NewPool(ctx, cfg.DatabaseURL)
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
    }

    repo = postgres.NewRatingRepository(pool)
    // Register database.HealthCheck(pool) in readiness probe
default:
    repo = memory.NewRatingRepository()
}
```

### Config Change

Add to `pkg/config.Config`:

```go
RunMigrations bool // RUN_MIGRATIONS (default "true")
```

Loaded as `envOrDefault("RUN_MIGRATIONS", "true") == "true"`.

### Health Check Wiring

When using postgres, pass `database.HealthCheck(pool)` to `health.ReadinessHandler` so `/readyz` reports database connectivity. Memory backend has no extra checks (current behavior).

---

## Docker Compose & E2E Testing

### `test/e2e/docker-compose.postgres.yml`

Override file that adds Postgres and reconfigures services:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: bookinfo
      POSTGRES_PASSWORD: bookinfo
    volumes:
      - ./init-databases.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-ONLY", "pg_isready", "-U", "bookinfo"]
      interval: 2s
      timeout: 5s
      retries: 5

  ratings:
    environment:
      STORAGE_BACKEND: postgres
      DATABASE_URL: postgres://bookinfo:bookinfo@postgres:5432/bookinfo_ratings?sslmode=disable
      RUN_MIGRATIONS: "true"
    depends_on:
      postgres:
        condition: service_healthy

  details:
    environment:
      STORAGE_BACKEND: postgres
      DATABASE_URL: postgres://bookinfo:bookinfo@postgres:5432/bookinfo_details?sslmode=disable
      RUN_MIGRATIONS: "true"
    depends_on:
      postgres:
        condition: service_healthy

  reviews:
    environment:
      STORAGE_BACKEND: postgres
      DATABASE_URL: postgres://bookinfo:bookinfo@postgres:5432/bookinfo_reviews?sslmode=disable
      RUN_MIGRATIONS: "true"
    depends_on:
      postgres:
        condition: service_healthy

  notification:
    environment:
      STORAGE_BACKEND: postgres
      DATABASE_URL: postgres://bookinfo:bookinfo@postgres:5432/bookinfo_notification?sslmode=disable
      RUN_MIGRATIONS: "true"
    depends_on:
      postgres:
        condition: service_healthy
```

### `test/e2e/init-databases.sql`

```sql
CREATE DATABASE bookinfo_ratings;
CREATE DATABASE bookinfo_details;
CREATE DATABASE bookinfo_reviews;
CREATE DATABASE bookinfo_notification;
```

### Makefile Targets

- `make e2e-postgres` — runs `docker compose -f docker-compose.yml -f docker-compose.postgres.yml up` + existing e2e test scripts
- Existing `make e2e` remains unchanged (memory backend)

E2e test scripts are reused as-is — they test the same HTTP API regardless of backend.

---

## New Files Summary

```
pkg/database/
├── pool.go                    # pgxpool.Pool creation
├── migrate.go                 # golang-migrate runner with embed.FS
└── health.go                  # pool.Ping health check

services/ratings/migrations/
├── embed.go                   # //go:embed *.sql
├── 001_create_ratings.up.sql
└── 001_create_ratings.down.sql

services/details/migrations/
├── embed.go
├── 001_create_details.up.sql
└── 001_create_details.down.sql

services/reviews/migrations/
├── embed.go
├── 001_create_reviews.up.sql
└── 001_create_reviews.down.sql

services/notification/migrations/
├── embed.go
├── 001_create_notifications.up.sql
└── 001_create_notifications.down.sql

services/ratings/internal/adapter/outbound/postgres/rating_repository.go
services/details/internal/adapter/outbound/postgres/detail_repository.go
services/reviews/internal/adapter/outbound/postgres/review_repository.go
services/notification/internal/adapter/outbound/postgres/notification_repository.go

test/e2e/docker-compose.postgres.yml
test/e2e/init-databases.sql
```

## Modified Files Summary

```
pkg/config/config.go                  # Add RunMigrations field
services/ratings/cmd/main.go          # Storage backend switch
services/details/cmd/main.go          # Storage backend switch
services/reviews/cmd/main.go          # Storage backend switch
services/notification/cmd/main.go     # Storage backend switch
go.mod / go.sum                       # New dependencies (pgx, golang-migrate)
Makefile                              # Add e2e-postgres target
```

---

## Out of Scope

- Connection pool tuning — use pgxpool defaults; tune later based on load testing
- Pagination/filtering — current port interfaces are simple CRUD, no changes
- Database-level idempotency (unique constraints on business keys) — separate concern
- Testcontainers for adapter unit tests — e2e docker-compose covers integration; unit tests use memory adapter
- Connection retry/backoff on startup — pgxpool handles reconnection; fail fast if unreachable
- Read replicas or connection routing — single writer, single pool per service
