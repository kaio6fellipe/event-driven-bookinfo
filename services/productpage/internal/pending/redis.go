package pending

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const (
	pendingKeyPrefix  = "pending:reviews:"
	deletingKeyPrefix = "deleting:reviews:"
)

// RedisStore implements Store backed by Redis lists.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a RedisStore from a Redis URL (e.g. "redis://localhost:6379").
// The client is instrumented with OpenTelemetry tracing — every command
// produces a child span with operation name and key.
func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(client,
		redisotel.WithDBStatement(false),
	); err != nil {
		return nil, fmt.Errorf("instrumenting redis tracing: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// Ping verifies the connection to Redis.
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// StorePending appends a pending review to the Redis list for the given product.
func (s *RedisStore) StorePending(ctx context.Context, productID string, review Review) error {
	data, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("marshaling pending review: %w", err)
	}
	return s.client.RPush(ctx, pendingKeyPrefix+productID, data).Err()
}

// StoreDeleting marks a review as being deleted by adding its ID to a Redis set.
func (s *RedisStore) StoreDeleting(ctx context.Context, productID string, reviewID string) error {
	return s.client.SAdd(ctx, deletingKeyPrefix+productID, reviewID).Err()
}

// GetAndReconcile returns pending reviews and deleting review IDs after reconciliation.
func (s *RedisStore) GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview, confirmedIDs []string) ([]Review, []string, error) {
	pendingReviews, err := s.reconcilePending(ctx, productID, confirmed)
	if err != nil {
		return nil, nil, fmt.Errorf("reconciling pending reviews: %w", err)
	}

	deletingIDs, err := s.reconcileDeleting(ctx, productID, confirmedIDs)
	if err != nil {
		return pendingReviews, nil, fmt.Errorf("reconciling deleting reviews: %w", err)
	}

	return pendingReviews, deletingIDs, nil
}

func (s *RedisStore) reconcilePending(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, error) {
	key := pendingKeyPrefix + productID

	vals, err := s.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching pending reviews: %w", err)
	}

	if len(vals) == 0 {
		return nil, nil
	}

	confirmedSet := make(map[string]struct{}, len(confirmed))
	for _, c := range confirmed {
		confirmedSet[c.Reviewer+"\x00"+c.Text] = struct{}{}
	}

	var remaining []Review
	for _, raw := range vals {
		var r Review
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			continue // skip corrupted entries
		}

		matchKey := r.Reviewer + "\x00" + r.Text
		if _, found := confirmedSet[matchKey]; found {
			s.client.LRem(ctx, key, 1, raw)
			delete(confirmedSet, matchKey)
		} else {
			remaining = append(remaining, r)
		}
	}

	return remaining, nil
}

func (s *RedisStore) reconcileDeleting(ctx context.Context, productID string, confirmedIDs []string) ([]string, error) {
	key := deletingKeyPrefix + productID

	deletingIDs, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("fetching deleting review IDs: %w", err)
	}

	if len(deletingIDs) == 0 {
		return nil, nil
	}

	confirmedSet := make(map[string]struct{}, len(confirmedIDs))
	for _, id := range confirmedIDs {
		confirmedSet[id] = struct{}{}
	}

	var stillDeleting []string
	for _, id := range deletingIDs {
		if _, found := confirmedSet[id]; !found {
			// Review no longer in confirmed list — deletion confirmed
			s.client.SRem(ctx, key, id)
		} else {
			// Review still exists — still deleting
			stillDeleting = append(stillDeleting, id)
		}
	}

	return stillDeleting, nil
}
