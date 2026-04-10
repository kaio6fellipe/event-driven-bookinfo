# Database Operation Tracing — Design Spec

**Date**: 2026-04-10
**Status**: Approved

## Goal

Add OpenTelemetry tracing to all PostgreSQL and Redis operations so that database calls appear as child spans in distributed traces. This fills the visibility gap between HTTP-level tracing (already instrumented) and storage-level operations (currently invisible).

## Motivation

Current traces show HTTP request flow between services but go dark once a request hits a database or cache call. This makes it impossible to:

- Identify slow queries contributing to request latency
- Correlate database errors with the HTTP requests that triggered them
- See which SQL statements ran and how long they took

## Approach: Adapter-Level Instrumentation

Instrument at the shared infrastructure layer rather than in individual repository methods. This follows the same pattern used for HTTP tracing (`otelhttp` wrapping in `pkg/server`).

## PostgreSQL Tracing

### Implementation

Modify `pkg/database/pool.go` to:

1. Parse the connection string into a `pgxpool.Config` (instead of passing the raw URL)
2. Attach an `otelpgx` tracer to `Config.ConnConfig.Tracer`
3. Create the pool from the config

```go
cfg, err := pgxpool.ParseConfig(databaseURL)
if err != nil {
    return nil, fmt.Errorf("parsing database URL: %w", err)
}
cfg.ConnConfig.Tracer = otelpgx.NewTracer(
    otelpgx.WithTrimSQLInSpanName(),
)
pool, err := pgxpool.NewWithConfig(ctx, cfg)
```

### Span Attributes

The `otelpgx` tracer automatically creates child spans for `Query`, `QueryRow`, `Exec`, `Prepare`, and `Connect` operations with:

- `db.system`: `"postgresql"`
- `db.statement`: full SQL with `$1` placeholders (sanitized — no bind parameter values)
- `db.operation`: `SELECT` / `INSERT` / `UPDATE` / `DELETE`
- Error status propagated to span

### Impact

All 4 services using `pkg/database.NewPool` (details, ratings, reviews, notification) get tracing automatically. Zero changes needed in any adapter or repository file.

### Dependency

- `github.com/exaring/otelpgx`

## Redis Tracing

### Implementation

Modify `services/productpage/internal/pending/redis.go` to add the `redisotel` tracing hook after client creation:

```go
client := redis.NewClient(opts)
if err := redisotel.InstrumentTracing(client,
    redisotel.WithDBStatement(false),
); err != nil {
    return nil, fmt.Errorf("instrumenting redis tracing: %w", err)
}
```

### Span Attributes

The `redisotel` hook automatically creates spans for every Redis command with:

- `db.system`: `"redis"`
- `db.operation`: `RPUSH` / `LRANGE` / `LREM` / `PING`
- `net.peer.name` / `net.peer.port`: Redis host info
- Span name includes command + key (e.g., `RPUSH pending:reviews:p001`)
- `WithDBStatement(false)` excludes full command arguments (values) from span attributes

### Impact

Only affects productpage service (the only Redis consumer). Transparent to `NoopStore` — no changes needed there.

### Dependency

- `github.com/redis/go-redis/extra/redisotel/v9`

## Testing Strategy

### Unit Tests

- **`pkg/database/`**: Verify pool is created with tracer configured (parse config, assert `ConnConfig.Tracer` is non-nil).
- **`pending/redis.go`**: Existing `miniredis`-based tests continue to pass — the tracing hook is transparent. Add a test verifying `InstrumentTracing` succeeds on a valid client.

### Integration Verification

After deployment to local k8s, validate in Grafana Tempo:

1. Submit a review via productpage
2. Open the resulting trace
3. Confirm Postgres spans appear as children of HTTP handler spans (e.g., `SELECT` under `GET /partials/reviews`)
4. Confirm Redis spans appear for pending review operations (e.g., `RPUSH`, `LRANGE`)

## Files Changed

| File | Change |
|------|--------|
| `go.mod` | Add `otelpgx` and `redisotel` dependencies |
| `pkg/database/pool.go` | Parse config, attach `otelpgx` tracer, create pool from config |
| `services/productpage/internal/pending/redis.go` | Add `redisotel.InstrumentTracing` hook |
| `pkg/database/pool_test.go` (new) | Test pool creation with tracer |
| `services/productpage/internal/pending/pending_test.go` | Add tracing hook verification test |

## Out of Scope

- Manual spans in business logic or service layer
- Database connection pool metrics (separate concern)
- Docker Compose OTEL configuration (tracing stays k8s-only, matching current pattern)
- Changes to adapter/repository files (instrumentation is transparent)
