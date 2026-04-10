// Package database provides PostgreSQL connection pool, migration, and health check utilities.
package database

import (
	"context"
	"fmt"
	"net/url"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
)

// NewPoolConfig parses a database URL and returns a pgxpool.Config with
// OpenTelemetry tracing enabled. The tracer creates child spans for every
// Query, QueryRow, Exec, Prepare, and Connect call.
func NewPoolConfig(databaseURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}
	tracerOpts := []otelpgx.Option{
		otelpgx.WithTrimSQLInSpanName(),
	}
	if u, err := url.Parse(databaseURL); err == nil && u.Hostname() != "" {
		tracerOpts = append(tracerOpts, otelpgx.WithTracerAttributes(
			attribute.String("server.address", u.Hostname()),
			attribute.String("server.port", u.Port()),
		))
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(tracerOpts...)
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
