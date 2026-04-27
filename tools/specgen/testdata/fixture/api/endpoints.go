package api

const APIVersion = "0.1.0"

type Endpoint struct {
	Method        string
	Path          string
	Summary       string
	OperationID   string
	Description   string
	Tags          []string
	EventName     string
	SuccessStatus int
	Request       any
	Response      any
	Errors        []ErrorResponse
}

type ErrorResponse struct {
	Status int
	Type   any
}

var Endpoints = []Endpoint{
	{
		Method:   "GET",
		Path:     "/v1/things/{id}",
		Summary:  "Get a thing",
		Response: GetThingResponse{},
		Errors:   []ErrorResponse{{Status: 404, Type: ErrResp{}}},
	},
	{
		Method:        "POST",
		Path:          "/v1/things",
		Summary:       "Create a thing",
		OperationID:   "createThing",
		Description:   "Creates a new thing record and emits a thing-created event.",
		Tags:          []string{"things", "v1"},
		EventName:     "thing-created",
		SuccessStatus: 200,
		Request:       CreateThingRequest{},
		Response:      GetThingResponse{},
		Errors:        []ErrorResponse{{Status: 400, Type: ErrResp{}}},
	},
}
