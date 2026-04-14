# OSSF Scorecard Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Raise OSSF Scorecard from 3.6 to 7.0+ by maxing all in-repo fixable checks.

**Architecture:** Config-only changes across GitHub workflows, Dockerfiles, and a handful of Go fuzz test files. No application code changes except a dependency bump for vulnerability fixes. Branch protection changes are manual GitHub UI steps documented at the end.

**Tech Stack:** GitHub Actions, Dependabot, CodeQL, Go native fuzzing, SLSA provenance (slsa-github-generator), Sigstore

---

## File Map

| Action | File |
|--------|------|
| Create | `.github/SECURITY.md` |
| Create | `.github/dependabot.yml` |
| Create | `.github/workflows/codeql.yml` |
| Create | `services/details/internal/core/domain/fuzz_test.go` |
| Create | `services/reviews/internal/core/domain/fuzz_test.go` |
| Create | `services/ratings/internal/core/domain/fuzz_test.go` |
| Create | `services/dlqueue/internal/core/domain/fuzz_test.go` |
| Modify | `.github/workflows/ci.yml` |
| Modify | `.github/workflows/auto-tag.yml` |
| Modify | `.github/workflows/release.yml` |
| Modify | `.github/workflows/scorecard.yml` |
| Modify | `build/Dockerfile.details` |
| Modify | `build/Dockerfile.notification` |
| Modify | `build/Dockerfile.productpage` |
| Modify | `build/Dockerfile.ratings` |
| Modify | `build/Dockerfile.reviews` |
| Modify | `build/Dockerfile.dlqueue` |
| Modify | `go.mod` |
| Modify | `go.sum` |

---

### Task 1: Add Security Policy

**Files:**
- Create: `.github/SECURITY.md`

- [ ] **Step 1: Create the security policy file**

```markdown
# Security Policy

## Supported Versions

| Service | Supported |
|---------|-----------|
| productpage | Latest release |
| details | Latest release |
| reviews | Latest release |
| ratings | Latest release |
| notification | Latest release |
| dlqueue | Latest release |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report vulnerabilities through [GitHub Security Advisories](https://github.com/kaio6fellipe/event-driven-bookinfo/security/advisories/new).

### What to expect

- **Acknowledgement:** Within 48 hours of your report.
- **Assessment:** We will evaluate the severity and affected versions within 7 days.
- **Fix:** Critical vulnerabilities will be patched within 14 days. Lower-severity issues will be addressed in the next regular release cycle.

### What to include

- Description of the vulnerability
- Steps to reproduce (or a proof-of-concept)
- Affected service(s) and version(s)
- Suggested fix (if any)

## Disclosure Policy

We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure). We will work with you to understand and address the issue before any public disclosure.
```

- [ ] **Step 2: Verify the file renders correctly**

Run: `cat .github/SECURITY.md`
Expected: The markdown content above, rendered correctly.

- [ ] **Step 3: Commit**

```bash
git add .github/SECURITY.md
git commit -m "docs: add security policy for vulnerability reporting"
```

---

### Task 2: Add Dependabot Configuration

**Files:**
- Create: `.github/dependabot.yml`

- [ ] **Step 1: Create the dependabot config**

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
    groups:
      go-minor-patch:
        update-types:
          - minor
          - patch

  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    groups:
      actions-minor-patch:
        update-types:
          - minor
          - patch

  - package-ecosystem: docker
    directory: /build
    schedule:
      interval: weekly
    groups:
      docker-minor-patch:
        update-types:
          - minor
          - patch
```

- [ ] **Step 2: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/dependabot.yml'))"`
Expected: No output (valid YAML).

- [ ] **Step 3: Commit**

```bash
git add .github/dependabot.yml
git commit -m "ci: add Dependabot for gomod, actions, and docker updates"
```

---

### Task 3: Pin GitHub Actions by SHA

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/auto-tag.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `.github/workflows/scorecard.yml`

**SHA reference table** (resolved from current tags):

