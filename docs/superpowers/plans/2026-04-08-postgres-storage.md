# PostgreSQL Storage Adapters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PostgreSQL storage adapters to all 4 backend services, selectable via `STORAGE_BACKEND=postgres` env var.

**Architecture:** Each service gets a postgres adapter implementing its existing outbound port interface, backed by `pgxpool.Pool`. A shared `pkg/database` package handles pool creation, migration execution (via `golang-migrate` with embedded SQL), and health checks. Composition roots switch between memory and postgres adapters based on config.

**Tech Stack:** `jackc/pgx/v5` (driver + pool), `golang-migrate/migrate/v4` (schema migrations), `embed` (bundled SQL files), PostgreSQL 17

---

### Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add pgx and golang-migrate dependencies**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
go get github.com/jackc/pgx/v5
go get github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/pgx/v5
go get github.com/golang-migrate/migrate/v4/source/iofs
```

- [ ] **Step 2: Tidy modules**

```bash
go mod tidy
```

- [ ] **Step 3: Verify build still passes**

Run: `go build ./...`
Expected: clean build, no errors

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add pgx and golang-migrate dependencies"
```

---

### Task 2: Add `RunMigrations` to Config

**Files:**
- Modify: `pkg/config/config.go`

- [ ] **Step 1: Add RunMigrations field to Config struct**

In `pkg/config/config.go`, add the `RunMigrations` field to the `Config` struct and load it in `Load()`:

```go
// In the Config struct, after DatabaseURL:
RunMigrations bool

// In Load(), after the DatabaseURL line:
RunMigrations: envOrDefault("RUN_MIGRATIONS", "true") == "true",
```

The full updated `Config` struct becomes:

```go
type Config struct {
	ServiceName            string
	HTTPPort               string
	AdminPort              string
	LogLevel               string
	StorageBackend         string
	DatabaseURL            string
	RunMigrations          bool
	OTLPEndpoint           string
	PyroscopeServerAddress string
}
```

And in `Load()`, the `cfg` initialization becomes:

```go
cfg := &Config{
	ServiceName:            os.Getenv("SERVICE_NAME"),
	HTTPPort:               envOrDefault("HTTP_PORT", "8080"),
	AdminPort:              envOrDefault("ADMIN_PORT", "9090"),
	LogLevel:               envOrDefault("LOG_LEVEL", "info"),
	StorageBackend:         envOrDefault("STORAGE_BACKEND", "memory"),
	DatabaseURL:            os.Getenv("DATABASE_URL"),
	RunMigrations:          envOrDefault("RUN_MIGRATIONS", "true") == "true",
	OTLPEndpoint:           os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	PyroscopeServerAddress: os.Getenv("PYROSCOPE_SERVER_ADDRESS"),
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean build

- [ ] **Step 3: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(pkg/config): add RunMigrations config field"
```

---

### Task 3: Create `pkg/database` — Pool, Migrate, Health

**Files:**
- Create: `pkg/database/pool.go`
- Create: `pkg/database/migrate.go`
- Create: `pkg/database/health.go`

- [ ] **Step 1: Create `pkg/database/pool.go`**

