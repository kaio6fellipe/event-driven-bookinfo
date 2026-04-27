package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/details/api/asyncapi.yaml and the
// events.exposed block in deploy/details/values-generated.yaml.
//
// ExposureKey "events" groups all descriptors under one EventSource —
// generated as deploy/details/values-generated.yaml's
// events.exposed.events block, publishing all details domain events
// from the bookinfo_details_events topic.
var Exposed = []events.Descriptor{
	{
		Name:        "book-added",
		ExposureKey: "events",
		Topic:       "bookinfo_details_events",
		CEType:      "com.bookinfo.details.book-added",
		CESource:    "details",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     BookAddedPayload{},
		Description: "Emitted after a successful AddDetail call.",
	},
}
