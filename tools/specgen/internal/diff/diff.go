// Package diff runs oasdiff to detect breaking OpenAPI changes between
// origin/main and the working tree.
package diff

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run iterates every services/*/api/openapi.yaml present in HEAD and
// compares it to the same path on origin/main using oasdiff. New specs
// (not present on origin/main) are reported but do not fail the run.
// Returns a non-nil error only if at least one existing spec has a
// breaking change.
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

		repoRelativePath := filepath.ToSlash(filepath.Join("services", e.Name(), "api", "openapi.yaml"))

		// Check whether the file exists on origin/main; oasdiff needs
		// both sides to compare.
		exists, err := pathExistsOnRef(repoRoot, "origin/main", repoRelativePath)
		if err != nil {
			return fmt.Errorf("checking %s on origin/main: %w", repoRelativePath, err)
		}
		if !exists {
			fmt.Printf("specgen diff: %s NEW (not on origin/main, skipping)\n", e.Name())
			continue
		}

		baseRef := "origin/main:" + repoRelativePath
		var stdout, stderr bytes.Buffer
		// #nosec G204 -- baseRef and spec come from a trusted filesystem walk under repoRoot
		cmd := exec.Command("oasdiff", "breaking", baseRef, spec, "--fail-on", "ERR")
		cmd.Dir = repoRoot
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
		return errors.New("breaking OpenAPI changes detected")
	}
	return nil
}

// pathExistsOnRef returns true when the given repo-relative path is
// reachable on the given git ref. Uses `git cat-file -e <ref>:<path>`.
func pathExistsOnRef(repoRoot, ref, path string) (bool, error) {
	// #nosec G204 -- ref and path come from a trusted filesystem walk under repoRoot
	cmd := exec.Command("git", "cat-file", "-e", ref+":"+path)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// `git cat-file -e` exits 1 when the object is missing — that's the
	// expected "new spec" signal, not an error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("git cat-file: %w (stderr: %s)", err, stderr.String())
}
