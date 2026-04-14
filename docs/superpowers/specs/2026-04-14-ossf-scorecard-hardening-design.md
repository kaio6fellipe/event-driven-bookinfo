# OSSF Scorecard Hardening — Design Spec

**Date:** 2026-04-14
**Goal:** Raise OSSF Scorecard from 3.6 to 7.0+
**Approach:** Max all in-repo fixable checks; skip CII badge; no merge workflow changes

## Current State

| Score | Check | Target |
|-------|-------|--------|
| 0 | Token-Permissions | 10 |
| 0 | SAST | 10 |
| 0 | Dependency-Update-Tool | 10 |
| 0 | Fuzzing | 10 |
| 0 | Signed-Releases | 8 |
| 0 | Security-Policy | 10 |
| 1 | Pinned-Dependencies | 10 |
| 4 | Branch-Protection | 8 |
| 8 | Vulnerabilities | 10 |
| 0 | Code-Review | 0 (no change) |
| 0 | Maintained | 0 (no change) |
| 0 | CII-Best-Practices | 0 (excluded) |
| 3 | Contributors | 3 (no change) |
| 10 | Dangerous-Workflow | 10 (already) |
| 10 | Binary-Artifacts | 10 (already) |
| 10 | License | 10 (already) |
| 10 | Packaging | 10 (already) |
| 10 | CI-Tests | 10 (already) |

**Projected: 139/18 = 7.7**

## Section 1: Security Policy

Add `.github/SECURITY.md`:

- Supported versions table listing the latest release tag per service
- Vulnerability reporting via GitHub Security Advisories (private disclosure)
- Expected response timeline: acknowledge within 48 hours, fix target within 14 days for critical
- Explicit instruction: do not open public issues for security vulnerabilities

## Section 2: Dependabot Configuration

Add `.github/dependabot.yml`:

- **gomod** ecosystem: weekly schedule, directory `/`, groups minor+patch into single PRs
- **github-actions** ecosystem: weekly schedule, directory `/`, groups minor+patch into single PRs
- **docker** ecosystem: weekly schedule, directory `/build`, groups minor+patch into single PRs
- Reviewer: repository owner

Dependabot also handles the maintenance burden of SHA-pinned actions and Docker digests from Section 3.

## Section 3: Pin All Dependencies by SHA

### GitHub Actions

Pin every `uses:` reference across all 4 workflows to the commit SHA of the currently-used tag. Format:

```yaml
uses: actions/checkout@<sha> # v4.x.x
```

Affected workflows: `ci.yml`, `auto-tag.yml`, `release.yml`, `scorecard.yml` (partially done).

Actions to pin (~15 unique actions, ~30 total references):
- `actions/checkout`
- `actions/setup-go`
- `actions/upload-artifact`
- `actions/download-artifact`
- `golangci/golangci-lint-action`
- `gitleaks/gitleaks-action`
- `docker/setup-buildx-action`
- `docker/setup-qemu-action`
- `docker/login-action`
- `goreleaser/goreleaser-action`
- `github/codeql-action/upload-sarif`

Already pinned (no change): `ossf/scorecard-action`, `aquasecurity/trivy-action`.

### Docker Base Images

Pin `golang:1.26.2-alpine` to its current digest in all 6 Dockerfiles and all 6 GoReleaser Dockerfiles:

```dockerfile
FROM golang:1.26.2-alpine@sha256:<digest> AS builder
```

`FROM scratch` is not pinnable and Scorecard does not flag it.

## Section 4: Workflow Token Permissions

Set top-level `permissions: {}` on all workflows. Scope per-job:

### ci.yml

| Job | Permissions |
|-----|-------------|
| lint | `contents: read` |
| vet | `contents: read` |
| test | `contents: read` |
| gitleaks | `contents: read` |
| govulncheck | `contents: read` |
| build | `contents: read` |
| docker | `contents: read` |
| e2e | `contents: read` |
| e2e-postgres | `contents: read` |
| coverage-badges | `contents: write` |

### auto-tag.yml

| Job | Permissions |
|-----|-------------|
| auto-tag | `contents: write`, `actions: write` |

### release.yml

| Job | Permissions |
|-----|-------------|
| release | `contents: write`, `packages: write` |
| provenance | `contents: read`, `id-token: write`, `attestations: write`, `actions: read` |

### scorecard.yml

Already correct — no changes.

## Section 5: SAST with CodeQL

Add `.github/workflows/codeql.yml`:

