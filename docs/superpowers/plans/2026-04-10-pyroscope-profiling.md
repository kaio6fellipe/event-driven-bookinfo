# Pyroscope Continuous Profiling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy Pyroscope and enable trace-to-profile correlation so Grafana links Tempo traces directly to continuous profiles.

**Architecture:** Push-based profiling using existing `pyroscope-go` SDK. `grafana/otel-profiling-go` wraps the OTel TracerProvider to tag profiling samples with span IDs. Pyroscope runs as monolithic Helm chart in `observability` namespace. Grafana's Tempo datasource gets `tracesToProfilesV2` config to link traces to profiles.

**Tech Stack:** Go, grafana/otel-profiling-go, grafana/pyroscope-go, Helm (grafana/pyroscope chart), Kubernetes downward API, Grafana datasource provisioning

---

## File Structure

| File | Action | Responsibility |
| --- | --- | --- |
| `pkg/telemetry/telemetry.go` | Modify | Wrap TracerProvider with otelpyroscope when profiling enabled |
| `pkg/telemetry/telemetry_test.go` | Modify | Test new pyroscopeEnabled parameter |
| `pkg/profiling/profiling.go` | Modify | Add pod-level tags from env vars |
| `pkg/profiling/profiling_test.go` | Modify | Test tags behavior |
| `services/*/cmd/main.go` (x5) | Modify | Pass pyroscopeEnabled to telemetry.Setup |
| `deploy/observability/local/pyroscope-values.yaml` | Create | Helm values for Pyroscope server |
| `deploy/observability/local/kube-prometheus-stack-values.yaml` | Modify | Add Pyroscope datasource + tracesToProfiles on Tempo |
| `Makefile` | Modify | Add Pyroscope install step in k8s-observability |
| `deploy/*/overlays/local/configmap-patch.yaml` (x5) | Modify | Add PYROSCOPE_SERVER_ADDRESS |
| `deploy/*/base/deployment.yaml` (x5) | Modify | Remove pull annotations, add downward API env vars |
| `deploy/*/overlays/local/deployment-write.yaml` (x3) | Modify | Remove pull annotations, add downward API env vars |
| `CLAUDE.md` | Modify | Update observability description |
| `README.md` | Modify | Update mermaid diagram, profiling section, stack references |

---

### Task 1: Add otel-profiling-go dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run:

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
go get github.com/grafana/otel-profiling-go
```

Expected: `go.mod` gains `github.com/grafana/otel-profiling-go` in the require block.

- [ ] **Step 2: Verify the dependency resolves**

Run:

```bash
go mod tidy
```

Expected: Clean exit, no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(pkg/telemetry): add grafana/otel-profiling-go dependency"
```

---

### Task 2: Update pkg/telemetry with otelpyroscope wrapper

**Files:**
- Modify: `pkg/telemetry/telemetry.go`
- Modify: `pkg/telemetry/telemetry_test.go`

- [ ] **Step 1: Write failing test for new pyroscopeEnabled parameter**

Replace the contents of `pkg/telemetry/telemetry_test.go` with:

```go
package telemetry_test

import (
	"context"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/telemetry"
)

func TestSetup_NoOpWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := telemetry.Setup(context.Background(), "test-service", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func should not be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}

func TestSetup_NoOpWhenEndpointUnsetWithPyroscope(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := telemetry.Setup(context.Background(), "test-service", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func should not be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown returned error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./pkg/telemetry/ -v -count=1
```

Expected: Compilation error — `telemetry.Setup` currently takes 2 args, tests pass 3.

- [ ] **Step 3: Implement the otelpyroscope wrapper**

Replace the contents of `pkg/telemetry/telemetry.go` with:

