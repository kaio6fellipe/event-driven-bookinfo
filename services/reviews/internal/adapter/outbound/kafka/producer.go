// Package kafka implements the EventPublisher port using a native Kafka producer.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

const (
	ceTypeReviewSubmitted = "com.bookinfo.reviews.review-submitted"
	ceTypeReviewDeleted   = "com.bookinfo.reviews.review-deleted"
	ceSource              = "reviews"
	ceVersion             = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

type submittedBody struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

type deletedBody struct {
	ReviewID       string `json:"review_id"`
	ProductID      string `json:"product_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer emits reviews domain events to Kafka.
type Producer struct {
	client Client
	topic  string
}

// NewProducer creates a real producer connecting to the brokers. Creates topic if missing.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	seeds := strings.Split(brokers, ",")
	client, err := kgo.NewClient(kgo.SeedBrokers(seeds...), kgo.DefaultProduceTopic(topic))
	if err != nil {
		return nil, fmt.Errorf("creating Kafka client: %w", err)
	}
	if err := ensureTopic(ctx, client, topic); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring topic %q: %w", topic, err)
	}
	return &Producer{client: client, topic: topic}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for tests).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishReviewSubmitted sends a review-submitted CloudEvent to Kafka.
func (p *Producer) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	body := submittedBody{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Text:           evt.Text,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.produce(ctx, ceTypeReviewSubmitted, evt.IdempotencyKey, evt.ProductID, body)
}

// PublishReviewDeleted sends a review-deleted CloudEvent to Kafka.
func (p *Producer) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	body := deletedBody{
		ReviewID:       evt.ReviewID,
		ProductID:      evt.ProductID,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.produce(ctx, ceTypeReviewDeleted, evt.IdempotencyKey, evt.ProductID, body)
}

func (p *Producer) produce(ctx context.Context, ceType, key, partitionHint string, body any) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	recordKey := []byte(partitionHint)
	if len(recordKey) == 0 {
		recordKey = []byte(key)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   recordKey,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceType)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(key)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	telemetry.InjectTraceContext(ctx, record)
	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published reviews event", "topic", p.topic, "ce_type", ceType, "idempotency_key", key)
	return nil
}

// Close flushes and closes the underlying client.
func (p *Producer) Close() { p.client.Close() }

func ensureTopic(ctx context.Context, client *kgo.Client, topic string) error {
	admin := kadm.NewClient(client)
	resp, err := admin.CreateTopics(ctx, int32(defaultPartitions), int16(defaultReplicationFactor), nil, topic)
	if err != nil {
		return fmt.Errorf("creating topic: %w", err)
	}
	for _, t := range resp.Sorted() {
		if t.Err != nil && t.ErrMessage != "" && !isTopicExistsError(t.ErrMessage) {
			return fmt.Errorf("topic %q: %s", t.Topic, t.ErrMessage)
		}
	}
	return nil
}

func isTopicExistsError(msg string) bool {
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "TOPIC_ALREADY_EXISTS")
}
