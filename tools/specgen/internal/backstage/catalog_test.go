package backstage_test

import (
	"os"
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
