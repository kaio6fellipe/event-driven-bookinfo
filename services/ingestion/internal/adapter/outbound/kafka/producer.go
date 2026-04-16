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
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

const (
	ceType    = "com.bookinfo.ingestion.book-added"
	ceSource  = "ingestion"
	ceVersion = "1.0"

	defaultPartitions        = 3
	defaultReplicationFactor = 1
)

// bookEvent is the CloudEvents data payload published to Kafka.
type bookEvent struct {
	Title          string   `json:"title"`
	Authors        []string `json:"authors"`
	ISBN           string   `json:"isbn"`
	PublishYear    int      `json:"publish_year"`
	Subjects       []string `json:"subjects,omitempty"`
	Pages          int      `json:"pages,omitempty"`
	Publisher      string   `json:"publisher,omitempty"`
	Language       string   `json:"language,omitempty"`
	ISBN10         string   `json:"isbn_10,omitempty"`
	ISBN13         string   `json:"isbn_13,omitempty"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// KafkaClient abstracts the franz-go client for testing.
type KafkaClient interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Producer implements port.EventPublisher by producing to Kafka.
type Producer struct {
	client KafkaClient
	topic  string
}

// NewProducer creates a real Kafka producer that connects to the given brokers.
// It auto-creates the topic if it does not exist.
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
func NewProducerWithClient(client KafkaClient, topic string) *Producer {
	return &Producer{client: client, topic: topic}
}

// PublishBookAdded sends a book-added CloudEvent to Kafka.
func (p *Producer) PublishBookAdded(ctx context.Context, book domain.Book) error {
	logger := logging.FromContext(ctx)

	isbn10, isbn13 := classifyISBN(book.ISBN)

	evt := bookEvent{
		Title:          book.Title,
		Authors:        book.Authors,
		ISBN:           book.ISBN,
		PublishYear:    book.PublishYear,
		Subjects:       book.Subjects,
		Pages:          book.Pages,
		Publisher:      book.Publisher,
		Language:       book.Language,
		ISBN10:         isbn10,
		ISBN13:         isbn13,
		IdempotencyKey: fmt.Sprintf("ingestion-isbn-%s", book.ISBN),
	}

	value, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshaling book event: %w", err)
	}

	now := time.Now().UTC()
	record := &kgo.Record{
		Topic: p.topic,
		Key:   []byte(book.ISBN),
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: "ce_specversion", Value: []byte(ceVersion)},
			{Key: "ce_type", Value: []byte(ceType)},
			{Key: "ce_source", Value: []byte(ceSource)},
			{Key: "ce_id", Value: []byte(uuid.New().String())},
			{Key: "ce_time", Value: []byte(now.Format(time.RFC3339))},
			{Key: "ce_subject", Value: []byte(book.ISBN)},
			{Key: "content-type", Value: []byte("application/json")},
		},
	}

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to Kafka: %w", err)
	}

	logger.Debug("published book-added event to Kafka", "topic", p.topic, "isbn", book.ISBN)
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

func classifyISBN(isbn string) (isbn10, isbn13 string) {
	if len(isbn) == 13 {
		return "", isbn
	}
	return isbn, ""
}
