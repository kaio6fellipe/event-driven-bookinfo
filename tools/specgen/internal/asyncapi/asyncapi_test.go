package asyncapi_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/asyncapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_FixtureMatchesGolden(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	exposed, err := walker.LoadExposed(fixtureDir, "fixture/events")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}

	got, err := asyncapi.Build(asyncapi.Input{
		ServiceName: "fixture",
		Version:     "0.1.0",
		Exposed:     exposed,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantBytes, _ := os.ReadFile("testdata/golden.yaml")
	if string(got) != string(wantBytes) {
		if os.Getenv("UPDATE") == "1" {
			if err := os.WriteFile("testdata/golden.yaml", got, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		t.Errorf("asyncapi mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, wantBytes)
	}
}
