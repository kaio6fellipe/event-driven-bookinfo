package messaging

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Exposed declares every event this service publishes. The producer reads
// each Descriptor to build CE headers; tools/specgen reads the same slice
// to derive services/ratings/api/asyncapi.yaml and the events.exposed
// block in deploy/ratings/values-generated.yaml.
var Exposed = []events.Descriptor{
	{
		Name:        "rating-submitted",
		ExposureKey: "events",
		Topic:       "bookinfo_ratings_events",
		CEType:      "com.bookinfo.ratings.rating-submitted",
		CESource:    "ratings",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     RatingSubmittedPayload{},
		Description: "Emitted after a successful SubmitRating call.",
	},
}