```go
// Package database provides PostgreSQL connection pool, migration, and health check utilities.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new PostgreSQL connection pool from the given database URL.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
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

- [ ] **Step 2: Create `pkg/database/migrate.go`**

```go
package database

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations runs all pending up-migrations using the given embedded filesystem.
// The databaseURL must be a valid PostgreSQL connection string.
// The migrations fs.FS should contain .up.sql and .down.sql files.
func RunMigrations(databaseURL string, migrations fs.FS) error {
	source, err := iofs.New(migrations, ".")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, convertToPgxURL(databaseURL))
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// convertToPgxURL converts a standard postgres:// URL to the pgx5:// scheme
// required by the golang-migrate pgx/v5 driver.
func convertToPgxURL(databaseURL string) string {
	if len(databaseURL) >= 11 && databaseURL[:11] == "postgres://" {
		return "pgx5://" + databaseURL[11:]
	}
	if len(databaseURL) >= 14 && databaseURL[:14] == "postgresql://" {
		return "pgx5://" + databaseURL[14:]
	}
	return databaseURL
}
```

- [ ] **Step 3: Create `pkg/database/health.go`**

```go
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthCheck returns a function that checks database connectivity.
// Compatible with pkg/health.ReadinessHandler check functions.
func HealthCheck(pool *pgxpool.Pool) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("database ping failed: %w", err)
		}
		return nil
	}
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./pkg/database/...`
Expected: clean build

- [ ] **Step 5: Commit**

```bash
git add pkg/database/
git commit -m "feat(pkg/database): add pool, migration runner, and health check"
```

---

### Task 4: Ratings — Migrations and Postgres Adapter

**Files:**
- Create: `services/ratings/migrations/embed.go`
- Create: `services/ratings/migrations/001_create_ratings.up.sql`
- Create: `services/ratings/migrations/001_create_ratings.down.sql`
- Create: `services/ratings/internal/adapter/outbound/postgres/rating_repository.go`

- [ ] **Step 1: Create migration embed file**

Create `services/ratings/migrations/embed.go`:

```go
// Package migrations provides embedded SQL migration files for the ratings service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Create up migration**

Create `services/ratings/migrations/001_create_ratings.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS ratings (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    stars      INTEGER NOT NULL CHECK (stars >= 1 AND stars <= 5)
);

CREATE INDEX idx_ratings_product_id ON ratings (product_id);
```

- [ ] **Step 3: Create down migration**

Create `services/ratings/migrations/001_create_ratings.down.sql`:

```sql
DROP INDEX IF EXISTS idx_ratings_product_id;
DROP TABLE IF EXISTS ratings;
```

- [ ] **Step 4: Create postgres adapter**

Create `services/ratings/internal/adapter/outbound/postgres/rating_repository.go`:

```go
// Package postgres provides a PostgreSQL implementation of the ratings repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository is a PostgreSQL implementation of port.RatingRepository.
type RatingRepository struct {
	pool *pgxpool.Pool
}

// NewRatingRepository creates a new PostgreSQL rating repository.
func NewRatingRepository(pool *pgxpool.Pool) *RatingRepository {
	return &RatingRepository{pool: pool}
}

// FindByProductID returns all ratings for the given product ID.
func (r *RatingRepository) FindByProductID(ctx context.Context, productID string) ([]domain.Rating, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, stars FROM ratings WHERE product_id = $1",
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying ratings for product %s: %w", productID, err)
	}
	defer rows.Close()

	var ratings []domain.Rating
	for rows.Next() {
		var rating domain.Rating
		if err := rows.Scan(&rating.ID, &rating.ProductID, &rating.Reviewer, &rating.Stars); err != nil {
			return nil, fmt.Errorf("scanning rating row: %w", err)
		}
		ratings = append(ratings, rating)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rating rows: %w", err)
	}

	return ratings, nil
}

// Save persists a rating in PostgreSQL.
func (r *RatingRepository) Save(ctx context.Context, rating *domain.Rating) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO ratings (id, product_id, reviewer, stars) VALUES ($1, $2, $3, $4)",
		rating.ID, rating.ProductID, rating.Reviewer, rating.Stars,
	)
	if err != nil {
		return fmt.Errorf("inserting rating %s: %w", rating.ID, err)
	}

	return nil
}
```

- [ ] **Step 5: Verify build**

Run: `go build ./services/ratings/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add services/ratings/migrations/ services/ratings/internal/adapter/outbound/postgres/
git commit -m "feat(ratings): add PostgreSQL adapter and migrations"
```

---

### Task 5: Details — Migrations and Postgres Adapter

**Files:**
- Create: `services/details/migrations/embed.go`
- Create: `services/details/migrations/001_create_details.up.sql`
- Create: `services/details/migrations/001_create_details.down.sql`
- Create: `services/details/internal/adapter/outbound/postgres/detail_repository.go`

- [ ] **Step 1: Create migration embed file**

Create `services/details/migrations/embed.go`:

```go
// Package migrations provides embedded SQL migration files for the details service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Create up migration**

Create `services/details/migrations/001_create_details.up.sql`:

```sql
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
```

- [ ] **Step 3: Create down migration**

Create `services/details/migrations/001_create_details.down.sql`:

```sql
DROP TABLE IF EXISTS details;
```

- [ ] **Step 4: Create postgres adapter**

Create `services/details/internal/adapter/outbound/postgres/detail_repository.go`:

```go
// Package postgres provides a PostgreSQL implementation of the details repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository is a PostgreSQL implementation of port.DetailRepository.
type DetailRepository struct {
	pool *pgxpool.Pool
}

// NewDetailRepository creates a new PostgreSQL detail repository.
func NewDetailRepository(pool *pgxpool.Pool) *DetailRepository {
	return &DetailRepository{pool: pool}
}

// FindByID returns a detail by its ID. Returns nil, nil if not found.
func (r *DetailRepository) FindByID(ctx context.Context, id string) (*domain.Detail, error) {
	var d domain.Detail
	err := r.pool.QueryRow(ctx,
		"SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details WHERE id = $1",
		id,
	).Scan(&d.ID, &d.Title, &d.Author, &d.Year, &d.Type, &d.Pages, &d.Publisher, &d.Language, &d.ISBN10, &d.ISBN13)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying detail %s: %w", id, err)
	}

	return &d, nil
}

// Save persists a detail in PostgreSQL.
func (r *DetailRepository) Save(ctx context.Context, detail *domain.Detail) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO details (id, title, author, year, type, pages, publisher, language, isbn10, isbn13) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
		detail.ID, detail.Title, detail.Author, detail.Year, detail.Type, detail.Pages, detail.Publisher, detail.Language, detail.ISBN10, detail.ISBN13,
	)
	if err != nil {
		return fmt.Errorf("inserting detail %s: %w", detail.ID, err)
	}

	return nil
}

// FindAll returns all stored details.
func (r *DetailRepository) FindAll(ctx context.Context) ([]*domain.Detail, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details ORDER BY title",
	)
	if err != nil {
		return nil, fmt.Errorf("querying all details: %w", err)
	}
	defer rows.Close()

	var details []*domain.Detail
	for rows.Next() {
		var d domain.Detail
		if err := rows.Scan(&d.ID, &d.Title, &d.Author, &d.Year, &d.Type, &d.Pages, &d.Publisher, &d.Language, &d.ISBN10, &d.ISBN13); err != nil {
			return nil, fmt.Errorf("scanning detail row: %w", err)
		}
		details = append(details, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating detail rows: %w", err)
	}

	return details, nil
}
```

Note: `FindAll` uses `ORDER BY title` to match the memory adapter's `sort.Slice` behavior that sorts by title.

- [ ] **Step 5: Verify build**

Run: `go build ./services/details/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add services/details/migrations/ services/details/internal/adapter/outbound/postgres/
git commit -m "feat(details): add PostgreSQL adapter and migrations"
```

---

### Task 6: Reviews — Migrations and Postgres Adapter

**Files:**
- Create: `services/reviews/migrations/embed.go`
- Create: `services/reviews/migrations/001_create_reviews.up.sql`
- Create: `services/reviews/migrations/001_create_reviews.down.sql`
- Create: `services/reviews/internal/adapter/outbound/postgres/review_repository.go`

- [ ] **Step 1: Create migration embed file**

Create `services/reviews/migrations/embed.go`:

```go
// Package migrations provides embedded SQL migration files for the reviews service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Create up migration**

Create `services/reviews/migrations/001_create_reviews.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS reviews (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    reviewer   TEXT NOT NULL,
    text       TEXT NOT NULL
);

CREATE INDEX idx_reviews_product_id ON reviews (product_id);
```

- [ ] **Step 3: Create down migration**

Create `services/reviews/migrations/001_create_reviews.down.sql`:

```sql
DROP INDEX IF EXISTS idx_reviews_product_id;
DROP TABLE IF EXISTS reviews;
```

- [ ] **Step 4: Create postgres adapter**

Create `services/reviews/internal/adapter/outbound/postgres/review_repository.go`:

```go
// Package postgres provides a PostgreSQL implementation of the reviews repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository is a PostgreSQL implementation of port.ReviewRepository.
type ReviewRepository struct {
	pool *pgxpool.Pool
}

// NewReviewRepository creates a new PostgreSQL review repository.
func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

// FindByProductID returns all reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(ctx context.Context, productID string) ([]domain.Review, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1",
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying reviews for product %s: %w", productID, err)
	}
	defer rows.Close()

	var reviews []domain.Review
	for rows.Next() {
		var review domain.Review
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text); err != nil {
			return nil, fmt.Errorf("scanning review row: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating review rows: %w", err)
	}

	return reviews, nil
}

// Save persists a review in PostgreSQL.
func (r *ReviewRepository) Save(ctx context.Context, review *domain.Review) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reviews (id, product_id, reviewer, text) VALUES ($1, $2, $3, $4)",
		review.ID, review.ProductID, review.Reviewer, review.Text,
	)
	if err != nil {
		return fmt.Errorf("inserting review %s: %w", review.ID, err)
	}

	return nil
}
```

- [ ] **Step 5: Verify build**

Run: `go build ./services/reviews/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add services/reviews/migrations/ services/reviews/internal/adapter/outbound/postgres/
git commit -m "feat(reviews): add PostgreSQL adapter and migrations"
```

---

### Task 7: Notification — Migrations and Postgres Adapter

**Files:**
- Create: `services/notification/migrations/embed.go`
- Create: `services/notification/migrations/001_create_notifications.up.sql`
- Create: `services/notification/migrations/001_create_notifications.down.sql`
- Create: `services/notification/internal/adapter/outbound/postgres/notification_repository.go`

- [ ] **Step 1: Create migration embed file**

Create `services/notification/migrations/embed.go`:

```go
// Package migrations provides embedded SQL migration files for the notification service.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 2: Create up migration**

Create `services/notification/migrations/001_create_notifications.up.sql`:

```sql
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
```

- [ ] **Step 3: Create down migration**

Create `services/notification/migrations/001_create_notifications.down.sql`:

```sql
DROP INDEX IF EXISTS idx_notifications_recipient;
DROP TABLE IF EXISTS notifications;
```

- [ ] **Step 4: Create postgres adapter**

Create `services/notification/internal/adapter/outbound/postgres/notification_repository.go`:

```go
// Package postgres provides a PostgreSQL implementation of the notification repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/domain"
)

// NotificationRepository is a PostgreSQL implementation of port.NotificationRepository.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository creates a new PostgreSQL notification repository.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// Save persists a notification in PostgreSQL.
func (r *NotificationRepository) Save(ctx context.Context, notification *domain.Notification) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO notifications (id, recipient, channel, subject, body, status, sent_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		notification.ID, notification.Recipient, string(notification.Channel), notification.Subject, notification.Body, string(notification.Status), notification.SentAt,
	)
	if err != nil {
		return fmt.Errorf("inserting notification %s: %w", notification.ID, err)
	}

	return nil
}

// FindByID returns a notification by its ID. Returns nil, nil if not found.
func (r *NotificationRepository) FindByID(ctx context.Context, id string) (*domain.Notification, error) {
	var n domain.Notification
	var channel string
	var status string
	err := r.pool.QueryRow(ctx,
		"SELECT id, recipient, channel, subject, body, status, sent_at FROM notifications WHERE id = $1",
		id,
	).Scan(&n.ID, &n.Recipient, &channel, &n.Subject, &n.Body, &status, &n.SentAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying notification %s: %w", id, err)
	}

	n.Channel = domain.Channel(channel)
	n.Status = domain.NotificationStatus(status)

	return &n, nil
}