```go
// Package telemetry configures OpenTelemetry tracing with an OTLP exporter.
package telemetry

import (
	"context"
	"fmt"
	"os"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup initializes OpenTelemetry tracing with an OTLP gRPC exporter when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. When pyroscopeEnabled is true, the
// TracerProvider is wrapped with otelpyroscope to tag profiling samples with
// span IDs for trace-to-profile correlation. Returns a shutdown function.
func Setup(ctx context.Context, serviceName string, pyroscopeEnabled bool) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	r, err := resource.New(ctx,
		resource.WithAttributes(
			resource.Default().Attributes()...,
		),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	)

	if pyroscopeEnabled {
		otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp))
	} else {
		otel.SetTracerProvider(tp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
go test ./pkg/telemetry/ -v -count=1
```

Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry/telemetry.go pkg/telemetry/telemetry_test.go
git commit -m "feat(pkg/telemetry): wrap TracerProvider with otelpyroscope for span profiling"
```

---

### Task 3: Update pkg/profiling with pod-level tags

**Files:**
- Modify: `pkg/profiling/profiling.go`
- Modify: `pkg/profiling/profiling_test.go`

- [ ] **Step 1: Write failing test for tags**

Replace the contents of `pkg/profiling/profiling_test.go` with:

```go
package profiling_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/profiling"
)

