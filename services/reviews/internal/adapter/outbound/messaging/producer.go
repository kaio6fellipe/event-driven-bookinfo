// Package messaging implements the EventPublisher port using a backend
// chosen at startup (kafka or jetstream).
package messaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewSubmittedPayload is the marshaled record value for a
// review-submitted CloudEvent.
type ReviewSubmittedPayload struct {
	ID             string `json:"id,omitempty"`
	ProductID      string `json:"product_id"`
	Reviewer       string `json:"reviewer"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ReviewDeletedPayload is the marshaled record value for a
// review-deleted CloudEvent.
type ReviewDeletedPayload struct {
	ReviewID       string `json:"review_id"`
	ProductID      string `json:"product_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Producer wraps an eventsmessaging.Publisher with service-specific
// typed methods. The Publisher impl is chosen by cmd/main.go.
type Producer struct {
	pub eventsmessaging.Publisher
}

// NewProducer builds a Producer from a Publisher. main.go decides which
// concrete impl to pass.
func NewProducer(pub eventsmessaging.Publisher) *Producer {
	return &Producer{pub: pub}
}

// Close releases the underlying publisher.
func (p *Producer) Close() { p.pub.Close() }

// PublishReviewSubmitted sends a review-submitted CloudEvent to the configured backend.
func (p *Producer) PublishReviewSubmitted(ctx context.Context, evt domain.ReviewSubmittedEvent) error {
	body := ReviewSubmittedPayload{
		ID:             evt.ID,
		ProductID:      evt.ProductID,
		Reviewer:       evt.Reviewer,
		Text:           evt.Text,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.pub.Publish(ctx, events.Find(Exposed, "review-submitted"), body, evt.ProductID, evt.IdempotencyKey)
}

// PublishReviewDeleted sends a review-deleted CloudEvent to the configured backend.
func (p *Producer) PublishReviewDeleted(ctx context.Context, evt domain.ReviewDeletedEvent) error {
	body := ReviewDeletedPayload{
		ReviewID:       evt.ReviewID,
		ProductID:      evt.ProductID,
		IdempotencyKey: evt.IdempotencyKey,
	}
	return p.pub.Publish(ctx, events.Find(Exposed, "review-deleted"), body, evt.ProductID, evt.IdempotencyKey)
}
