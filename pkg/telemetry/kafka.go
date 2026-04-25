package telemetry

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// kafkaHeaderCarrier adapts a *kgo.Record's Headers slice to TextMapCarrier
// for W3C trace context propagation.
type kafkaHeaderCarrier struct {
	record *kgo.Record
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range c.record.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	for i := range c.record.Headers {
		if c.record.Headers[i].Key == key {
			c.record.Headers[i].Value = []byte(value)
			return
		}
	}
	c.record.Headers = append(c.record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.record.Headers))
	for _, h := range c.record.Headers {
		keys = append(keys, h.Key)
	}
	return keys
}

// InjectTraceContext writes W3C traceparent (and tracestate, when set) from
// ctx into the Kafka record headers via the registered TextMapPropagator.
// No-op when ctx carries no active span.
func InjectTraceContext(ctx context.Context, record *kgo.Record) {
	otel.GetTextMapPropagator().Inject(ctx, &kafkaHeaderCarrier{record: record})
}

var _ propagation.TextMapCarrier = (*kafkaHeaderCarrier)(nil)

// StartProducerSpan starts a child span for a Kafka publish operation following
// OTel messaging semantic conventions. The caller must End() the span (use defer).
// Span name is "<topic> publish"; SpanKind is Producer.
func StartProducerSpan(ctx context.Context, topic, key string) (context.Context, trace.Span) {
	return otel.Tracer("kafka-producer").Start(ctx,
		topic+" publish",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination.name", topic),
			attribute.String("messaging.operation.type", "publish"),
			attribute.String("messaging.kafka.message.key", key),
		),
	)
}
