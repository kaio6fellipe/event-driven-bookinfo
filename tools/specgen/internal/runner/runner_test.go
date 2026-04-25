package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/runner"
)

func TestDiscoverServices_FindsRatings(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "services/ratings")); err != nil {
		t.Skip("ratings service not present")
	}

	svcs, err := runner.DiscoverServices(repoRoot)
	if err != nil {
		t.Fatalf("DiscoverServices: %v", err)
	}

	found := false
	for _, s := range svcs {
		if s.Name == "ratings" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ratings service not discovered, got %v", svcs)
	}
}
