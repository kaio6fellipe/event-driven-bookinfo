package pending

import "context"

// NoopStore is a PendingStore that does nothing. Used when REDIS_URL is unset.
type NoopStore struct{}

// StorePending is a no-op.
func (NoopStore) StorePending(_ context.Context, _ string, _ Review) error {
	return nil
}

// GetAndReconcile is a no-op that always returns nil.
func (NoopStore) GetAndReconcile(_ context.Context, _ string, _ []ConfirmedReview) ([]Review, error) {
	return nil, nil
}
