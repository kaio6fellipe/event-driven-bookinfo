package telemetry

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// natsHeaderCarrier adapts nats.Header to propagation.TextMapCarrier for
// W3C trace context propagation over JetStream message headers.
type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string { return nats.Header(c).Get(key) }
func (c natsHeaderCarrier) Set(key, val string)   { nats.Header(c).Set(key, val) }
func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// StartNATSProducerSpan opens a producer span around a JetStream publish
// following OTel messaging semantic conventions. The caller must call
// span.End() when the publish completes (typically via defer).
func StartNATSProducerSpan(ctx context.Context, subject, idempotencyKey string) (context.Context, trace.Span) {
	tracer := otel.Tracer("eventsmessaging/natspub")
	return tracer.Start(ctx, "jetstream.publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "jetstream"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.message.idempotency_key", idempotencyKey),
		),
	)
}

// InjectTraceContextNATS writes the active span context from ctx into
// msg.Header using the configured global TextMapPropagator. No-op when
// ctx carries no active span. Initialises msg.Header if nil.
func InjectTraceContextNATS(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = nats.Header{}
	}
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))
}
