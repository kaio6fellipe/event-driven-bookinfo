package walker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

const realModulePath = "github.com/kaio6fellipe/event-driven-bookinfo"

// realRepoRoot returns the absolute path of the parent module's root,
// or fails the test if it can't be located. Walker integration tests
// load real services from this root.
func realRepoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p, "go.mod")); err != nil {
		t.Fatalf("not in expected repo layout (no go.mod at %s): %v", p, err)
	}
	return p
}

// TestLoadEndpoints_RealRatingsService catches field renames in
// pkg/api.Endpoint that the synthetic fixture would silently miss.
// The walker matches by AST field name; if a future PR renames
// EventName → CommandName in pkg/api.Endpoint, the real service's
// endpoints.go won't compile (fixed by the same PR), but the walker's
// extraction would be silently broken if the fixture mirror missed
// the rename. This test loads the real service and asserts on
// known-good extracted fields.
func TestLoadEndpoints_RealRatingsService(t *testing.T) {
	repoRoot := realRepoRoot(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "services", "ratings", "internal", "adapter", "inbound", "http", "endpoints.go")); err != nil {
		t.Skip("services/ratings/internal/adapter/inbound/http/endpoints.go not present")
	}

	endpoints, version, err := walker.LoadEndpoints(repoRoot,
		realModulePath+"/services/ratings/internal/adapter/inbound/http")
	if err != nil {
		t.Fatalf("LoadEndpoints: %v", err)
	}

	if version == "" {
		t.Error("expected APIVersion to be a non-empty string")
	}
	if len(endpoints) < 2 {
		t.Fatalf("expected ≥2 endpoints, got %d", len(endpoints))
	}

	// Find the POST /v1/ratings endpoint and assert on its known fields.
	var post walker.EndpointInfo
	for _, ep := range endpoints {
		if ep.Method == "POST" && ep.Path == "/v1/ratings" {
			post = ep
			break
		}
	}
	if post.Method != "POST" {
		t.Fatal("POST /v1/ratings not found in the endpoints slice")
	}
	if post.EventName != "rating-submitted" {
		t.Errorf("EventName = %q, want %q", post.EventName, "rating-submitted")
	}
	if post.RequestType == nil {
		t.Error("RequestType expected non-nil for POST /v1/ratings")
	}
	if post.ResponseType == nil {
		t.Error("ResponseType expected non-nil for POST /v1/ratings")
	}
	if post.Summary == "" {
		t.Error("Summary expected non-empty")
	}

	// Find the GET /v1/ratings/{id} endpoint.
	var get walker.EndpointInfo
	for _, ep := range endpoints {
		if ep.Method == "GET" && ep.Path == "/v1/ratings/{id}" {
			get = ep
			break
		}
	}
	if get.Method != "GET" {
		t.Fatal("GET /v1/ratings/{id} not found in the endpoints slice")
	}
	if get.RequestType != nil {
		t.Errorf("RequestType expected nil for GET, got %v", get.RequestType)
	}
	if get.ResponseType == nil {
		t.Error("ResponseType expected non-nil for GET /v1/ratings/{id}")
	}
}

// TestLoadExposed_RealRatingsService catches field renames in
// pkg/events.Descriptor for the same reasons as the endpoints variant.
func TestLoadExposed_RealRatingsService(t *testing.T) {
	repoRoot := realRepoRoot(t)
	if _, err := os.Stat(filepath.Join(repoRoot, "services", "ratings", "internal", "adapter", "outbound", "kafka", "exposed.go")); err != nil {
		t.Skip("services/ratings/internal/adapter/outbound/kafka/exposed.go not present")
	}

	exposed, err := walker.LoadExposed(repoRoot,
		realModulePath+"/services/ratings/internal/adapter/outbound/kafka")
	if err != nil {
		t.Fatalf("LoadExposed: %v", err)
	}
	if len(exposed) < 1 {
		t.Fatal("expected ≥1 exposed descriptor")
	}

	d := exposed[0]
	if d.Name != "rating-submitted" {
		t.Errorf("Name = %q, want %q", d.Name, "rating-submitted")
	}
	if d.CEType != "com.bookinfo.ratings.rating-submitted" {
		t.Errorf("CEType = %q, want %q", d.CEType, "com.bookinfo.ratings.rating-submitted")
	}
	if d.CESource != "ratings" {
		t.Errorf("CESource = %q, want %q", d.CESource, "ratings")
	}
	if d.PayloadType == nil {
		t.Error("PayloadType expected non-nil")
	}
}
