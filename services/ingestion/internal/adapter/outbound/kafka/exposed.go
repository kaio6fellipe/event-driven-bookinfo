package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/ingestion/api/asyncapi.yaml and the
// events.exposed block in deploy/ingestion/values-generated.yaml.
//
// ExposureKey "raw-books-details" matches the existing chart key in
// deploy/ingestion/values-local.yaml — the EventSource bound to the
// raw_books_details Kafka topic. The CE type is a logical content type
// (book-added), but the Helm grouping key is the topic-derived name.
var Exposed = []events.Descriptor{
	{
		Name:        "book-added",
		ExposureKey: "raw-books-details",
		Topic:       "raw_books_details",
		CEType:      "com.bookinfo.ingestion.book-added",
		CESource:    "ingestion",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     BookEvent{},
		Description: "Emitted for every Book scraped from Open Library.",
	},
}
