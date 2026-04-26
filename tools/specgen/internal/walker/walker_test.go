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
	if post.SuccessStatus != 200 {
		t.Errorf("endpoints[1].SuccessStatus = %d, want 200", post.SuccessStatus)
	}

	// First endpoint (GET): new fields are zero/nil.
	if get.OperationID != "" {
		t.Errorf("endpoints[0].OperationID = %q, want empty", get.OperationID)
	}
	if get.Description != "" {
		t.Errorf("endpoints[0].Description = %q, want empty", get.Description)
	}
	if get.Tags != nil {
		t.Errorf("endpoints[0].Tags = %v, want nil", get.Tags)
	}

	// Second endpoint (POST): explicit overrides.
	if post.OperationID != "createThing" {
		t.Errorf("endpoints[1].OperationID = %q, want createThing", post.OperationID)
	}
	if post.Description != "Creates a new thing record and emits a thing-created event." {
		t.Errorf("endpoints[1].Description = %q", post.Description)
	}
	wantTags := []string{"things", "v1"}
	if len(post.Tags) != 2 || post.Tags[0] != wantTags[0] || post.Tags[1] != wantTags[1] {
		t.Errorf("endpoints[1].Tags = %v, want %v", post.Tags, wantTags)
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
	if d.Name != "thing-created" {
		t.Errorf("Name = %q, want thing-created", d.Name)
	}
	if d.ExposureKey != "events" {
		t.Errorf("ExposureKey = %q, want events", d.ExposureKey)
	}
	if d.CEType != "com.fixture.thing-created" {
		t.Errorf("CEType = %q, want com.fixture.thing-created", d.CEType)
	}
	if d.CESource != "fixture" {
		t.Errorf("CESource = %q, want fixture", d.CESource)
	}
	if d.Version != "1.0" {
		t.Errorf("Version = %q, want 1.0", d.Version)
	}
	if d.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want application/json", d.ContentType)
	}
	if d.Description != "Emitted when a thing is created." {
		t.Errorf("Description = %q, want %q", d.Description, "Emitted when a thing is created.")
	}
	if d.PayloadType == nil || d.PayloadType.Obj().Name() != "ThingCreatedPayload" {
		t.Errorf("PayloadType = %v, want ThingCreatedPayload", d.PayloadType)
	}
}

func TestLoadConsumed_Fixture(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}

	consumed, err := walker.LoadConsumed(fixtureDir, "fixture/events")
	if err != nil {
		t.Fatalf("LoadConsumed: %v", err)
	}
	if len(consumed) != 1 {
		t.Fatalf("len = %d, want 1", len(consumed))
	}
	c := consumed[0]
	if c.Name != "thing-updated" {
		t.Errorf("Name = %q, want thing-updated", c.Name)
	}
	if c.SourceService != "other-service" {
		t.Errorf("SourceService = %q, want other-service", c.SourceService)
	}
	if c.SourceEventName != "thing-updated" {
		t.Errorf("SourceEventName = %q, want thing-updated", c.SourceEventName)
	}
	if c.CEType != "com.other.thing-updated" {
		t.Errorf("CEType = %q, want com.other.thing-updated", c.CEType)
	}
}

func TestLoadConsumed_AbsentSlice(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	consumed, err := walker.LoadConsumed(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadConsumed: %v", err)
	}
	if consumed != nil {
		t.Errorf("expected nil consumed (slice absent), got %v", consumed)
	}
}
