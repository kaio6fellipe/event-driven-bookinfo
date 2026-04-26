package kafka

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer
// reads each Descriptor to build CE headers; tools/specgen reads the
// same slice to derive services/reviews/api/asyncapi.yaml and the
// events.exposed.events block in deploy/reviews/values-generated.yaml.
//
// Both descriptors share ExposureKey "events" — one EventSource
// publishes both CE types from the bookinfo_reviews_events topic. The
// generator emits a union list of CETypes under events.exposed.events.eventTypes.
var Exposed = []events.Descriptor{
	{
		Name:        "review-submitted",
		ExposureKey: "events",
		CEType:      "com.bookinfo.reviews.review-submitted",
		CESource:    "reviews",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ReviewSubmittedPayload{},
		Description: "Emitted after a successful SubmitReview call.",
	},
	{
		Name:        "review-deleted",
		ExposureKey: "events",
		CEType:      "com.bookinfo.reviews.review-deleted",
		CESource:    "reviews",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ReviewDeletedPayload{},
		Description: "Emitted after a successful DeleteReview call.",
	},
}