- **Triggers:** `push` to `main`, `pull_request` to `main`, weekly `schedule`
- **Language:** `go`
- **Steps:** `codeql-action/init` → `codeql-action/autobuild` → `codeql-action/analyze`
- **Permissions:** `contents: read`, `security-events: write` (per-job)
- **Top-level permissions:** `{}`
- All action references SHA-pinned

No custom query packs — default suite covers injection, crypto, OWASP top 10.

Complements existing `golangci-lint` (style/bugs) and `govulncheck` (dependency CVEs) with path-sensitive dataflow analysis.

## Section 6: Fuzzing

Native Go fuzz tests in domain layer. Scorecard detects `func Fuzz*` functions automatically.

### Files

| File | Fuzz Function | Input |
|------|--------------|-------|
| `services/details/internal/core/domain/fuzz_test.go` | `FuzzCreateDetail` | Arbitrary title, author, ISBN strings |
| `services/reviews/internal/core/domain/fuzz_test.go` | `FuzzCreateReview` | Arbitrary body, reviewer strings |
| `services/ratings/internal/core/domain/fuzz_test.go` | `FuzzCreateRating` | Arbitrary star rating integers |
| `services/dlqueue/internal/core/domain/fuzz_test.go` | `FuzzCreateDLQEvent` | Arbitrary payload, sensor_name, trigger strings |

### Design

Each fuzz test:
1. Calls `f.Fuzz(func(t *testing.T, ...) { ... })` with typed seed inputs
2. Exercises the domain constructor or validation function
3. Asserts no panics (implicit in fuzz framework)
4. Validates error returns for invalid input

No corpus files. Not run in CI (fuzz tests run indefinitely). Purpose: Scorecard detection + local developer use with `-fuzztime` flag.

## Section 7: Signed Releases with SLSA Provenance

Modify `release.yml` to add SLSA Level 3 provenance after GoReleaser builds.

### Approach

Use `slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml` as a reusable workflow (called job).

### Flow

1. Existing `release` job builds with GoReleaser, creates GitHub Release, uploads `.tar.gz` artifacts
2. Existing `release` job gains a new final step: compute SHA256 digests of `dist/*.tar.gz`, base64-encode the `<sha256>  <filename>` lines, and set as a job output (`hashes`)
3. New `provenance` job (needs: release):
   - Receives `hashes` output from `release` job
   - Calls SLSA generator reusable workflow with `base64-subjects: ${{ needs.release.outputs.hashes }}`
   - Sigstore keyless signing via GitHub OIDC (no secrets needed)
   - Uploads `.intoto.jsonl` provenance file to the same GitHub Release

### Permissions

The `provenance` job needs:
- `id-token: write` — Sigstore OIDC token
- `attestations: write` — attach provenance to release
- `contents: read` — read release artifacts
- `actions: read` — required by SLSA generator

### Verification

Consumers verify with: `slsa-verifier verify-artifact --source-uri github.com/kaio6fellipe/event-driven-bookinfo <artifact>`

## Section 8: Branch Protection Settings

Manual GitHub settings changes to the existing `main` branch ruleset.

### Changes

| Setting | Current | Target |
|---------|---------|--------|
| Require PR before merge | Yes | Keep |
| Required approvals | 0 | **1** |
| Dismiss stale reviews | Off | **On** |
| Require last push approval | Off | **On** |
| Require status checks | Partial | **Enforce: lint, vet, test, build, e2e, e2e-postgres** |
| Require up-to-date branch | Off | **On** |

Target: Tier 3 = 8/10. Tier 4 (9 pts) requires 2 reviewers + CODEOWNERS — not practical for solo project.

As a solo maintainer with admin bypass, you can still approve and merge your own PRs. The approval record satisfies Scorecard.

## Section 9: Fix Go Vulnerabilities

Update dependencies to resolve GO-2026-4771 and GO-2026-4772. Identify affected packages via `govulncheck`, run `go get` to update, `go mod tidy`.

## Out of Scope

- **CII-Best-Practices badge:** Excluded per decision — requires external questionnaire
- **Code-Review workflow changes:** Stays at 0 — solo maintainer, no required approvals on merge
- **Maintained score:** Time-based, will improve naturally with ongoing commits
- **Contributors:** Needs external contributors, cannot force
- **CI fuzz budget:** Fuzz tests are for Scorecard detection and local use only

## Implementation Order

1. Security Policy + Dependabot config (trivial, no deps)
2. Pin all dependencies by SHA (groundwork for SLSA)
3. Token permissions (all workflows)
4. CodeQL workflow
5. Fuzz tests
6. Fix Go vulnerabilities
7. Signed releases (SLSA provenance)
8. Branch protection settings (manual, done last to avoid blocking PRs during implementation)
