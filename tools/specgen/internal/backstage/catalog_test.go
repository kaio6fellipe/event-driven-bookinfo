package backstage_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/backstage"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_BothApisMatchesGolden(t *testing.T) {
	got, err := backstage.Build(backstage.Input{
		ServiceName:   "ratings",
		HasOpenAPI:    true,
		HasAsyncAPI:   true,
		ExposedNames:  []string{"rating-submitted"},
		Owner:         "bookinfo-team",
		RepoTreeURL:   "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/ratings",
		ExposedSubset: []walker.DescriptorInfo{{Name: "rating-submitted"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want, _ := os.ReadFile("testdata/golden.yaml")
	if string(got) != string(want) {
		if os.Getenv("UPDATE") == "1" {
			_ = os.WriteFile("testdata/golden.yaml", got, 0o644)
			return
		}
		t.Errorf("catalog mismatch — set UPDATE=1 to regenerate.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuild_OpenAPIOnly(t *testing.T) {
	got, err := backstage.Build(backstage.Input{
		ServiceName: "ratings",
		HasOpenAPI:  true,
		HasAsyncAPI: false,
		Owner:       "bookinfo-team",
		RepoTreeURL: "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/ratings",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "ratings-rest") {
		t.Errorf("expected ratings-rest entity, got:\n%s", out)
	}
	if strings.Contains(out, "ratings-events") {
		t.Errorf("did not expect ratings-events entity in OpenAPI-only build")
	}
}

func TestBuild_AsyncAPIOnly(t *testing.T) {
	got, err := backstage.Build(backstage.Input{
		ServiceName:  "ingestion",
		HasOpenAPI:   false,
		HasAsyncAPI:  true,
		ExposedNames: []string{"raw-books-details"},
		Owner:        "bookinfo-team",
		RepoTreeURL:  "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/ingestion",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out := string(got)
	if strings.Contains(out, "ingestion-rest") {
		t.Errorf("did not expect ingestion-rest entity in AsyncAPI-only build")
	}
	if !strings.Contains(out, "ingestion-events") {
		t.Errorf("expected ingestion-events entity, got:\n%s", out)
	}
	if !strings.Contains(out, "raw-books-details") {
		t.Errorf("expected exposed-event-names annotation, got:\n%s", out)
	}
}

func TestBuild_RejectsMissingOwner(t *testing.T) {
	_, err := backstage.Build(backstage.Input{
		ServiceName: "ratings",
		HasOpenAPI:  true,
		RepoTreeURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error for missing Owner")
	}
}

func TestBuild_RejectsMissingRepoTreeURL(t *testing.T) {
	_, err := backstage.Build(backstage.Input{
		ServiceName: "ratings",
		HasOpenAPI:  true,
		Owner:       "bookinfo-team",
	})
	if err == nil {
		t.Fatal("expected error for missing RepoTreeURL")
	}
}
