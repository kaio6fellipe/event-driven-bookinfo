// Command specgen generates OpenAPI 3.1, AsyncAPI 3.0, Backstage
// catalog-info.yaml, and Helm values-generated.yaml for every service in
// services/*/, by walking the declarative Endpoints and Exposed slices in
// each service's adapter packages.
//
// Subcommands:
//
//	specgen all     Regenerate every artifact for every service.
//	specgen lint    Run spectral against every generated spec.
//	specgen diff    Run oasdiff (OpenAPI) and an advisory diff (AsyncAPI)
//	                against origin/main.
//
// Run from the repository root.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

const usage = `specgen — generate API specs from service source

Usage:
  specgen <subcommand> [flags]

Subcommands:
  all      Regenerate every artifact for every service.
  lint     Run spectral against every generated spec.
  diff     Run oasdiff against origin/main and warn on AsyncAPI changes.

Run from the repository root.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "all":
		if err := runAll(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen all: %v\n", err)
			os.Exit(1)
		}
	case "lint":
		if err := runLint(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen lint: %v\n", err)
			os.Exit(1)
		}
	case "diff":
		if err := runDiff(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "specgen diff: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runAll(args []string) error {
	fs := flag.NewFlagSet("all", flag.ExitOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = repoRoot
	return errors.New("not implemented")
}

func runLint(args []string) error { return errors.New("not implemented") }
func runDiff(args []string) error { return errors.New("not implemented") }
