package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/events"

// Consumed declares every CloudEvent type notification subscribes to.
// notification fans out alerts triggered by domain events from
// details, reviews, and ratings.
var Consumed = []events.ConsumedDescriptor{
	{
		Name:            "book-added",
		SourceService:   "details",
		SourceEventName: "book-added",
		CEType:          "com.bookinfo.details.book-added",
		Description:     "Notify on new book additions.",
	},
	{
		Name:            "review-submitted",
		SourceService:   "reviews",
		SourceEventName: "review-submitted",
		CEType:          "com.bookinfo.reviews.review-submitted",
		Description:     "Notify on new review submissions.",
	},
	{
		Name:            "review-deleted",
		SourceService:   "reviews",
		SourceEventName: "review-deleted",
		CEType:          "com.bookinfo.reviews.review-deleted",
		Description:     "Notify on review deletions.",
	},
	{
		Name:            "rating-submitted",
		SourceService:   "ratings",
		SourceEventName: "rating-submitted",
		CEType:          "com.bookinfo.ratings.rating-submitted",
		Description:     "Notify on new rating submissions.",
	},
}
