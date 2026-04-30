// Package eventsmessaging defines the bus-agnostic Publisher contract used
// by every producer service. Concrete impls live in subpackages
// (kafkapub for Kafka, natspub for NATS JetStream); cmd/main.go selects
// one at startup via the EVENT_BACKEND env var.
package eventsmessaging

import (
	"context"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
)

// Publisher abstracts the per-record send path for any messaging backend.
// Implementations are responsible for marshalling the payload, building
// CloudEvents-binary headers from the descriptor, and propagating OTel
// trace context.
type Publisher interface {
	Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error
	Close()
}
