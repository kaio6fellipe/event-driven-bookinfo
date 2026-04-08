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
