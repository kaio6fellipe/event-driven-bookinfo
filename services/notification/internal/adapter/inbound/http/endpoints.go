package http //nolint:revive // package name matches directory convention

import "github.com/kaio6fellipe/event-driven-bookinfo/pkg/api"

// APIVersion is the OpenAPI info.version emitted by tools/specgen.
const APIVersion = "1.0.0"

// Endpoints declares every HTTP route this service exposes. The handler's
// RegisterRoutes loops over this slice; tools/specgen reads it to derive
// services/notification/api/openapi.yaml.
//
// POST /v1/notifications has no EventName because notification is not
// behind the CQRS gateway split; sensors invoke this endpoint directly
// via events.consumed in deploy/notification/values-local.yaml.
var Endpoints = []api.Endpoint{
	{
		Method:   "POST",
		Path:     "/v1/notifications",
		Summary:  "Dispatch a notification (called by Argo Events sensors)",
		Request:  DispatchNotificationRequest{},
		Response: NotificationResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/notifications/{id}",
		Summary:  "Get a single notification by ID",
		Response: NotificationResponse{},
		Errors: []api.ErrorResponse{
			{Status: 404, Type: ErrorResponse{}},
		},
	},
	{
		Method:   "GET",
		Path:     "/v1/notifications",
		Summary:  "List notifications for a recipient (?recipient=<email>)",
		Response: NotificationsListResponse{},
		Errors: []api.ErrorResponse{
			{Status: 400, Type: ErrorResponse{}},
			{Status: 500, Type: ErrorResponse{}},
		},
	},
}
