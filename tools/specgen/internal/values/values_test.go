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