| Action | Tag | SHA |
|--------|-----|-----|
| actions/checkout | v4 | `34e114876b0b11c390a56381ad16ebd13914f8d5` |
| actions/setup-go | v5 | `40f1582b2485089dde7abd97c1529aa768e1baff` |
| actions/upload-artifact | v4 | `ea165f8d65b6e75b540449e92b4886f43607fa02` |
| actions/download-artifact | v4 | `d3f86a106a0bac45b974a628896c90dbdf5c8093` |
| golangci/golangci-lint-action | v7 | `9fae48acfc02a90574d7c304a1758ef9895495fa` |
| gitleaks/gitleaks-action | v2 | `ff98106e4c7b2bc287b24eaf42907196329070c7` |
| docker/setup-buildx-action | v3 | `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f` |
| docker/setup-qemu-action | v3 | `c7c53464625b32c7a7e944ae62b3e17d2b600130` |
| docker/login-action | v3 | `c94ce9fb468520275223c153574b00df6fe4bcc9` |
| goreleaser/goreleaser-action | v6 | `e435ccd777264be153ace6237001ef4d979d3a7a` |
| github/codeql-action/* | v3 | `5c8a8a642e79153f5d047b10ec1cba1d1cc65699` |

Already pinned (no change): `ossf/scorecard-action`, `aquasecurity/trivy-action`.

- [ ] **Step 1: Pin actions in ci.yml**

Replace every unpinned `uses:` line in `.github/workflows/ci.yml`:

```yaml
# Before:
uses: actions/checkout@v4
# After:
uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

# Before:
uses: actions/setup-go@v5
# After:
uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5

# Before:
uses: actions/upload-artifact@v4
# After:
uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4

# Before:
uses: actions/download-artifact@v4
# After:
uses: actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4

# Before:
uses: golangci/golangci-lint-action@v7
# After:
uses: golangci/golangci-lint-action@9fae48acfc02a90574d7c304a1758ef9895495fa # v7

# Before:
uses: gitleaks/gitleaks-action@v2
# After:
uses: gitleaks/gitleaks-action@ff98106e4c7b2bc287b24eaf42907196329070c7 # v2

# Before:
uses: docker/setup-buildx-action@v3
# After:
uses: docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3

# Before:
uses: github/codeql-action/upload-sarif@v3
# After:
uses: github/codeql-action/upload-sarif@5c8a8a642e79153f5d047b10ec1cba1d1cc65699 # v3
```

Apply each replacement to ALL occurrences in the file (e.g., `actions/checkout@v4` appears many times).

- [ ] **Step 2: Pin actions in auto-tag.yml**

```yaml
# Before:
uses: actions/checkout@v4
# After:
uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
```

- [ ] **Step 3: Pin actions in release.yml**

```yaml
# Before:
uses: actions/checkout@v4
# After:
uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

# Before:
uses: actions/setup-go@v5
# After:
uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5

# Before:
uses: docker/setup-qemu-action@v3
# After:
uses: docker/setup-qemu-action@c7c53464625b32c7a7e944ae62b3e17d2b600130 # v3

# Before:
uses: docker/setup-buildx-action@v3
# After:
uses: docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3

# Before:
uses: docker/login-action@v3
# After:
uses: docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3

# Before:
uses: goreleaser/goreleaser-action@v6
# After:
uses: goreleaser/goreleaser-action@e435ccd777264be153ace6237001ef4d979d3a7a # v6
```

- [ ] **Step 4: Pin actions in scorecard.yml**

```yaml
# Before:
uses: actions/checkout@v4
# After:
uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

# Before:
uses: actions/upload-artifact@v4
# After:
uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4

# Before:
uses: github/codeql-action/upload-sarif@v3
# After:
uses: github/codeql-action/upload-sarif@5c8a8a642e79153f5d047b10ec1cba1d1cc65699 # v3
```

`ossf/scorecard-action` is already SHA-pinned — do not change it.

- [ ] **Step 5: Verify no unpinned actions remain**

Run: `grep -rn 'uses:' .github/workflows/*.yml | grep -v '@[a-f0-9]\{40\}' | grep -v 'slsa-framework'`
Expected: Empty output (all actions pinned by SHA). The `slsa-framework` exclusion is for the reusable workflow added in Task 8 which uses a different pinning pattern.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/auto-tag.yml .github/workflows/release.yml .github/workflows/scorecard.yml
git commit -m "ci: pin all GitHub Actions to commit SHAs"
```

---

### Task 4: Pin Docker Base Images by Digest

**Files:**
- Modify: `build/Dockerfile.details`
- Modify: `build/Dockerfile.notification`
- Modify: `build/Dockerfile.productpage`
- Modify: `build/Dockerfile.ratings`
- Modify: `build/Dockerfile.reviews`
- Modify: `build/Dockerfile.dlqueue`

GoReleaser Dockerfiles (`build/Dockerfile.goreleaser.*`) use `FROM scratch` only — no change needed.

**Digest:** `sha256:c216c4343b489259302908b67a3c8fa55b283bdc30be729baa38b9953ca28857`

- [ ] **Step 1: Pin the golang base image in all 6 Dockerfiles**

In each of the 6 files above, replace:

```dockerfile
FROM golang:1.26.2-alpine AS builder
```

with:

```dockerfile
FROM golang:1.26.2-alpine@sha256:c216c4343b489259302908b67a3c8fa55b283bdc30be729baa38b9953ca28857 AS builder
```

- [ ] **Step 2: Verify all Dockerfiles are pinned**

Run: `grep '^FROM golang' build/Dockerfile.* | grep -v '@sha256:'`
Expected: Empty output (all golang images pinned).

- [ ] **Step 3: Verify builds still work**

Run: `docker build -f build/Dockerfile.ratings -t test-pin:latest . 2>&1 | tail -5`
Expected: Build succeeds with `Successfully built` or `Successfully tagged`.

- [ ] **Step 4: Commit**

```bash
git add build/Dockerfile.details build/Dockerfile.notification build/Dockerfile.productpage build/Dockerfile.ratings build/Dockerfile.reviews build/Dockerfile.dlqueue
git commit -m "ci: pin Docker base images to digest"
```

---

### Task 5: Scope Workflow Token Permissions

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/auto-tag.yml`
- Modify: `.github/workflows/release.yml`

`scorecard.yml` already has correct permissions — no change.

- [ ] **Step 1: Update ci.yml permissions**

Replace the top-level permissions block:

```yaml
# Before:
permissions:
  contents: read
  security-events: write

# After:
permissions: {}
```

Add per-job permissions to every job in ci.yml:

```yaml
  lint:
    name: Lint
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      # ...

  vet:
    name: Vet
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      # ...

  test:
    name: Test
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      # ...

  gitleaks:
    name: Secret Scan
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      # ...

  govulncheck:
    name: Vulnerability Scan (Go)
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      # ...

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint, vet, test, govulncheck, gitleaks]
    permissions:
      contents: read
    steps:
      # ...

  docker:
    name: Docker Build + Scan
    runs-on: ubuntu-latest
    needs: [build]
    permissions:
      contents: read
    steps:
      # ...

  e2e:
    name: E2E Tests
    runs-on: ubuntu-latest
    needs: [docker]
    permissions:
      contents: read
    steps:
      # ...

  e2e-postgres:
    name: E2E Tests (PostgreSQL)
    runs-on: ubuntu-latest
    needs: [docker]
    permissions:
      contents: read
    steps:
      # ...

  coverage-badges:
    name: Publish coverage badges
    runs-on: ubuntu-latest
    needs: [test]
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    permissions:
      contents: write
    steps:
      # ...
```

- [ ] **Step 2: Update auto-tag.yml permissions**

Replace the top-level permissions block:

```yaml
# Before:
permissions:
  contents: write
  actions: write

# After:
permissions: {}
```

Add per-job permissions:

```yaml
  auto-tag:
    name: Auto Tag Services
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    permissions:
      contents: write
      actions: write
    steps:
      # ...
```

- [ ] **Step 3: Update release.yml permissions**

Replace the top-level permissions block:

```yaml
# Before:
permissions:
  contents: write
  packages: write

# After:
permissions: {}
```

Add per-job permissions to the existing `release` job:

```yaml
  release:
    name: Release ${{ inputs.service }}
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write
    steps:
      # ...
```

The `provenance` job permissions will be added in Task 8 (SLSA).

- [ ] **Step 4: Verify YAML is valid**

Run: `for f in .github/workflows/*.yml; do python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "$f OK"; done`
Expected: All 4 files print "OK".

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/auto-tag.yml .github/workflows/release.yml
git commit -m "ci: scope workflow token permissions per-job"
```

---

### Task 6: Add CodeQL SAST Workflow

**Files:**
- Create: `.github/workflows/codeql.yml`

- [ ] **Step 1: Create the CodeQL workflow**

```yaml
name: CodeQL

on:
  push:
    branches:
      - main
  pull_request:
    branches:
      - main
  schedule:
    - cron: '0 6 * * 3'

permissions: {}

jobs:
  analyze:
    name: Analyze Go
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

      - name: Initialize CodeQL
        uses: github/codeql-action/init@5c8a8a642e79153f5d047b10ec1cba1d1cc65699 # v3
        with:
          languages: go

      - name: Autobuild
        uses: github/codeql-action/autobuild@5c8a8a642e79153f5d047b10ec1cba1d1cc65699 # v3

      - name: Perform CodeQL Analysis
        uses: github/codeql-action/analyze@5c8a8a642e79153f5d047b10ec1cba1d1cc65699 # v3
        with:
          category: /language:go
```

- [ ] **Step 2: Verify YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/codeql.yml'))"`
Expected: No output (valid YAML).

- [ ] **Step 3: Verify actions are SHA-pinned**

Run: `grep 'uses:' .github/workflows/codeql.yml | grep -v '@[a-f0-9]\{40\}'`
Expected: Empty output.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/codeql.yml
git commit -m "ci: add CodeQL SAST analysis workflow"
```

---

### Task 7: Add Fuzz Tests

**Files:**
- Create: `services/details/internal/core/domain/fuzz_test.go`
- Create: `services/reviews/internal/core/domain/fuzz_test.go`
- Create: `services/ratings/internal/core/domain/fuzz_test.go`
- Create: `services/dlqueue/internal/core/domain/fuzz_test.go`

- [ ] **Step 1: Create details fuzz test**

File: `services/details/internal/core/domain/fuzz_test.go`

```go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

func FuzzNewDetail(f *testing.F) {
	f.Add("The Go Programming Language", "Alan Donovan", 2015, "hardcover", 380, "Addison-Wesley", "EN", "0134190440", "978-0134190440")
	f.Add("", "", 0, "", 0, "", "", "", "")
	f.Add("A", "B", -1, "pdf", -100, "P", "X", "1234567890", "1234567890123")

	f.Fuzz(func(t *testing.T, title, author string, year int, bookType string, pages int, publisher, language, isbn10, isbn13 string) {
		d, err := domain.NewDetail(title, author, year, bookType, pages, publisher, language, isbn10, isbn13)
		if err != nil {
			return
		}
		if d.ID == "" {
			t.Error("valid detail must have non-empty ID")
		}
		if d.Title != title {
			t.Errorf("Title = %q, want %q", d.Title, title)
		}
	})
}
```

- [ ] **Step 2: Create reviews fuzz test**

File: `services/reviews/internal/core/domain/fuzz_test.go`

```go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

func FuzzNewReview(f *testing.F) {
	f.Add("product-1", "reviewer-1", "Great book!")
	f.Add("", "", "")
	f.Add("p", "r", "A really long review text that goes on and on and on")

	f.Fuzz(func(t *testing.T, productID, reviewer, text string) {
		r, err := domain.NewReview(productID, reviewer, text)
		if err != nil {
			return
		}
		if r.ID == "" {
			t.Error("valid review must have non-empty ID")
		}
		if r.ProductID != productID {
			t.Errorf("ProductID = %q, want %q", r.ProductID, productID)
		}
	})
}
```

- [ ] **Step 3: Create ratings fuzz test**

File: `services/ratings/internal/core/domain/fuzz_test.go`

```go
package domain_test

import (
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

func FuzzNewRating(f *testing.F) {
	f.Add("product-1", "reviewer-1", 5)
	f.Add("", "", 0)
	f.Add("p", "r", -1)
	f.Add("product-2", "reviewer-2", 100)

	f.Fuzz(func(t *testing.T, productID, reviewer string, stars int) {
		r, err := domain.NewRating(productID, reviewer, stars)
		if err != nil {
			return
		}
		if r.ID == "" {
			t.Error("valid rating must have non-empty ID")
		}
		if r.Stars < 1 || r.Stars > 5 {
			t.Errorf("Stars = %d, want 1-5", r.Stars)
		}
	})
}
```

- [ ] **Step 4: Create dlqueue fuzz test**

File: `services/dlqueue/internal/core/domain/fuzz_test.go`

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
)

func FuzzNewDLQEvent(f *testing.F) {
	f.Add("evt-1", "book-added", "webhook", "sub-1", "sensor-1", "trigger-1", "http://example.com", "default", []byte(`{"key":"val"}`), "application/json", int64(1700000000), 3)
	f.Add("", "", "", "", "", "", "", "", []byte{}, "", int64(0), 0)
	f.Add("e", "t", "s", "x", "sn", "ft", "u", "ns", []byte("garbage"), "text/plain", int64(-1), -5)

	f.Fuzz(func(t *testing.T, eventID, eventType, eventSource, eventSubject, sensorName, failedTrigger, eventSourceURL, namespace string, payload []byte, dataContentType string, tsUnix int64, maxRetries int) {
		p := domain.NewDLQEventParams{
			EventID:         eventID,
			EventType:       eventType,
			EventSource:     eventSource,
			EventSubject:    eventSubject,
			SensorName:      sensorName,
			FailedTrigger:   failedTrigger,
			EventSourceURL:  eventSourceURL,
			Namespace:       namespace,
			OriginalPayload: payload,
			DataContentType: dataContentType,
			EventTimestamp:  time.Unix(tsUnix, 0),
			MaxRetries:      maxRetries,
		}

		e, err := domain.NewDLQEvent(p)
		if err != nil {
			return
		}
		if e.ID == "" {
			t.Error("valid DLQ event must have non-empty ID")
		}
		if e.Status != domain.StatusPending {
			t.Errorf("Status = %q, want %q", e.Status, domain.StatusPending)
		}
	})
}
```

- [ ] **Step 5: Run existing tests to verify no breakage**

Run: `go test ./services/details/... ./services/reviews/... ./services/ratings/... ./services/dlqueue/...`
Expected: All tests PASS.

- [ ] **Step 6: Run each fuzz test briefly to verify they work**

Run:
```bash
go test -fuzz=FuzzNewDetail -fuzztime=5s ./services/details/internal/core/domain/
go test -fuzz=FuzzNewReview -fuzztime=5s ./services/reviews/internal/core/domain/
go test -fuzz=FuzzNewRating -fuzztime=5s ./services/ratings/internal/core/domain/
go test -fuzz=FuzzNewDLQEvent -fuzztime=5s ./services/dlqueue/internal/core/domain/
```
Expected: Each prints `PASS` after ~5 seconds. No panics.

- [ ] **Step 7: Commit**

```bash
git add services/details/internal/core/domain/fuzz_test.go \
       services/reviews/internal/core/domain/fuzz_test.go \
       services/ratings/internal/core/domain/fuzz_test.go \
       services/dlqueue/internal/core/domain/fuzz_test.go
git commit -m "test: add native Go fuzz tests for domain constructors"
```

---

### Task 8: Fix Go Vulnerabilities

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Identify affected packages**

Run: `govulncheck ./... 2>&1 | head -40`
Expected: Shows GO-2026-4771 and GO-2026-4772 with the affected module names and fixed versions.

- [ ] **Step 2: Update affected dependencies**

Run the `go get` commands for the specific modules identified in step 1. Example (adjust based on actual output):

```bash
go get <affected-module>@latest
```

Repeat for each vulnerable module.

- [ ] **Step 3: Tidy modules**

Run: `go mod tidy`

- [ ] **Step 4: Verify vulnerabilities are resolved**

Run: `govulncheck ./...`
Expected: `No vulnerabilities found.`

- [ ] **Step 5: Run full test suite**

Run: `go test -race -count=1 ./...`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum
git commit -m "fix: update dependencies to resolve GO-2026-4771 and GO-2026-4772"
```

---

### Task 9: Add SLSA Provenance to Releases

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add outputs and hash step to the release job**

In `.github/workflows/release.yml`, add `outputs` to the `release` job and a new final step:

```yaml
jobs:
  release:
    name: Release ${{ inputs.service }}
    runs-on: ubuntu-latest
    permissions:
      contents: write
      packages: write
    outputs:
      hashes: ${{ steps.hash.outputs.hashes }}
      tag_name: ${{ inputs.tag }}
    steps:
      # ... all existing steps remain unchanged ...

      - name: Generate artifact hashes
        id: hash
        run: |
          cd dist
          sha256sum *.tar.gz > checksums.txt
          echo "hashes=$(base64 -w0 checksums.txt)" >> "$GITHUB_OUTPUT"
```

- [ ] **Step 2: Add the provenance job**

Append after the `release` job:

```yaml
  provenance:
    name: Generate SLSA provenance
    needs: [release]
    permissions:
      actions: read
      id-token: write
      contents: write
    uses: slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@v2.1.0
    with:
      base64-subjects: ${{ needs.release.outputs.hashes }}
      upload-assets: true
      upload-tag-name: ${{ needs.release.outputs.tag_name }}
```

Note: The SLSA reusable workflow is referenced by tag (`@v2.1.0`), not by SHA. This is required — GitHub does not support SHA references for reusable workflows from external repos. Scorecard understands this exception.

- [ ] **Step 3: Verify YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`
Expected: No output (valid YAML).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add SLSA Level 3 provenance to releases"
```

---

### Task 10: Update Branch Protection Settings (Manual)

These are GitHub UI changes — no files to modify.

- [ ] **Step 1: Open repository ruleset settings**

Navigate to: `https://github.com/kaio6fellipe/event-driven-bookinfo/settings/rules`

Edit the existing ruleset for the `main` branch.

- [ ] **Step 2: Configure review requirements**

Set:
- Required approvals: **1**
- Dismiss stale reviews on new pushes: **On**
- Require approval of the most recent push: **On**

- [ ] **Step 3: Configure status checks**

Set:
- Require status checks to pass before merging: **On**
- Require branches to be up to date before merging: **On**
- Add required checks: **Lint**, **Vet**, **Test**, **Build**, **E2E Tests**, **E2E Tests (PostgreSQL)**

- [ ] **Step 4: Verify settings**

Run: `gh api repos/kaio6fellipe/event-driven-bookinfo/rulesets --jq '.[].rules'`
Expected: Shows the updated rules with required approvals, dismiss stale reviews, status checks.

---

### Task 11: Final Verification

- [ ] **Step 1: Run the full test suite**

Run: `make test`
Expected: All tests PASS.

- [ ] **Step 2: Verify no unpinned actions**

Run: `grep -rn 'uses:' .github/workflows/*.yml | grep -v '@[a-f0-9]\{40\}' | grep -v 'slsa-framework'`
Expected: Empty output.

- [ ] **Step 3: Verify all workflow YAML is valid**

Run: `for f in .github/workflows/*.yml; do python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "$f OK"; done`
Expected: All files print "OK".

- [ ] **Step 4: Verify top-level permissions are empty**

Run: `grep -A1 '^permissions:' .github/workflows/*.yml`
Expected: All workflows show `permissions: {}` or `permissions: read-all` (scorecard).

- [ ] **Step 5: Trigger a scorecard run**

Run: `gh workflow run scorecard.yml`
Expected: Workflow dispatched. Check results after ~5 minutes at `https://github.com/kaio6fellipe/event-driven-bookinfo/security/code-scanning`.
