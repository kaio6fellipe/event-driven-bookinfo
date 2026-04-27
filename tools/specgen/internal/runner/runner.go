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

// DiscoverServices returns every service under <repoRoot>/services/ that has
// migrated: either internal/adapter/inbound/http/endpoints.go (HTTP) or
// internal/adapter/outbound/kafka/exposed.go (Kafka) must exist.
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

		httpEndpoints := filepath.Join(root, "internal/adapter/inbound/http/endpoints.go")
		if _, err := os.Stat(httpEndpoints); err == nil {
			svc.HasHTTPPkg = true
			svc.HTTPPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/inbound/http"
		}
		kafkaExposed := filepath.Join(root, "internal/adapter/outbound/kafka/exposed.go")
		if _, err := os.Stat(kafkaExposed); err == nil {
			svc.HasKafkaPkg = true
			svc.KafkaPkg = modulePath + "/services/" + e.Name() + "/internal/adapter/outbound/kafka"
		}

		if svc.HasHTTPPkg || svc.HasKafkaPkg {
			out = append(out, svc)
		}
	}
	return out, nil
}

// RunAll regenerates artifacts for every discovered service. Per-service
// failures are printed and accumulated; the function returns a sentinel error
// if any service failed so the caller can exit non-zero.
func RunAll(repoRoot string) error {
	svcs, err := DiscoverServices(repoRoot)
	if err != nil {
		return err
	}
	var hadFailure bool
	for _, s := range svcs {
		if err := generateOne(repoRoot, s); err != nil {
			fmt.Fprintf(os.Stderr, "specgen: %s SKIPPED: %v\n", s.Name, err)
			hadFailure = true
			continue
		}
		fmt.Printf("specgen: %s OK\n", s.Name)
	}
	if hadFailure {
		return fmt.Errorf("one or more services failed (see SKIPPED messages above)")
	}
	return nil
}

// writeFile ensures the parent directory exists before writing, creating it
// only on first actual write — so failed services leave no breadcrumbs.
func writeFile(path string, data []byte) error {
	// #nosec G301 -- generated artifact dirs need group/other read for tooling
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	// #nosec G306 -- generated artifact files need group/other read for tooling
	return os.WriteFile(path, data, 0o644)
}

func generateOne(repoRoot string, svc Service) error {
	apiDir := filepath.Join(repoRoot, "services", svc.Name, "api")
	deployDir := filepath.Join(repoRoot, "deploy", svc.Name)

	var (
		endpoints []walker.EndpointInfo
		exposed   []walker.DescriptorInfo
		consumed  []walker.ConsumedInfo
		version   string
	)

	if svc.HasHTTPPkg {
		eps, ver, err := walker.LoadEndpoints(repoRoot, svc.HTTPPkg)
		if err != nil {
			return fmt.Errorf("loading endpoints: %w", err)
		}
		endpoints = eps
		version = ver

		cs, err := walker.LoadConsumed(repoRoot, svc.HTTPPkg)
		if err != nil {
			return fmt.Errorf("loading consumed: %w", err)
		}
		consumed = cs

		yamlBytes, err := openapi.Build(openapi.Input{
			ServiceName: svc.Name,
			Version:     version,
			Endpoints:   endpoints,
			Metadata: openapi.SpecMetadata{
				OrgName:     Metadata.OrgName,
				OrgURL:      Metadata.OrgURL,
				LicenseName: Metadata.LicenseName,
				LicenseURL:  Metadata.LicenseURL,
				OpenAPIServer: openapi.ServerEntry{
					URL:         Metadata.OpenAPIServer.URL,
					Description: Metadata.OpenAPIServer.Description,
				},
			},
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
			Metadata: asyncapi.SpecMetadata{
				OrgName:     Metadata.OrgName,
				OrgURL:      Metadata.OrgURL,
				LicenseName: Metadata.LicenseName,
				LicenseURL:  Metadata.LicenseURL,
				AsyncAPIServer: asyncapi.ServerEntry{
					URL:         Metadata.AsyncAPIServer.URL,
					Description: Metadata.AsyncAPIServer.Description,
				},
			},
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
		ServiceName:    svc.Name,
		Owner:          "bookinfo-team",
		RepoTreeURL:    "https://github.com/kaio6fellipe/event-driven-bookinfo/tree/main/services/" + svc.Name,
		HasOpenAPI:     svc.HasHTTPPkg,
		HasAsyncAPI:    svc.HasKafkaPkg,
		ExposedNames:   exposedNames,
		ExposedSubset:  exposed,
		ConsumedSubset: consumed,
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