// FindByRecipient returns all notifications for a given recipient.
func (r *NotificationRepository) FindByRecipient(ctx context.Context, recipient string) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, recipient, channel, subject, body, status, sent_at FROM notifications WHERE recipient = $1",
		recipient,
	)
	if err != nil {
		return nil, fmt.Errorf("querying notifications for recipient %s: %w", recipient, err)
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		var n domain.Notification
		var channel string
		var status string
		if err := rows.Scan(&n.ID, &n.Recipient, &channel, &n.Subject, &n.Body, &status, &n.SentAt); err != nil {
			return nil, fmt.Errorf("scanning notification row: %w", err)
		}
		n.Channel = domain.Channel(channel)
		n.Status = domain.NotificationStatus(status)
		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notification rows: %w", err)
	}

	return notifications, nil
}
```

Note: `Channel` and `Status` are stored as TEXT in Postgres and cast back to domain types on read. The `SentAt` field uses `TIMESTAMPTZ` which pgx maps to `time.Time` directly.

- [ ] **Step 5: Verify build**

Run: `go build ./services/notification/...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add services/notification/migrations/ services/notification/internal/adapter/outbound/postgres/
git commit -m "feat(notification): add PostgreSQL adapter and migrations"
```

---

### Task 8: Wire Ratings Composition Root

**Files:**
- Modify: `services/ratings/cmd/main.go:19-22,65-68`

- [ ] **Step 1: Update ratings main.go with storage backend switch**

Replace the current hex arch wiring section in `services/ratings/cmd/main.go`. The file currently has these imports and wiring:

```go
// Current imports (line 19-21):
handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
```

```go
// Current wiring (lines 65-68):
repo := memory.NewRatingRepository()
svc := service.NewRatingService(repo)
h := handler.NewHandler(svc)
```

Update the full file to:

```go
// Package main is the entry point for the ratings service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	ratingsSubmitted, _ := meter.Int64Counter(
		"ratings_submitted_total",
		metric.WithDescription("Total number of ratings submitted"),
	)
	_ = ratingsSubmitted // Will be incremented via middleware or service decorator in a future iteration

	// Wire hex arch — select adapter based on storage backend
	var repo port.RatingRepository
	var readinessChecks []func() error

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
			logger.Info("database migrations applied")
		}

		repo = postgres.NewRatingRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewRatingRepository()
		logger.Info("using memory storage backend")
	}

	svc := service.NewRatingService(repo)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./services/ratings/...`
Expected: clean build

- [ ] **Step 3: Run existing tests to ensure no regression**

Run: `go test ./services/ratings/... -v`
Expected: all tests pass (existing tests use memory backend)

- [ ] **Step 4: Commit**

```bash
git add services/ratings/cmd/main.go
git commit -m "feat(ratings): wire postgres adapter in composition root"
```

---

### Task 9: Wire Details Composition Root

**Files:**
- Modify: `services/details/cmd/main.go`

- [ ] **Step 1: Update details main.go with storage backend switch**

Update the full file to:

```go
// Package main is the entry point for the details service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/inbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	booksAdded, _ := meter.Int64Counter(
		"books_added_total",
		metric.WithDescription("Total number of books added"),
	)
	_ = booksAdded

	// Wire hex arch — select adapter based on storage backend
	var repo port.DetailRepository
	var readinessChecks []func() error

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
			logger.Info("database migrations applied")
		}

		repo = postgres.NewDetailRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewDetailRepository()
		logger.Info("using memory storage backend")
	}

	svc := service.NewDetailService(repo)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./services/details/...`
Expected: clean build

- [ ] **Step 3: Run existing tests**

Run: `go test ./services/details/... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add services/details/cmd/main.go
git commit -m "feat(details): wire postgres adapter in composition root"
```

---

### Task 10: Wire Reviews Composition Root

**Files:**
- Modify: `services/reviews/cmd/main.go`

- [ ] **Step 1: Update reviews main.go with storage backend switch**

The reviews service has an additional `RatingsClient` outbound port that is unaffected by the storage backend change. Only the `ReviewRepository` switches. The `envOrDefault` function remains for `RATINGS_SERVICE_URL`.

Update the full file to:

```go
// Package main is the entry point for the reviews service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/inbound/http"
	ratingshttp "github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/http"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metric
	meter := otel.Meter(cfg.ServiceName)
	reviewsSubmitted, _ := meter.Int64Counter(
		"reviews_submitted_total",
		metric.WithDescription("Total number of reviews submitted"),
	)
	_ = reviewsSubmitted

	// Wire hex arch — select adapter based on storage backend
	var repo port.ReviewRepository
	var readinessChecks []func() error

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
			logger.Info("database migrations applied")
		}

		repo = postgres.NewReviewRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewReviewRepository()
		logger.Info("using memory storage backend")
	}

	ratingsURL := envOrDefault("RATINGS_SERVICE_URL", "http://localhost:8080")
	ratingsClient := ratingshttp.NewRatingsClient(ratingsURL)
	svc := service.NewReviewService(repo, ratingsClient)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./services/reviews/...`
Expected: clean build

- [ ] **Step 3: Run existing tests**

Run: `go test ./services/reviews/... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add services/reviews/cmd/main.go
git commit -m "feat(reviews): wire postgres adapter in composition root"
```

---

### Task 11: Wire Notification Composition Root

**Files:**
- Modify: `services/notification/cmd/main.go`

- [ ] **Step 1: Update notification main.go with storage backend switch**

The notification service has two outbound ports: `NotificationRepository` (switches based on storage backend) and `NotificationDispatcher` (always log dispatcher, unaffected).

Update the full file to:

```go
// Package main is the entry point for the notification service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/database"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/metrics"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	handler "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/inbound/http"
	logdispatcher "github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/log"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/adapter/outbound/postgres"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/port"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/internal/core/service"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/notification/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, cfg.ServiceName)

	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(ctx) }()

	metricsHandler, err := metrics.Setup(cfg.ServiceName)
	if err != nil {
		logger.Error("failed to setup metrics", "error", err)
		os.Exit(1)
	}

	stopProfiler, err := profiling.Start(cfg)
	if err != nil {
		logger.Error("failed to start profiling", "error", err)
		os.Exit(1)
	}
	defer stopProfiler()

	metrics.RegisterRuntimeMetrics()

	// Business metrics
	meter := otel.Meter(cfg.ServiceName)
	notificationsDispatched, _ := meter.Int64Counter(
		"notifications_dispatched_total",
		metric.WithDescription("Total number of notifications dispatched by channel"),
	)
	notificationsFailed, _ := meter.Int64Counter(
		"notifications_failed_total",
		metric.WithDescription("Total number of failed notification dispatches by channel"),
	)
	notificationsByStatus, _ := meter.Int64UpDownCounter(
		"notifications_by_status",
		metric.WithDescription("Current count of notifications by status"),
	)
	_ = notificationsDispatched
	_ = notificationsFailed
	_ = notificationsByStatus

	// Wire hex arch — select adapter based on storage backend
	var repo port.NotificationRepository
	var readinessChecks []func() error

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
			logger.Info("database migrations applied")
		}

		repo = postgres.NewNotificationRepository(pool)
		readinessChecks = append(readinessChecks, database.HealthCheck(pool))
		logger.Info("using postgres storage backend")
	default:
		repo = memory.NewNotificationRepository()
		logger.Info("using memory storage backend")
	}

	dispatcher := logdispatcher.NewDispatcher()
	svc := service.NewNotificationService(repo, dispatcher)
	h := handler.NewHandler(svc)

	registerRoutes := func(mux *http.ServeMux) {
		h.RegisterRoutes(mux)
	}

	if err := server.Run(ctx, cfg, registerRoutes, metricsHandler, readinessChecks...); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./services/notification/...`
Expected: clean build

- [ ] **Step 3: Run existing tests**

Run: `go test ./services/notification/... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add services/notification/cmd/main.go
git commit -m "feat(notification): wire postgres adapter in composition root"
```

---

### Task 12: Full Build and Test Verification

**Files:** (none — verification only)

- [ ] **Step 1: Build all services**

Run: `make build-all`
Expected: all 5 services build successfully (productpage is unmodified but should still build)

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v`
Expected: all tests pass (they all use memory backend by default)

