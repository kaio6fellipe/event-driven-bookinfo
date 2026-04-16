package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaadapter "github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/adapter/outbound/kafka"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/ingestion/internal/core/domain"
)

// fakeClient captures records produced via ProduceSync.
type fakeClient struct {
	mu      sync.Mutex
	records []*kgo.Record
}

func (f *fakeClient) ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	var results kgo.ProduceResults
	for _, r := range rs {
		f.records = append(f.records, r)
		results = append(results, kgo.ProduceResult{Record: r})
	}
	return results
}

func (f *fakeClient) Close() {}

func TestPublishBookAdded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		book    domain.Book
		wantKey string
	}{
		{
			name: "valid book with ISBN-13",
			book: domain.Book{
				Title:       "The Go Programming Language",
				Authors:     []string{"Alan Donovan", "Brian Kernighan"},
				ISBN:        "9780134190440",
				PublishYear: 2015,
				Pages:       380,
				Publisher:   "Addison-Wesley",
				Language:    "en",
			},
			wantKey: "9780134190440",
		},
		{
			name: "valid book with ISBN-10",
			book: domain.Book{
				Title:       "Learning Go",
				Authors:     []string{"Jon Bodner"},
				ISBN:        "1492077216",
				PublishYear: 2021,
				Pages:       375,
				Publisher:   "O'Reilly",
				Language:    "en",
			},
			wantKey: "1492077216",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := &fakeClient{}
			p := kafkaadapter.NewProducerWithClient(fc, "raw_books_details")

			err := p.PublishBookAdded(context.Background(), tt.book)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			fc.mu.Lock()
			defer fc.mu.Unlock()

			if len(fc.records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(fc.records))
			}

			r := fc.records[0]

			// Verify topic
			if r.Topic != "raw_books_details" {
				t.Errorf("topic = %q, want %q", r.Topic, "raw_books_details")
			}

			// Verify key is ISBN
			if string(r.Key) != tt.wantKey {
				t.Errorf("key = %q, want %q", string(r.Key), tt.wantKey)
			}

			// Verify CloudEvents headers
			headers := make(map[string]string)
			for _, h := range r.Headers {
				headers[h.Key] = string(h.Value)
			}

			if headers["ce_type"] != "com.bookinfo.ingestion.book-added" {
				t.Errorf("ce_type = %q, want %q", headers["ce_type"], "com.bookinfo.ingestion.book-added")
			}
			if headers["ce_source"] != "ingestion" {
				t.Errorf("ce_source = %q, want %q", headers["ce_source"], "ingestion")
			}
			if headers["ce_specversion"] != "1.0" {
				t.Errorf("ce_specversion = %q, want %q", headers["ce_specversion"], "1.0")
			}
			if headers["ce_subject"] != tt.wantKey {
				t.Errorf("ce_subject = %q, want %q", headers["ce_subject"], tt.wantKey)
			}
			if headers["content-type"] != "application/json" {
				t.Errorf("content-type = %q, want %q", headers["content-type"], "application/json")
			}
			if headers["ce_id"] == "" {
				t.Error("ce_id header is empty, expected a UUID")
			}
			if headers["ce_time"] == "" {
				t.Error("ce_time header is empty, expected RFC3339 timestamp")
			}

			// Verify body is valid JSON with expected fields
			var body map[string]interface{}
			if err := json.Unmarshal(r.Value, &body); err != nil {
				t.Fatalf("failed to unmarshal record value: %v", err)
			}
			if body["title"] != tt.book.Title {
				t.Errorf("body title = %q, want %q", body["title"], tt.book.Title)
			}
			// Verify author is a joined string (matches details service DTO)
			if body["author"] == nil || body["author"] == "" {
				t.Error("body missing author field")
			}
			if body["year"] == nil {
				t.Error("body missing year field")
			}
			if body["type"] != "paperback" {
				t.Errorf("body type = %q, want %q", body["type"], "paperback")
			}
			if body["idempotency_key"] == nil {
				t.Error("body missing idempotency_key")
			}
		})
	}
}

func TestPublishBookAdded_ProduceError(t *testing.T) {
	t.Parallel()

	fc := &errorClient{}
	p := kafkaadapter.NewProducerWithClient(fc, "raw_books_details")

	book := domain.Book{
		Title:       "Test Book",
		Authors:     []string{"Author"},
		ISBN:        "1234567890",
		PublishYear: 2024,
	}

	err := p.PublishBookAdded(context.Background(), book)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// errorClient simulates a produce failure.
type errorClient struct{}

func (e *errorClient) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	var results kgo.ProduceResults
	for _, r := range rs {
		results = append(results, kgo.ProduceResult{
			Record: r,
			Err:    context.DeadlineExceeded,
		})
	}
	return results
}

func (e *errorClient) Close() {}
