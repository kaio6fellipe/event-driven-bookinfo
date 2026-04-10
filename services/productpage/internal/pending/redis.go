package pending

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "pending:reviews:"

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
	return s.client.RPush(ctx, keyPrefix+productID, data).Err()
}

// GetAndReconcile returns pending reviews after removing any that match confirmed reviews.
func (s *RedisStore) GetAndReconcile(ctx context.Context, productID string, confirmed []ConfirmedReview) ([]Review, error) {
	key := keyPrefix + productID

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
			// Review confirmed — remove from Redis
			s.client.LRem(ctx, key, 1, raw)
			delete(confirmedSet, matchKey) // only remove first match
		} else {
			remaining = append(remaining, r)
		}
	}

	return remaining, nil
}
