package values_test

import (
	"strings"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/values"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

func TestBuild_GeneratesCqrsAndEventsBlocks(t *testing.T) {
	got, err := values.Build(values.Input{
		ServiceName: "details",
		Endpoints: []walker.EndpointInfo{
			{Method: "POST", Path: "/v1/details", EventName: "book-added"},
			{Method: "GET", Path: "/v1/details/{id}"},
		},
		Exposed: []walker.DescriptorInfo{
			{Name: "book-added", ExposureKey: "events", CEType: "com.bookinfo.details.book-added", ContentType: "application/json"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := string(got)
	for _, want := range []string{
		"DO NOT EDIT",
		"cqrs:\n  endpoints:\n    book-added:\n      method: POST\n      endpoint: /v1/details",
		"events:\n  exposed:\n    events:\n      contentType: application/json",
		"- com.bookinfo.details.book-added",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	if strings.Contains(out, "/v1/details/{id}") {
		t.Error("GET endpoint must NOT appear in cqrs.endpoints (no EventName)")
	}
}

func TestBuild_OmitsEmptyTopLevelKeys(t *testing.T) {
	// All-GET endpoints (no EventName) and no Exposed events:
	// neither top-level cqrs nor events should appear in the output.
	got, err := values.Build(values.Input{
		ServiceName: "productpage",
		Endpoints: []walker.EndpointInfo{
			{Method: "GET", Path: "/v1/products"},
			{Method: "GET", Path: "/v1/products/{id}"},
		},
		Exposed: nil,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := string(got)
	if strings.Contains(out, "cqrs:") {
		t.Errorf("expected no cqrs: key when no endpoint has EventName, got:\n%s", out)
	}
	if strings.Contains(out, "events:") {
		t.Errorf("expected no events: key when Exposed is empty, got:\n%s", out)
	}
	// Header comment must still be there.
	if !strings.Contains(out, "DO NOT EDIT") {
		t.Errorf("header comment missing, got:\n%s", out)
	}
}

func TestBuild_MultipleDescriptorsShareExposureKey(t *testing.T) {
	// Two descriptors with the same ExposureKey produce ONE events.exposed
	// entry whose eventTypes array unions both CETypes alphabetically.
	got, err := values.Build(values.Input{
		ServiceName: "details",
		Exposed: []walker.DescriptorInfo{
			// Note the input order is reversed alphabetically; output must be sorted.
			{Name: "book-updated", ExposureKey: "events", CEType: "com.bookinfo.details.book-updated", ContentType: "application/json"},
			{Name: "book-added", ExposureKey: "events", CEType: "com.bookinfo.details.book-added", ContentType: "application/json"},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := string(got)

	// Single events.exposed.events block.
	if strings.Count(out, "    events:\n") != 1 {
		t.Errorf("expected exactly one events.exposed.events block, got:\n%s", out)
	}

	// Both CETypes appear, alphabetically (book-added < book-updated).
	addedIdx := strings.Index(out, "com.bookinfo.details.book-added")
	updatedIdx := strings.Index(out, "com.bookinfo.details.book-updated")
	if addedIdx < 0 || updatedIdx < 0 {
		t.Fatalf("missing CE types in output:\n%s", out)
	}
	if addedIdx >= updatedIdx {
		t.Errorf("expected book-added before book-updated alphabetically, got positions %d/%d in:\n%s", addedIdx, updatedIdx, out)
	}
}
