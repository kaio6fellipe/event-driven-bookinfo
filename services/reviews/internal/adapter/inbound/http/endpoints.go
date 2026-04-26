package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// DeleteReviewRequest is the JSON body for POST /v1/reviews/delete.
// Hoisted out of the handler's anonymous struct so tools/specgen can
// resolve it as a JSONSchema.
type DeleteReviewRequest struct {
	ReviewID string `json:"review_id"`
}

// Endpoints declares every HTTP route this service exposes.
var Endpoints = []api.Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/reviews/{id}",
		Summary:  "Get reviews for a product (paginated)",
		Response: ProductReviewsResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
	{
		Method:    "POST",
		Path:      "/v1/reviews",
		Summary:   "Submit a new review",
		EventName: "review-submitted",
		Request:   SubmitReviewRequest{},
		Response:  ReviewResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
	{
		Method:        "POST",
		Path:          "/v1/reviews/delete",
		Summary:       "Delete a review by ID (Sensor-routed command)",
		EventName:     "review-deleted",
		SuccessStatus: 204,
		Request:       DeleteReviewRequest{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