- [ ] **Step 3: Run linter**

Run: `make lint`
Expected: no new violations

- [ ] **Step 4: Tidy modules**

Run: `go mod tidy`
Expected: no changes (if changes, commit them)

---

### Task 13: Docker Compose Postgres E2E Infrastructure

**Files:**
- Create: `test/e2e/init-databases.sql`
- Create: `test/e2e/docker-compose.postgres.yml`

- [ ] **Step 1: Create database initialization script**

Create `test/e2e/init-databases.sql`:

```sql
CREATE DATABASE bookinfo_ratings;
CREATE DATABASE bookinfo_details;
CREATE DATABASE bookinfo_reviews;
CREATE DATABASE bookinfo_notification;
```

- [ ] **Step 2: Create postgres docker-compose override**

Create `test/e2e/docker-compose.postgres.yml`:

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

- [ ] **Step 3: Commit**

```bash
git add test/e2e/init-databases.sql test/e2e/docker-compose.postgres.yml
git commit -m "feat(e2e): add docker-compose postgres override and init script"
```

---

### Task 14: Add Makefile E2E Postgres Target

**Files:**
- Modify: `Makefile:93-98`

- [ ] **Step 1: Add e2e-postgres target**

In the `Makefile`, after the existing `e2e` target (line 97), add:

