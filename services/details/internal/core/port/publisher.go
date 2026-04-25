// file: services/details/internal/core/port/publisher.go
package port

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// EventPublisher is the outbound port for emitting domain events to a message broker.
type EventPublisher interface {
	PublishBookAdded(ctx context.Context, evt domain.BookAddedEvent) error
}
