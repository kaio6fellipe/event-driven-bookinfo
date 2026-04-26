package jsonschema_test

import (
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/jsonschema"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestSchemaFromType_PrimitiveFields(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatal(err)
	}
	endpoints, _, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	// endpoints[1] is the POST: Request = CreateThingRequest{Name string}
	schema, err := jsonschema.SchemaFromType(endpoints[1].RequestType)
	if err != nil {
		t.Fatalf("SchemaFromType: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("schema.Type = %q, want object", schema.Type)
	}
	if _, ok := schema.Properties["name"]; !ok {
		t.Errorf("expected property %q in %v", "name", schema.Properties)
	}
	if got := schema.Properties["name"].Type; got != "string" {
		t.Errorf("name.Type = %q, want string", got)
	}
}

func TestSchemaFromType_TimeTime(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	endpoints, _, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	// endpoints[0] is GET; Response = GetThingResponse with CreatedAt time.Time
	schema, err := jsonschema.SchemaFromType(endpoints[0].ResponseType)
	if err != nil {
		t.Fatalf("SchemaFromType: %v", err)
	}

	createdAt, ok := schema.Properties["created_at"]
	if !ok {
		t.Fatalf("expected created_at property, got %v", schema.Properties)
	}
	if createdAt.Type != "string" || createdAt.Format != "date-time" {
		t.Errorf("created_at = {Type: %q, Format: %q}, want {string, date-time}", createdAt.Type, createdAt.Format)
	}

	// Pointer-to-time.Time also gets the same treatment.
	deletedAt, ok := schema.Properties["deleted_at"]
	if !ok {
		t.Fatalf("expected deleted_at property")
	}
	if deletedAt.Type != "string" || deletedAt.Format != "date-time" {
		t.Errorf("deleted_at = {Type: %q, Format: %q}, want {string, date-time}", deletedAt.Type, deletedAt.Format)
	}
}