```makefile
.PHONY: e2e-postgres
e2e-postgres: ## Run E2E tests with PostgreSQL backend
	COMPOSE_FILE="docker-compose.yml -f docker-compose.postgres.yml" bash test/e2e/run-tests.sh
```

This requires a small update to `test/e2e/run-tests.sh` to use the `COMPOSE_FILE` env var if set. Update lines 14 and 19 of `run-tests.sh`:

Replace line 14:
```bash
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d --build
```
with:
```bash
COMPOSE_FILES="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_ARGS=""
for f in $COMPOSE_FILES; do
    COMPOSE_ARGS="$COMPOSE_ARGS -f $SCRIPT_DIR/$f"
done
docker compose $COMPOSE_ARGS up -d --build
```

Replace line 19:
```bash
docker compose -f "$SCRIPT_DIR/docker-compose.yml" down -v
```
with:
```bash
docker compose $COMPOSE_ARGS down -v
```

The full updated `run-tests.sh` becomes:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Build compose args from COMPOSE_FILE env var (default: just docker-compose.yml)
COMPOSE_FILES="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_ARGS=""
for f in $COMPOSE_FILES; do
    COMPOSE_ARGS="$COMPOSE_ARGS -f $SCRIPT_DIR/$f"
done

# Start docker-compose
echo -e "${YELLOW}Starting services...${NC}"
docker compose $COMPOSE_ARGS up -d --build

# Cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}Stopping services...${NC}"
    docker compose $COMPOSE_ARGS down -v
}
trap cleanup EXIT

# Wait for services to be ready (poll health endpoints from host)
wait_for_service() {
    local name=$1 url=$2 max=30
    echo -n "Waiting for $name..."
    for i in $(seq 1 $max); do
        if curl -sf "$url" > /dev/null 2>&1; then
            echo -e " ${GREEN}ready${NC}"
            return 0
        fi
        sleep 1
        echo -n "."
    done
    echo -e " ${RED}timeout${NC}"
    return 1
}

wait_for_service "ratings"       "http://localhost:9093/healthz"
wait_for_service "details"       "http://localhost:9091/healthz"
wait_for_service "reviews"       "http://localhost:9092/healthz"
wait_for_service "notification"  "http://localhost:9094/healthz"
wait_for_service "productpage"   "http://localhost:9090/healthz"

# Run tests
FAILED=0
run_test() {
    local script=$1
    echo -e "\n${YELLOW}Running $script...${NC}"
    if bash "$SCRIPT_DIR/$script"; then
        echo -e "${GREEN}PASS: $script${NC}"
    else
        echo -e "${RED}FAIL: $script${NC}"
        FAILED=1
    fi
}

run_test test-ratings.sh
run_test test-details.sh
run_test test-reviews.sh
run_test test-notification.sh
run_test test-productpage.sh

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}All E2E tests passed!${NC}"
else
    echo -e "\n${RED}Some E2E tests failed.${NC}"
    exit 1
fi
```

- [ ] **Step 2: Verify Makefile syntax**

Run: `make help`
Expected: `e2e-postgres` appears in the help output with description "Run E2E tests with PostgreSQL backend"

- [ ] **Step 3: Commit**

```bash
git add Makefile test/e2e/run-tests.sh
git commit -m "feat: add e2e-postgres Makefile target"
```
