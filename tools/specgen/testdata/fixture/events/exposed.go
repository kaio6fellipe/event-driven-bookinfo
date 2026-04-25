package events

type ThingCreatedPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Descriptor struct {
	Name        string
	ExposureKey string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Payload     any
	Description string
}

var Exposed = []Descriptor{
	{
		Name:        "thing-created",
		ExposureKey: "events",
		CEType:      "com.fixture.thing-created",
		CESource:    "fixture",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ThingCreatedPayload{},
		Description: "Emitted when a thing is created.",
	},
}
