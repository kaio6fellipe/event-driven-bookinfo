package http_test

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	replay "github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/http"
)

func TestReplay_SuccessForwardsHeadersAndBody(t *testing.T) {
	var gotBody string
	var gotTP string
	ts := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotTP = r.Header.Get("traceparent")
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer ts.Close()

	client := replay.NewReplayClient(ts.Client())
	err := client.Replay(context.Background(), ts.URL,
		[]byte(`{"hello":"world"}`),
		map[string][]string{"traceparent": {"00-traceid"}})
	if err != nil {
		t.Fatalf("Replay err = %v", err)
	}
	if gotBody != `{"hello":"world"}` {
		t.Errorf("body = %q", gotBody)
	}
	if gotTP != "00-traceid" {
		t.Errorf("traceparent = %q", gotTP)
	}
}

func TestReplay_Non2xxReturnsError(t *testing.T) {
	ts := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusInternalServerError)
	}))
	defer ts.Close()

	client := replay.NewReplayClient(ts.Client())
	err := client.Replay(context.Background(), ts.URL, []byte(`{}`), nil)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}
