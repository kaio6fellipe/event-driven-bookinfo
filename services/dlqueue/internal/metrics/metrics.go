// Package metrics declares dlqueue-specific Prometheus metrics.
package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the dlqueue counters.
type Metrics struct {
	ingested Ingested
	replayed Replayed
	resolved Resolved
	poisoned Poisoned
}

// Ingested is a typed counter for ingest events.
type Ingested struct{ c metric.Int64Counter }

// Add increments the ingested counter.
func (i Ingested) Add(ctx context.Context, source, sensor, trigger string) {
	i.c.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("event_source", source),
			attribute.String("sensor_name", sensor),
			attribute.String("failed_trigger", trigger),
		),
	)
}

// Replayed is a typed counter for replays.
type Replayed struct{ c metric.Int64Counter }

// Add increments the replayed counter.
func (r Replayed) Add(ctx context.Context, source, sensor string) {
	r.c.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event_source", source),
		attribute.String("sensor_name", sensor),
	))
}

// Resolved is a typed counter for resolutions.
type Resolved struct{ c metric.Int64Counter }

// Add increments the resolved counter.
func (r Resolved) Add(ctx context.Context, source, by string) {
	r.c.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event_source", source),
		attribute.String("resolved_by", by),
	))
}

// Poisoned is a typed counter for poison transitions.
type Poisoned struct{ c metric.Int64Counter }

// Add increments the poisoned counter.
func (p Poisoned) Add(ctx context.Context, source, sensor string) {
	p.c.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event_source", source),
		attribute.String("sensor_name", sensor),
	))
}

// New wires OTel counters for the dlqueue service.
func New(serviceName string) (*Metrics, error) {
	meter := otel.Meter(serviceName)

	ing, err := meter.Int64Counter("dlq_events_ingested_total",
		metric.WithDescription("DLQ events ingested"))
	if err != nil {
		return nil, err
	}
	rep, err := meter.Int64Counter("dlq_events_replayed_total",
		metric.WithDescription("DLQ events replayed"))
	if err != nil {
		return nil, err
	}
	res, err := meter.Int64Counter("dlq_events_resolved_total",
		metric.WithDescription("DLQ events resolved"))
	if err != nil {
		return nil, err
	}
	poi, err := meter.Int64Counter("dlq_events_poisoned_total",
		metric.WithDescription("DLQ events transitioned to poisoned"))
	if err != nil {
		return nil, err
	}
	return &Metrics{
		ingested: Ingested{c: ing},
		replayed: Replayed{c: rep},
		resolved: Resolved{c: res},
		poisoned: Poisoned{c: poi},
	}, nil
}

// Ingested returns the ingested counter.
func (m *Metrics) Ingested() Ingested { return m.ingested }

// Replayed returns the replayed counter.
func (m *Metrics) Replayed() Replayed { return m.replayed }

// Resolved returns the resolved counter.
func (m *Metrics) Resolved() Resolved { return m.resolved }

// Poisoned returns the poisoned counter.
func (m *Metrics) Poisoned() Poisoned { return m.poisoned }
