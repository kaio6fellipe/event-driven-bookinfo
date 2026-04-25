package walker_test

import (
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestLoadEndpoints_Fixture(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}

	endpoints, version, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	if version != "0.1.0" {
		t.Errorf("APIVersion = %q, want %q", version, "0.1.0")
	}
	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2", len(endpoints))
	}

	get := endpoints[0]
	if get.Method != "GET" || get.Path != "/v1/things/{id}" {
		t.Errorf("endpoints[0] = {%q, %q}, want {GET, /v1/things/{id}}", get.Method, get.Path)
	}
	if get.Summary != "Get a thing" {
		t.Errorf("endpoints[0].Summary = %q", get.Summary)
	}
	if get.ResponseType == nil || get.ResponseType.Obj().Name() != "GetThingResponse" {
		t.Errorf("endpoints[0].ResponseType = %v, want GetThingResponse", get.ResponseType)
	}
	if len(get.Errors) != 1 || get.Errors[0].Status != 404 {
		t.Errorf("endpoints[0].Errors = %v, want [{404, ...}]", get.Errors)
	}

	post := endpoints[1]
	if post.EventName != "thing-created" {
		t.Errorf("endpoints[1].EventName = %q, want thing-created", post.EventName)
	}
	if post.RequestType == nil || post.RequestType.Obj().Name() != "CreateThingRequest" {
		t.Errorf("endpoints[1].RequestType = %v, want CreateThingRequest", post.RequestType)
	}
}

func TestLoadExposed_Fixture(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}

	exposed, err := walker.LoadExposed(fixtureDir, "fixture/events")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}
	if len(exposed) != 1 {
		t.Fatalf("len(exposed) = %d, want 1", len(exposed))
	}

	d := exposed[0]
	if d.Name != "thing-created" || d.ExposureKey != "events" {
		t.Errorf("Name=%q ExposureKey=%q, want thing-created/events", d.Name, d.ExposureKey)
	}
	if d.CEType != "com.fixture.thing-created" {
		t.Errorf("CEType = %q", d.CEType)
	}
	if d.PayloadType == nil || d.PayloadType.Obj().Name() != "ThingCreatedPayload" {
		t.Errorf("PayloadType = %v, want ThingCreatedPayload", d.PayloadType)
	}
}
