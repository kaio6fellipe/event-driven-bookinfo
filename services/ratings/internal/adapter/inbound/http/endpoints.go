package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/ratings/api/openapi.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/ratings/{id}",
		Summary:  "Get all ratings for a product",
		Response: ProductRatingsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/ratings",
		Summary:   "Submit a new rating",
		EventName: "rating-submitted",
		Request:   SubmitRatingRequest{},
		Response:  RatingResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
}
