// Package runner is the orchestrator that turns one repo root into all
// generated artifacts. It is the body of `specgen all`.
package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/asyncapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/backstage"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/openapi"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/values"
	"github.com/kaio6fellipe/event-driven-bookinfo/tools/specgen/internal/walker"
)

// Service describes one discovered service in the repo.
type Service struct {
	Name        string
	Root        string
	HTTPPkg     string // import path of internal/adapter/inbound/http
	KafkaPkg    string // import path of internal/adapter/outbound/kafka
	HasHTTPPkg  bool
	HasKafkaPkg bool
}

const modulePath = "github.com/kaio6fellipe/event-driven-bookinfo"

// DiscoverServices returns every directory under <repoRoot>/services/ that
// contains either an http endpoints.go or a kafka exposed.go.
func DiscoverServices(repoRoot string) ([]Service, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "services"))
	if err != nil {
		return nil, fmt.Errorf("reading services dir: %w", err)
	}

	var out []Service
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(repoRoot, "services", e.Name())
		svc := Service{Name: e.Name(), Root: root}

		httpDir := filepath.Join(root, "internal/adapter/inbound/http")
		if _, err := os.Stat(httpDir); err == nil {
			svc.HasHTTPPkg = true
			svc.HTTPPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/inbound/http"
		}
		kafkaDir := filepath.Join(root, "internal/adapter/outbound/kafka")
		if _, err := os.Stat(kafkaDir); err == nil {
			svc.HasKafkaPkg = true
			svc.KafkaPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/outbound/kafka"
		}

		if svc.HasHTTPPkg || svc.HasKafkaPkg {
			out = append(out, svc)
		}
	}
	return out, nil
}

// RunAll regenerates artifacts for every discovered service. Skips per-service
// failures so a missing endpoints.go in one service doesn't block another.
func RunAll(repoRoot string) error {
	svcs, err := DiscoverServices(repoRoot)
	if err != nil {
		return err
	}
	for _, s := range svcs {
		if err := generateOne(repoRoot, s); err != nil {
			fmt.Fprintf(os.Stderr, "specgen: %s SKIPPED: %v\n", s.Name, err)
			continue
		}
		fmt.Printf("specgen: %s OK\n", s.Name)
	}
	return nil
}

// writeFile ensures the parent directory exists before writing, creating it
// only on first actual write — so failed services leave no breadcrumbs.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}

func generateOne(repoRoot string, svc Service) error {
	apiDir := filepath.Join(repoRoot, "services", svc.Name, "api")
	deployDir := filepath.Join(repoRoot, "deploy", svc.Name)

	var (
		endpoints []walker.EndpointInfo
		exposed   []walker.DescriptorInfo
		version   string
	)

	if svc.HasHTTPPkg {
		eps, ver, err := walker.LoadEndpoints(repoRoot, svc.HTTPPkg)
		if err != nil {
			return fmt.Errorf("loading endpoints: %w", err)
		}
		endpoints = eps
		version = ver

		yamlBytes, err := openapi.Build(openapi.Input{
			ServiceName: svc.Name,
			Version:     version,
			Endpoints:   endpoints,
		})
		if err != nil {
			return fmt.Errorf("building OpenAPI: %w", err)
		}
		if err := writeFile(filepath.Join(apiDir, "openapi.yaml"), yamlBytes); err != nil {
			return err
		}
	}

	if svc.HasKafkaPkg {
		exps, err := walker.LoadExposed(repoRoot, svc.KafkaPkg)
		if err != nil {
			return fmt.Errorf("loading exposed: %w", err)
		}
		exposed = exps

		yamlBytes, err := asyncapi.Build(asyncapi.Input{
			ServiceName: svc.Name,
			Version:     version, // ok if empty when only AsyncAPI side present
			Exposed:     exposed,
		})
		if err != nil {
			return fmt.Errorf("building AsyncAPI: %w", err)
		}
		if err := writeFile(filepath.Join(apiDir, "asyncapi.yaml"), yamlBytes); err != nil {
			return err
		}
	}

	// catalog-info.yaml
	exposedNames := make([]string, len(exposed))
	for i, d := range exposed {
		exposedNames[i] = d.Name
	}
	cBytes, err := backstage.Build(backstage.Input{
		ServiceName:   svc.Name,
		Owner:         "bookinfo-team",
		RepoTreeURL:   "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/" + svc.Name,
		HasOpenAPI:    svc.HasHTTPPkg,
		HasAsyncAPI:   svc.HasKafkaPkg,
		ExposedNames:  exposedNames,
		ExposedSubset: exposed,
	})
	if err != nil {
		return fmt.Errorf("building catalog-info: %w", err)
	}
	if err := writeFile(filepath.Join(apiDir, "catalog-info.yaml"), cBytes); err != nil {
		return err
	}

	// values-generated.yaml
	vBytes, err := values.Build(values.Input{
		ServiceName: svc.Name,
		Endpoints:   endpoints,
		Exposed:     exposed,
	})
	if err != nil {
		return fmt.Errorf("building values: %w", err)
	}
	if err := writeFile(filepath.Join(deployDir, "values-generated.yaml"), vBytes); err != nil {
		return err
	}

	return nil
}
