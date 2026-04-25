package api

const APIVersion = "0.1.0"

type Endpoint struct {
	Method    string
	Path      string
	Summary   string
	EventName string
	Request   any
	Response  any
	Errors    []ErrorResponse
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
		Method:    "POST",
		Path:      "/v1/things",
		Summary:   "Create a thing",
		EventName: "thing-created",
		Request:   CreateThingRequest{},
		Response:  GetThingResponse{},
		Errors:    []ErrorResponse{{Status: 400, Type: ErrResp{}}},
	},
}
