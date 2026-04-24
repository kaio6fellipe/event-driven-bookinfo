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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

const (
	ceTypeBookAdded = "com.bookinfo.details.book-added"
	ceSource        = "details"
	ceVersion       = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

// bookAddedBody is the marshaled Kafka record value for a BookAddedEvent.
type bookAddedBody struct {
	ID             string `json:"id,omitempty"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Year           int    `json:"year"`
	Type           string `json:"type"`
	Pages          int    `json:"pages,omitempty"`
	Publisher      string `json:"publisher,omitempty"`
	Language       string `json:"language,omitempty"`
	ISBN10         string `json:"isbn_10,omitempty"`
	ISBN13         string `json:"isbn_13,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Client abstracts the franz-go client for testing.
type Client interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer implements port.EventPublisher by producing to Kafka.
type Producer struct {
	client Client
	topic  string
}

// NewProducer creates a real Kafka producer connecting to the given brokers.
// Auto-creates the topic if it does not exist.
func NewProducer(ctx context.Context, brokers, topic string) (*Producer, error) {
	seeds := strings.Split(brokers, ",")

	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Kafka client: %w", err)
	}

	if err := ensureTopic(ctx, client, topic); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensuring topic %q: %w", topic, err)
	}

	return &Producer{client: client, topic: topic}, nil
}

// NewProducerWithClient creates a Producer with an injected client (for testing).
func NewProducerWithClient(client Client, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishBookAdded sends a book-added CloudEvent to Kafka.
func (p *Producer) PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error {
	logger := logging.FromContext(ctx)

	body := bookAddedBody{
		ID:             evt.ID,
		Title:          evt.Title,
		Author:         evt.Author,
		Year:           evt.Year,
		Type:           evt.Type,
		Pages:          evt.Pages,
		Publisher:      evt.Publisher,
		Language:       evt.Language,
		ISBN10:         evt.ISBN10,
		ISBN13:         evt.ISBN13,
		IdempotencyKey: evt.IdempotencyKey,
	}

	value, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling book-added event: %w", err)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(evt.IdempotencyKey),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceTypeBookAdded)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(evt.IdempotencyKey)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published book-added event to Kafka", "topic", p.topic, "idempotency_key", evt.IdempotencyKey)
	return nil
}

// Close flushes pending messages and closes the Kafka client.
func (p *Producer) Close() {
	p.client.Close()
}

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
