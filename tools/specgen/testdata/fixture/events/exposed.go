package events

type ThingCreatedPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Descriptor struct {
	Name        string
	ExposureKey string
	Topic       string
	CEType      string
	CESource    string
	Version     string
	ContentType string
	Payload     any
	Description string
}

type ConsumedDescriptor struct {
	Name            string
	SourceService   string
	SourceEventName string
	CEType          string
	Description     string
}

var Consumed = []ConsumedDescriptor{
	{
		Name:            "thing-updated",
		SourceService:   "other-service",
		SourceEventName: "thing-updated",
		CEType:          "com.other.thing-updated",
		Description:     "Reacts to upstream updates.",
	},
}

var Exposed = []Descriptor{
	{
		Name:        "thing-created",
		ExposureKey: "events",
		Topic:       "fixture_things_events",
		CEType:      "com.fixture.thing-created",
		CESource:    "fixture",
		Version:     "1.0",
		ContentType: "application/json",
		Payload:     ThingCreatedPayload{},
		Description: "Emitted when a thing is created.",
	},
}
