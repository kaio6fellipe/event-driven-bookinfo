package telemetry

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
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
