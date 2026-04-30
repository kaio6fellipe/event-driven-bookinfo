package natspub_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/eventsmessaging/natspub"
)

func runJetStreamServer(t *testing.T) string {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1 // random port
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natstest.RunServer(&opts)
	t.Cleanup(s.Shutdown)
	return s.ClientURL()
}

func TestProducer_PublishCreatesStreamAndDelivers(t *testing.T) {
	url := runJetStreamServer(t)

	d := events.Descriptor{
		Name:        "book-added",
		Topic:       "raw_books_details",
		CEType:      "com.bookinfo.ingestion.book-added",
		CESource:    "ingestion",
		Version:     "1.0",
		ContentType: "application/json",
	}

	p, err := natspub.NewProducer(context.Background(), url, "", "" /*no auth*/, d.Topic, d.Topic)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	defer p.Close()

	if err := p.Publish(context.Background(), d, map[string]string{"hello": "world"}, "key-1", "idem-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Verify a message landed on the subject.
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	sub, err := js.SubscribeSync(d.Topic, nats.OrderedConsumer())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("next msg: %v", err)
	}
	if got := msg.Header.Get("ce-type"); got != d.CEType {
		t.Errorf("ce-type header = %q, want %q", got, d.CEType)
	}
	if got := msg.Header.Get("ce-source"); got != d.CESource {
		t.Errorf("ce-source header = %q, want %q", got, d.CESource)
	}
	if got := msg.Header.Get("ce-subject"); got != "key-1" {
		t.Errorf("ce-subject header = %q, want %q", got, "key-1")
	}
}

func TestProducer_StreamEnsureIdempotent(t *testing.T) {
	url := runJetStreamServer(t)

	for i := 0; i < 2; i++ {
		p, err := natspub.NewProducer(context.Background(), url, "", "", "raw_books_details", "raw_books_details")
		if err != nil {
			t.Fatalf("new producer iter %d: %v", i, err)
		}
		p.Close()
	}
}

// TestProducer_Publish_EmptyRecordKey verifies that an empty recordKey still
// publishes successfully and sets an empty ce-subject header (mirroring
// kafkapub's behaviour where the empty string is used as-is for ce_subject).
func TestProducer_Publish_EmptyRecordKey(t *testing.T) {
	url := runJetStreamServer(t)

	d := events.Descriptor{
		Name:        "book-added",
		Topic:       "raw_books_details",
		CEType:      "com.bookinfo.ingestion.book-added",
		CESource:    "ingestion",
		Version:     "1.0",
		ContentType: "application/json",
	}

	p, err := natspub.NewProducer(context.Background(), url, "", "", d.Topic, d.Topic)
	if err != nil {
		t.Fatalf("new producer: %v", err)
	}
	defer p.Close()

	// Empty recordKey — should not error.
	if err := p.Publish(context.Background(), d, map[string]string{"k": "v"}, "", "idem-empty"); err != nil {
		t.Fatalf("publish with empty recordKey: %v", err)
	}

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	sub, err := js.SubscribeSync(d.Topic, nats.OrderedConsumer())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("next msg: %v", err)
	}
	// ce-subject should be the empty string.
	if got := msg.Header.Get("ce-subject"); got != "" {
		t.Errorf("ce-subject header = %q, want empty string", got)
	}
}

func TestProducer_Publish_WrapsProduceError(t *testing.T) {
	fake := &errJetStream{err: errors.New("boom")}
	p := natspub.NewProducerWithJetStream(fake, "subject.test")

	d := events.Descriptor{Name: "x", CEType: "ce.x", CESource: "src", Version: "1.0", ContentType: "application/json"}
	err := p.Publish(context.Background(), d, struct{}{}, "k", "ik")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "publishing to JetStream subject \"subject.test\":") {
		t.Errorf("error not wrapped with subject context: %v", err)
	}
	if !errors.Is(err, fake.err) {
		t.Errorf("error chain does not contain sentinel: %v", err)
	}
}

type errJetStream struct {
	err error
}

func (e *errJetStream) PublishMsg(*nats.Msg, ...nats.PubOpt) (*nats.PubAck, error) {
	return nil, e.err
}
