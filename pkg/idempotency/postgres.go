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
