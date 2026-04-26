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
	"go.opentelemetry.io/otel/codes"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

const (
	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

// RatingSubmittedPayload is the marshaled Kafka record value for a
// rating-submitted CloudEvent. Exported because the events.Descriptor in
// exposed.go references it as a JSONSchema source for tools/specgen.
type RatingSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Stars          int    `json:"stars"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer emits ratings domain events to Kafka.
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

// Publish marshals payload to JSON, builds a CloudEvents-binary Kafka
// record using the descriptor's CEType/CESource/Version, and produces it
// to the producer's configured topic. recordKey is the partition key
// (typically the natural-key field of payload, e.g. ProductID); when
// empty, idempotencyKey is used as a fallback.
func (p *Producer) Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", d.Name, err)
	}

	keyBytes := []byte(recordKey)
	if len(keyBytes) == 0 {
		keyBytes = []byte(idempotencyKey)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   keyBytes,
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(d.Version)},
			{Key: "ce_type", Value: []byte(d.CEType)},
			{Key: "ce_source", Value: []byte(d.CESource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(recordKey)},
			{Key: "content-type", Value: []byte(d.ContentType)},
		},
	}

	ctx, span := telemetry.StartProducerSpan(ctx, p.topic, idempotencyKey)
	defer span.End()

	telemetry.InjectTraceContext(ctx, record)

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published event",
		"topic", p.topic,
		"ce_type", d.CEType,
		"idempotency_key", idempotencyKey,
	)
	return nil
}

// PublishRatingSubmitted sends a rating-submitted CloudEvent to Kafka.
// Thin typed wrapper around Publish; the descriptor is the single source
// of truth for CE headers (exposed.go).
func (p *Producer) PublishRatingSubmitted(ctx context.Context, evt domain.RatingSubmittedEvent) error {
	body := RatingSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Stars:          evt.Stars,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.Publish(ctx, Exposed[0], body, evt.ProductID, evt.IdempotencyKey)
}

// Close flushes pending messages and closes the Kafka client.
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
