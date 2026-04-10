package pending

import "context"

// NoopStore is a PendingStore that does nothing. Used when REDIS_URL is unset.
type NoopStore struct{}

func (NoopStore) StorePending(_ context.Context, _ string, _ Review) error {
	return nil
}

func (NoopStore) GetAndReconcile(_ context.Context, _ string, _ []ConfirmedReview) ([]Review, error) {
	return nil, nil
}
