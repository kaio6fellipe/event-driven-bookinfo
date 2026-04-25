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
