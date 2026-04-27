package openapi_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/openapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_FixtureMatchesGolden(t *testing.T) {
	fixtureDir, _ := filepath.Abs("../../testdata/fixture")
	endpoints, version, err := walker.LoadEndpoints(fixtureDir, "fixture/api")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	got, err := openapi.Build(openapi.Input{
		ServiceName: "fixture",
		Version:     version,
		Endpoints:   endpoints,
		Metadata: openapi.SpecMetadata{
			OrgName:     "bookinfo-team",
			OrgURL:      "https://github.com/kaio6fellipe/event-driven-bookinfo",
			OrgEmail:    "noreply@bookinfo.local",
			LicenseName: "Apache-2.0",
			LicenseURL:  "https://www.apache.org/licenses/LICENSE-2.0",
			OpenAPIServer: openapi.ServerEntry{
				URL:         "http://localhost:8080",
				Description: "Local k3d gateway",
			},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantBytes, err := os.ReadFile("testdata/golden.yaml")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if string(got) != string(wantBytes) {
		// Allow regeneration via UPDATE=1.
		if os.Getenv("UPDATE") == "1" {
			if err := os.WriteFile("testdata/golden.yaml", got, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Log("golden updated")
			return
		}
		t.Errorf("openapi mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, wantBytes)
	}
}
