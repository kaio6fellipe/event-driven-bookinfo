// Package natspub implements eventsmessaging.Publisher on top of NATS
// JetStream. NewProducer ensures the stream exists (idempotent) and
// publishes CloudEvents-binary messages with NATS headers and OTel
// trace-context propagation.
package natspub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/codes"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/logging"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

// JetStream is the subset of nats.JetStreamContext used by Producer.
// Defined locally so tests can inject a stub via NewProducerWithJetStream
// without spinning up a real JetStream server for every error-path test.
type JetStream interface {
	PublishMsg(*nats.Msg, ...nats.PubOpt) (*nats.PubAck, error)
}

// Producer publishes CloudEvents-binary messages to a JetStream stream.
type Producer struct {
	nc      *nats.Conn
	js      JetStream
	subject string
}

// Compile-time interface compliance check.
var _ eventsmessaging.Publisher = (*Producer)(nil)

// NewProducer connects to NATS, ensures the JetStream stream exists, and
// returns a Publisher bound to the configured subject.
//
// streamName is the JetStream stream name. subject is the publish target;
// for current usage they are the same string (e.g. "raw_books_details").
// token is optional — empty string skips token auth (local-dev mode).
// When ctx carries a deadline, that remaining duration is used as the
// NATS connection timeout so the parameter has real semantics.
func NewProducer(ctx context.Context, url, token, streamName, subject string) (*Producer, error) {
	opts := []nats.Option{nats.Name("event-driven-bookinfo")}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining > 0 {
			opts = append(opts, nats.Timeout(remaining))
		}
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS at %s: %w", url, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("acquiring JetStream context: %w", err)
	}
	if err := ensureStream(ctx, js, streamName, subject); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensuring stream %q: %w", streamName, err)
	}
	return &Producer{nc: nc, js: js, subject: subject}, nil
}

// NewProducerWithJetStream wires Producer with an injected JetStream
// (for tests). nc may be nil; Close() guards against it.
func NewProducerWithJetStream(js JetStream, subject string) *Producer {
	return &Producer{js: js, subject: subject}
}

// Publish marshals payload to JSON and emits a NATS JetStream message
// with CloudEvents-binary headers + OTel traceparent.
func (p *Producer) Publish(ctx context.Context, d events.Descriptor, payload any, recordKey, idempotencyKey string) error {
	logger := logging.FromContext(ctx)

	value, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling %s event: %w", d.Name, err)
	}

	now := time.Now().UTC()
	hdr := nats.Header{}
	hdr.Set("ce-specversion", d.Version)
	hdr.Set("ce-type", d.CEType)
	hdr.Set("ce-source", d.CESource)
	hdr.Set("ce-id", uuid.New().String())
	hdr.Set("ce-time", now.Format(time.RFC3339))
	hdr.Set("ce-subject", recordKey)
	hdr.Set("content-type", d.ContentType)

	msg := &nats.Msg{
		Subject: p.subject,
		Data:    value,
		Header:  hdr,
	}

	ctx, span := telemetry.StartNATSProducerSpan(ctx, p.subject, idempotencyKey)
	defer span.End()
	telemetry.InjectTraceContextNATS(ctx, msg)

	if _, err := p.js.PublishMsg(msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publishing to JetStream subject %q: %w", p.subject, err)
	}

	logger.Debug("published event",
		"subject", p.subject,
		"ce_type", d.CEType,
		"idempotency_key", idempotencyKey,
	)
	return nil
}

// Close drains and closes the NATS connection.
func (p *Producer) Close() {
	if p.nc != nil {
		_ = p.nc.Drain()
	}
}

func ensureStream(ctx context.Context, js nats.JetStreamContext, name, subject string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before AddStream: %w", err)
	}
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     name,
		Subjects: []string{subject},
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil
	}
	return err
}
