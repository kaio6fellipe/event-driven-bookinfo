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