func TestStart_NoOpWhenUnset(t *testing.T) {
	cfg := &config.Config{
		ServiceName:            "test-service",
		PyroscopeServerAddress: "",
	}

	stop, err := profiling.Start(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stop == nil {
		t.Fatal("stop func should not be nil")
	}

	// No-op stop should not panic
	stop()
}

func TestStart_BuildsTagsFromEnv(t *testing.T) {
	t.Setenv("POD_NAME", "ratings-abc123")
	t.Setenv("POD_NAMESPACE", "bookinfo")

	tags := profiling.BuildTags()

	if tags["pod"] != "ratings-abc123" {
		t.Errorf("expected pod tag 'ratings-abc123', got %q", tags["pod"])
	}
	if tags["namespace"] != "bookinfo" {
		t.Errorf("expected namespace tag 'bookinfo', got %q", tags["namespace"])
	}
}

func TestStart_BuildsTagsOmitsEmptyEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")

	tags := profiling.BuildTags()

	if _, ok := tags["pod"]; ok {
		t.Error("expected pod tag to be absent when POD_NAME is empty")
	}
	if _, ok := tags["namespace"]; ok {
		t.Error("expected namespace tag to be absent when POD_NAMESPACE is empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./pkg/profiling/ -v -count=1
```

Expected: Compilation error — `profiling.BuildTags` does not exist.

- [ ] **Step 3: Implement tags support**

Replace the contents of `pkg/profiling/profiling.go` with:

```go
// Package profiling provides Pyroscope continuous profiling integration.
package profiling

import (
	"fmt"
	"os"
	"runtime"

	"github.com/grafana/pyroscope-go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
)

// BuildTags reads pod-level metadata from environment variables and returns
// a tag map for Pyroscope labels. Empty values are omitted.
func BuildTags() map[string]string {
	tags := make(map[string]string)

	if v := os.Getenv("POD_NAME"); v != "" {
		tags["pod"] = v
	}
	if v := os.Getenv("POD_NAMESPACE"); v != "" {
		tags["namespace"] = v
	}

	return tags
}

// Start initializes the Pyroscope profiling SDK if PyroscopeServerAddress is set.
// Returns a stop function to shut down the profiler. When the address is empty,
// returns a no-op stop function.
func Start(cfg *config.Config) (func(), error) {
	if cfg.PyroscopeServerAddress == "" {
		return func() {}, nil
	}

	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: cfg.ServiceName,
		ServerAddress:   cfg.PyroscopeServerAddress,
		Tags:            BuildTags(),
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("starting Pyroscope profiler: %w", err)
	}

	return func() {
		_ = profiler.Stop()
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
go test ./pkg/profiling/ -v -count=1
```

Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/profiling/profiling.go pkg/profiling/profiling_test.go
git commit -m "feat(pkg/profiling): add pod-level tags from Kubernetes downward API env vars"
```

---

### Task 4: Update all service cmd/main.go to pass pyroscopeEnabled

**Files:**
- Modify: `services/details/cmd/main.go:39`
- Modify: `services/reviews/cmd/main.go:40`
- Modify: `services/ratings/cmd/main.go:39`
- Modify: `services/notification/cmd/main.go:40`
- Modify: `services/productpage/cmd/main.go:32`

All 5 services have the same pattern. Change the `telemetry.Setup` call from:

```go
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
```

to:

```go
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 1: Update details/cmd/main.go**

In `services/details/cmd/main.go`, change line 39:

```go
// Before:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
// After:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 2: Update reviews/cmd/main.go**

In `services/reviews/cmd/main.go`, change line 40:

```go
// Before:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
// After:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 3: Update ratings/cmd/main.go**

In `services/ratings/cmd/main.go`, change line 39:

```go
// Before:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
// After:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 4: Update notification/cmd/main.go**

In `services/notification/cmd/main.go`, change line 40:

```go
// Before:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
// After:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 5: Update productpage/cmd/main.go**

In `services/productpage/cmd/main.go`, change line 32:

```go
// Before:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName)
// After:
shutdown, err := telemetry.Setup(ctx, cfg.ServiceName, cfg.PyroscopeServerAddress != "")
```

- [ ] **Step 6: Build all services to verify compilation**

Run:

```bash
make build-all
```

Expected: All 5 binaries compile successfully with no errors.

- [ ] **Step 7: Run full test suite**

Run:

```bash
make test
```

Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add services/details/cmd/main.go services/reviews/cmd/main.go services/ratings/cmd/main.go services/notification/cmd/main.go services/productpage/cmd/main.go
git commit -m "feat(services): pass pyroscopeEnabled to telemetry.Setup for span profiling"
```

---

### Task 5: Create Pyroscope Helm values

**Files:**
- Create: `deploy/observability/local/pyroscope-values.yaml`

- [ ] **Step 1: Create the values file**

Create `deploy/observability/local/pyroscope-values.yaml` with:

```yaml
pyroscope:
  extraArgs:
    store.max-disk-usage: 1GB

persistence:
  enabled: true
  size: 2Gi

resources:
  requests:
    cpu: 50m
    memory: 128Mi
  limits:
    cpu: 300m
    memory: 512Mi
```

- [ ] **Step 2: Commit**

```bash
git add deploy/observability/local/pyroscope-values.yaml
git commit -m "feat(deploy): add Pyroscope Helm values for local k8s"
```

---

### Task 6: Add Pyroscope install step to Makefile

**Files:**
- Modify: `Makefile:299-346`

- [ ] **Step 1: Update k8s-observability target**

In the `Makefile`, update the `k8s-observability` target. The current target has 7 steps. Insert Pyroscope as step 4 (after Loki, before Alloy) and bump all subsequent step numbers from `[N/7]` to `[N/8]`.

Change the target description on line 300 from:

```makefile
k8s-observability: ##@Kubernetes Install observability: Prometheus, Grafana, Tempo, Loki, Alloy, Headlamp
```

to:

```makefile
k8s-observability: ##@Kubernetes Install observability: Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy, Headlamp
```

Change step counts from `[N/7]` to `[N/8]` for all existing steps (1-3 stay as-is, 4-7 become 5-8).

After the Loki section (after `@printf "  $(GREEN)Loki ready.$(NC)\n"`), insert:

```makefile
	@printf "$(BOLD)[4/8] Installing Pyroscope...$(NC)\n"
	@$(HELM) upgrade --install pyroscope grafana/pyroscope \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/pyroscope-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Pyroscope ready.$(NC)\n"
```

The full updated target should read:

```makefile
.PHONY: k8s-observability
k8s-observability: ##@Kubernetes Install observability: Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy, Headlamp
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Observability Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_OBSERVABILITY) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@printf "$(BOLD)[1/8] Installing kube-prometheus-stack...$(NC)\n"
	@$(HELM) repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install prometheus prometheus-community/kube-prometheus-stack \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/kube-prometheus-stack-values.yaml \
		--wait --timeout 300s
	@printf "  $(GREEN)kube-prometheus-stack ready.$(NC)\n"
	@printf "$(BOLD)[2/8] Installing Tempo...$(NC)\n"
	@$(HELM) repo add grafana https://grafana.github.io/helm-charts --force-update 2>/dev/null || true
	@$(HELM) upgrade --install tempo grafana/tempo \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/tempo-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Tempo ready.$(NC)\n"
	@printf "$(BOLD)[3/8] Installing Loki...$(NC)\n"
	@$(HELM) upgrade --install loki grafana/loki \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/loki-values.yaml \
		--wait --timeout 300s
	@printf "  $(GREEN)Loki ready.$(NC)\n"
	@printf "$(BOLD)[4/8] Installing Pyroscope...$(NC)\n"
	@$(HELM) upgrade --install pyroscope grafana/pyroscope \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/pyroscope-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Pyroscope ready.$(NC)\n"
	@printf "$(BOLD)[5/8] Installing Alloy (logs)...$(NC)\n"
	@$(HELM) upgrade --install alloy-logs grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-logs-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (logs) ready.$(NC)\n"
	@printf "$(BOLD)[6/8] Installing Alloy (metrics+traces)...$(NC)\n"
	@$(HELM) upgrade --install alloy-metrics-traces grafana/alloy \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/alloy-metrics-traces-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Alloy (metrics+traces) ready.$(NC)\n"
	@printf "$(BOLD)[7/8] Applying Grafana dashboards...$(NC)\n"
	@$(KUBECTL) apply -k deploy/observability/local/dashboards/
	@printf "  $(GREEN)Grafana dashboards applied.$(NC)\n"
	@printf "$(BOLD)[8/8] Installing Headlamp dashboard...$(NC)\n"
	@$(HELM) repo add headlamp https://kubernetes-sigs.github.io/headlamp/ --force-update 2>/dev/null || true
	@$(HELM) upgrade --install headlamp headlamp/headlamp \
		-n $(K8S_NS_OBSERVABILITY) \
		-f deploy/observability/local/headlamp-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Headlamp dashboard ready.$(NC)\n"
	@printf "\n$(GREEN)$(BOLD)Observability layer complete.$(NC)\n\n"
```

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -m "feat(deploy): add Pyroscope Helm install step to k8s-observability"
```

---

### Task 7: Add Pyroscope datasource and tracesToProfiles to Grafana

**Files:**
- Modify: `deploy/observability/local/kube-prometheus-stack-values.yaml:36-72`

- [ ] **Step 1: Add tracesToProfilesV2 to the Tempo datasource**

In `deploy/observability/local/kube-prometheus-stack-values.yaml`, inside the Tempo datasource `jsonData` block (after the `nodeGraph` section at line 72), add:

```yaml
        tracesToProfilesV2:
          datasourceUid: pyroscope
          profileTypeId: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
          customQuery: true
          query: '{service_name="${__span.tags["service.name"]}"}'
          tags:
            - key: "service.name"
              value: "service_name"
            - key: "k8s.namespace.name"
              value: "namespace"
            - key: "k8s.pod.name"
              value: "pod"
```

- [ ] **Step 2: Add Pyroscope as a new datasource**

In the same file, after the Loki datasource block (after line 84), add a new datasource entry:

```yaml
    - name: Pyroscope
      type: grafana-pyroscope-datasource
      uid: pyroscope
      url: http://pyroscope.observability.svc.cluster.local:4040
      access: proxy
      isDefault: false
```

The full `additionalDataSources` section should have 3 entries: Tempo, Loki, Pyroscope.

- [ ] **Step 3: Commit**

```bash
git add deploy/observability/local/kube-prometheus-stack-values.yaml
git commit -m "feat(deploy): add Pyroscope datasource and tracesToProfiles linking in Grafana"
```

---

### Task 8: Add PYROSCOPE_SERVER_ADDRESS to all ConfigMap patches

**Files:**
- Modify: `deploy/details/overlays/local/configmap-patch.yaml`
- Modify: `deploy/reviews/overlays/local/configmap-patch.yaml`
- Modify: `deploy/ratings/overlays/local/configmap-patch.yaml`
- Modify: `deploy/notification/overlays/local/configmap-patch.yaml`
- Modify: `deploy/productpage/overlays/local/configmap-patch.yaml`

- [ ] **Step 1: Add PYROSCOPE_SERVER_ADDRESS to details configmap**

In `deploy/details/overlays/local/configmap-patch.yaml`, add at the end of the `data:` section:

```yaml
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 2: Add PYROSCOPE_SERVER_ADDRESS to reviews configmap**

In `deploy/reviews/overlays/local/configmap-patch.yaml`, add at the end of the `data:` section:

```yaml
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 3: Add PYROSCOPE_SERVER_ADDRESS to ratings configmap**

In `deploy/ratings/overlays/local/configmap-patch.yaml`, add at the end of the `data:` section:

```yaml
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 4: Add PYROSCOPE_SERVER_ADDRESS to notification configmap**

In `deploy/notification/overlays/local/configmap-patch.yaml`, add at the end of the `data:` section:

```yaml
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 5: Add PYROSCOPE_SERVER_ADDRESS to productpage configmap**

In `deploy/productpage/overlays/local/configmap-patch.yaml`, add at the end of the `data:` section:

```yaml
  PYROSCOPE_SERVER_ADDRESS: "http://pyroscope.observability.svc.cluster.local:4040"
```

- [ ] **Step 6: Commit**

```bash
git add deploy/details/overlays/local/configmap-patch.yaml \
       deploy/reviews/overlays/local/configmap-patch.yaml \
       deploy/ratings/overlays/local/configmap-patch.yaml \
       deploy/notification/overlays/local/configmap-patch.yaml \
       deploy/productpage/overlays/local/configmap-patch.yaml
git commit -m "feat(deploy): add PYROSCOPE_SERVER_ADDRESS to all service ConfigMaps"
```

---

### Task 9: Update base deployments — remove pull annotations, add downward API

**Files:**
- Modify: `deploy/details/base/deployment.yaml`
- Modify: `deploy/reviews/base/deployment.yaml`
- Modify: `deploy/ratings/base/deployment.yaml`
- Modify: `deploy/notification/base/deployment.yaml`
- Modify: `deploy/productpage/base/deployment.yaml`

For each file, make two changes:

**Change A — Remove the `profiles.grafana.com/*` annotations block.** Delete these 10 lines from the `template.metadata.annotations` section:

```yaml
      annotations:
        profiles.grafana.com/cpu.scrape: "true"
        profiles.grafana.com/cpu.port: "9090"
        profiles.grafana.com/memory.scrape: "true"
        profiles.grafana.com/memory.port: "9090"
        profiles.grafana.com/goroutine.scrape: "true"
        profiles.grafana.com/goroutine.port: "9090"
        profiles.grafana.com/block.scrape: "true"
        profiles.grafana.com/block.port: "9090"
        profiles.grafana.com/mutex.scrape: "true"
        profiles.grafana.com/mutex.port: "9090"
```

**Change B — Add downward API env vars.** In the container spec, after the `envFrom` block, add:

```yaml
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
```

- [ ] **Step 1: Update details/base/deployment.yaml**

After changes, the `template` section should look like:

```yaml
  template:
    metadata:
      labels:
        app: details
        part-of: event-driven-bookinfo
    spec:
      containers:
        - name: details
          image: event-driven-bookinfo/details:latest
          ports:
            - name: http
              containerPort: 8080
            - name: admin
              containerPort: 9090
          envFrom:
            - configMapRef:
                name: details
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          livenessProbe:
            httpGet:
              path: /healthz
              port: admin
            initialDelaySeconds: 10
            periodSeconds: 10
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /readyz
              port: admin
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
```

- [ ] **Step 2: Update reviews/base/deployment.yaml**

Same pattern as Step 1. Remove annotations, add `env` block with `POD_NAME` and `POD_NAMESPACE`. Container name is `reviews`, image is `event-driven-bookinfo/reviews:latest`, configMapRef name is `reviews`.

- [ ] **Step 3: Update ratings/base/deployment.yaml**

Same pattern. Container name is `ratings`, image is `event-driven-bookinfo/ratings:latest`, configMapRef name is `ratings`.

- [ ] **Step 4: Update notification/base/deployment.yaml**

Same pattern. Container name is `notification`, image is `event-driven-bookinfo/notification:latest`, configMapRef name is `notification`.

- [ ] **Step 5: Update productpage/base/deployment.yaml**

Same pattern. Container name is `productpage`, image is `event-driven-bookinfo/productpage:latest`, configMapRef name is `productpage`.

- [ ] **Step 6: Commit**

```bash
git add deploy/details/base/deployment.yaml \
       deploy/reviews/base/deployment.yaml \
       deploy/ratings/base/deployment.yaml \
       deploy/notification/base/deployment.yaml \
       deploy/productpage/base/deployment.yaml
git commit -m "feat(deploy): remove pull-based profiling annotations, add downward API env vars"
```

---

### Task 10: Update write deployment overlays — remove pull annotations, add downward API

**Files:**
- Modify: `deploy/details/overlays/local/deployment-write.yaml`
- Modify: `deploy/reviews/overlays/local/deployment-write.yaml`
- Modify: `deploy/ratings/overlays/local/deployment-write.yaml`

Same two changes as Task 9 for each file:
- Remove the `profiles.grafana.com/*` annotations block
- Add `env` block with `POD_NAME` and `POD_NAMESPACE` downward API

- [ ] **Step 1: Update details write deployment**

After changes, the `template` section of `deploy/details/overlays/local/deployment-write.yaml` should look like:

```yaml
  template:
    metadata:
      labels:
        app: details
        role: write
        part-of: event-driven-bookinfo
    spec:
      containers:
        - name: details
          image: event-driven-bookinfo/details:local
          ports:
            - name: http
              containerPort: 8080
            - name: admin
              containerPort: 9090
          envFrom:
            - configMapRef:
                name: details
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          livenessProbe:
            httpGet:
              path: /healthz
              port: admin
            initialDelaySeconds: 10
            periodSeconds: 10
            failureThreshold: 5
          readinessProbe:
            httpGet:
              path: /readyz
              port: admin
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
```

- [ ] **Step 2: Update reviews write deployment**

Same pattern. Container name `reviews`, image `event-driven-bookinfo/reviews:local`, configMapRef `reviews`. Labels include `role: write`.

- [ ] **Step 3: Update ratings write deployment**

Same pattern. Container name `ratings`, image `event-driven-bookinfo/ratings:local`, configMapRef `ratings`. Labels include `role: write`.

- [ ] **Step 4: Commit**

```bash
git add deploy/details/overlays/local/deployment-write.yaml \
       deploy/reviews/overlays/local/deployment-write.yaml \
       deploy/ratings/overlays/local/deployment-write.yaml
git commit -m "feat(deploy): remove pull annotations and add downward API to write deployments"
```

---

### Task 11: Update CLAUDE.md and README.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md — observability stack description**

In `CLAUDE.md`, the project overview already mentions "continuous profiling" — verify it is present. If so, no change needed there.

In the "Local k8s" section, change:

```
full observability stack (Prometheus, Grafana, Tempo, Loki, Alloy)
```

to:

```
full observability stack (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy)
```

In the Namespaces section, change:

```
`observability` (Prometheus, Grafana, Tempo, Loki, Alloy)
```

to:

```
`observability` (Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy)
```

- [ ] **Step 2: Update README.md — local k8s description (line 281)**

Change:

```
observability (Prometheus + Grafana + Tempo + Loki + Alloy)
```

to:

```
observability (Prometheus + Grafana + Tempo + Loki + Pyroscope + Alloy)
```

- [ ] **Step 3: Update README.md — mermaid architecture diagram**

In the `observability` subgraph (lines 320-326), add `Pyroscope` node and connection. Change:

```mermaid
    subgraph observability
        Alloy["Alloy"]
        Prom["Prometheus"]
        Tempo["Tempo"]
        Loki["Loki"]
        Grafana["Grafana :3000"]
    end
```

to:

```mermaid
    subgraph observability
        Alloy["Alloy"]
        Prom["Prometheus"]
        Tempo["Tempo"]
        Loki["Loki"]
        Pyro["Pyroscope"]
        Grafana["Grafana :3000"]
    end
```

Add Pyroscope connections after the existing Alloy connections (after line 350):

```mermaid
    PP & DR & DW & RR & RW & RTR & RTW & N -.->|push profiles| Pyro
    Prom & Tempo & Loki & Pyro --> Grafana
```

This replaces the old `Prom & Tempo & Loki --> Grafana` line. Services push profiles directly to Pyroscope (not via Alloy).

- [ ] **Step 4: Update README.md — namespace table (line 373)**

Change:

```
| `observability` | Prometheus, Grafana, Tempo, Loki, Alloy (DaemonSet for logs + Deployment for metrics/traces) |
```

to:

```
| `observability` | Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy (DaemonSet for logs + Deployment for metrics/traces) |
```

- [ ] **Step 5: Update README.md — make k8s-observability target (line 211)**

Change:

```
| `make k8s-observability` | Install Prometheus, Grafana, Tempo, Loki, Alloy |
```

to:

```
| `make k8s-observability` | Install Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy |
```

- [ ] **Step 6: Update README.md — profiling section (lines 484-487)**

Change the profiling section from:

```
Two complementary profiling modes:

- **Local/dev (push mode)**: Pyroscope Go SDK. Set `PYROSCOPE_SERVER_ADDRESS` to enable. No-op when unset. Profiles CPU, alloc, inuse objects/space, goroutines, mutex, and block.
- **Production (pull mode)**: Grafana Alloy DaemonSet with `pyroscope.ebpf` scrapes profiles via pod annotations on the admin port. Zero application changes required.
```

to:

```
Push-based continuous profiling with trace-to-profile correlation:

- **Pyroscope Go SDK** with `grafana/otel-profiling-go` wrapper. Set `PYROSCOPE_SERVER_ADDRESS` to enable. No-op when unset. Profiles CPU, alloc, inuse objects/space, goroutines, mutex, and block. Span IDs are automatically injected into profiling samples, enabling Grafana to link Tempo traces directly to Pyroscope profiles.
```

- [ ] **Step 7: Update README.md — profiling annotations note (line 273)**

Change:

```
All deployments expose dual ports (API `:8080` + admin `:9090`). Liveness and readiness probes target `/healthz` and `/readyz` on the admin port. Pyroscope eBPF scraping is configured via pod annotations on the admin port; no application changes are needed.
```

to:

```
All deployments expose dual ports (API `:8080` + admin `:9090`). Liveness and readiness probes target `/healthz` and `/readyz` on the admin port. Continuous profiling is push-based via the Pyroscope Go SDK with trace-to-profile correlation.
```

- [ ] **Step 8: Update README.md — deploy structure (line 546)**

Change:

```
│   ├── observability/local/    # Helm values: Prometheus, Grafana, Tempo, Loki, Alloy
```

to:

```
│   ├── observability/local/    # Helm values: Prometheus, Grafana, Tempo, Loki, Pyroscope, Alloy
```

- [ ] **Step 9: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: add Pyroscope to architecture diagrams and observability references"
```
