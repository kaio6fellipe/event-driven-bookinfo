// Package diff runs oasdiff to detect breaking OpenAPI changes between
// origin/main and the working tree.
package diff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run iterates every services/*/api/openapi.yaml present in HEAD and
// compares it to the same path on origin/main using oasdiff. Returns a
// non-nil error if any breaking change is detected.
func Run(repoRoot string) error {
	servicesDir := filepath.Join(repoRoot, "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return fmt.Errorf("reading services dir: %w", err)
	}

	hadBreaking := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		spec := filepath.Join(servicesDir, e.Name(), "api", "openapi.yaml")
		if _, err := os.Stat(spec); err != nil {
			continue
		}

		baseRef := "origin/main:" + filepath.ToSlash(filepath.Join("services", e.Name(), "api", "openapi.yaml"))
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("oasdiff", "breaking", baseRef, spec, "--fail-on", "ERR")
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		if runErr != nil {
			fmt.Printf("=== %s breaking changes ===\n%s%s", e.Name(), stdout.String(), stderr.String())
			hadBreaking = true
			continue
		}
		fmt.Printf("specgen diff: %s OK\n", e.Name())
	}
	if hadBreaking {
		return fmt.Errorf("breaking OpenAPI changes detected")
	}
	return nil
}
