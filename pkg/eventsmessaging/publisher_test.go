package eventsmessaging_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
)

// stubPublisher confirms the interface compiles with a minimal impl.
type stubPublisher struct {
	calls int
}

func (s *stubPublisher) Publish(_ context.Context, _ events.Descriptor, _ any, _, _ string) error {
	s.calls++
	return nil
}

func (s *stubPublisher) Close() {}

func TestPublisherInterface(t *testing.T) {
	var p eventsmessaging.Publisher = &stubPublisher{}
	if err := p.Publish(context.Background(), events.Descriptor{}, nil, "k", "ik"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	p.Close()
}
