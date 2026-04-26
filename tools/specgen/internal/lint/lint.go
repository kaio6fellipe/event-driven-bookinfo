// Package lint runs spectral over every generated spec via npx.
package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run invokes spectral on every generated openapi.yaml and asyncapi.yaml
// under repoRoot/services/*/api/.
func Run(repoRoot string) error {
	servicesDir := filepath.Join(repoRoot, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return fmt.Errorf("reading services dir: %w", err)
	}

	var specs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		apiDir := filepath.Join(servicesDir, e.Name(), "api")
		for _, f := range []string{"openapi.yaml", "asyncapi.yaml"} {
			full := filepath.Join(apiDir, f)
			if _, err := os.Stat(full); err == nil {
				specs = append(specs, full)
			}
		}
	}
	if len(specs) == 0 {
		fmt.Println("specgen lint: no specs found")
		return nil
	}

	args := append([]string{"--yes", "@stoplight/spectral-cli", "lint", "--ruleset", filepath.Join(repoRoot, ".spectral.yaml")}, specs...)
	// #nosec G204 -- args derived from trusted filesystem walk under repoRoot
	cmd := exec.Command("npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("spectral: %w", err)
	}
	return nil
}
