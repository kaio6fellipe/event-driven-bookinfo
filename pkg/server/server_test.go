package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/server"
)

func getFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("%d", port)
}

func TestServer_BothPortsStart(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-server",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"pong": "true"})
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, nil)
	}()

	// Wait for servers to start
	waitForServer(t, httpPort)
	waitForServer(t, adminPort)

	// Test API port
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/v1/ping", httpPort))
	if err != nil {
		t.Fatalf("failed to reach API port: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("API status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["pong"] != "true" {
		t.Errorf("pong = %q, want %q", body["pong"], "true")
	}

	// Test admin healthz
	resp2, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", adminPort))
	if err != nil {
		t.Fatalf("failed to reach admin port: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("admin /healthz status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	var healthBody map[string]string
	_ = json.NewDecoder(resp2.Body).Decode(&healthBody)
	if healthBody["status"] != "ok" {
		t.Errorf("healthz status = %q, want %q", healthBody["status"], "ok")
	}

	// Test admin readyz
	resp3, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/readyz", adminPort))
	if err != nil {
		t.Fatalf("failed to reach admin readyz: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()

	if resp3.StatusCode != http.StatusOK {
		t.Errorf("admin /readyz status = %d, want %d", resp3.StatusCode, http.StatusOK)
	}

	// Shut down
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-shutdown",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/slow", func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, nil)
	}()

	waitForServer(t, httpPort)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestServer_AdminMetricsEndpoint(t *testing.T) {
	httpPort := getFreePort(t)
	adminPort := getFreePort(t)

	cfg := &config.Config{
		ServiceName: "test-metrics-endpoint",
		HTTPPort:    httpPort,
		AdminPort:   adminPort,
		LogLevel:    "error",
	}

	registerRoutes := func(_ *http.ServeMux) {}

	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# HELP test_metric A test metric\n# TYPE test_metric counter\ntest_metric 42\n"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, registerRoutes, metricsHandler)
	}()

	waitForServer(t, adminPort)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/metrics", adminPort))
	if err != nil {
		t.Fatalf("failed to reach /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	if body == "" {
		t.Error("expected non-empty /metrics response")
	}

	cancel()
	<-errCh
}

func waitForServer(t *testing.T, port string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server on port %s did not start in time", port)
}
