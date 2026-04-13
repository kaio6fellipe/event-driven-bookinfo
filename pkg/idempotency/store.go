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
