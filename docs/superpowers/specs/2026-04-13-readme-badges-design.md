# README Badges Design

**Date:** 2026-04-13
**Status:** Approved
**Scope:** Main `README.md` enhancement, plus `.github/workflows/scorecard.yml`.

## Problem

The main `README.md` lacks the "at-a-glance" signals common to production Go monorepos: build health, code quality, Go version, license, security posture, and release status per service. Visitors cannot quickly verify that CI is green, the module is indexable, or which service version is current.

## Goal

Add a badge set that mirrors the jit-runners aesthetic (centered, shields.io-based) while respecting this repo's monorepo reality: per-service release tags (`<service>-v<semver>`) rather than a single unified version.

## Non-goals

- Changing module path or repo name.
- Adding per-service README badges (only main README in scope).
- Adding code coverage badge (requires coverage publishing pipeline not currently in place).
- Adding community/chat badges (no community channel exists).

## Module + repo facts

- Remote: `git@github.com:kaio6fellipe/go-http-server.git` (local dir `go-http-server`).
- GitHub redirects to canonical repo: `kaio6fellipe/event-driven-bookinfo`.
- Go module: `github.com/kaio6fellipe/event-driven-bookinfo` (matches canonical repo → pkg.go.dev + Go Report Card resolve).
- Per-service release tag pattern: `<service>-v<X.Y.Z>` (e.g., `dlqueue-v0.1.0`).

## Design

### 1. Top-of-README centered badge block

Inserted between the H1 (`<h1 align="center">Event-Driven Bookinfo</h1>`) and the existing Mermaid architecture diagram.

Accent color `#9F50DA` applied to the Go version badge (identity marker). All other badges use shields.io or service-native defaults.

```html
<p align="center">
  <a href="https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain"><img src="https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml/badge.svg" alt="CI Status" /></a>
  <a href="https://pkg.go.dev/github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://pkg.go.dev/badge/github.com/kaio6fellipe/event-driven-bookinfo.svg" alt="Go Reference" /></a>
  <a href="https://goreportcard.com/report/github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://goreportcard.com/badge/github.com/kaio6fellipe/event-driven-bookinfo" alt="Go Report Card" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/kaio6fellipe/event-driven-bookinfo?color=%239F50DA&label=Go" alt="Go Version" /></a>
  <a href="https://unlicense.org/"><img src="https://img.shields.io/badge/license-Unlicense-blue.svg" alt="License" /></a>
  <a href="https://securityscorecards.dev/viewer/?uri=github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://api.securityscorecards.dev/projects/github.com/kaio6fellipe/event-driven-bookinfo/badge" alt="OpenSSF Scorecard" /></a>
  <a href="https://www.conventionalcommits.org"><img src="https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg" alt="Conventional Commits" /></a>
</p>
```

Badge order (left-to-right): CI, Go Reference, Go Report Card, Go version, License, OpenSSF Scorecard, Conventional Commits.

### 2. Per-service release badge column in Services table

Insert a new **Release** column as the second column of the existing Services table in the "Services" section.

Badge URL template:
```
https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=<service>-v*&sort=semver&display_name=tag&label=release
```

Badge links to:
```
https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=<service>-v&expanded=true
```

Final table shape:

| Service | Release | Type | Port | Description |
|---|---|---|---|---|
| **productpage** | [badge] | BFF (Go + HTMX) | :8080 web / :9090 admin | Aggregator; fans out sync reads to details + reviews; renders HTML with HTMX; pending review cache via Redis |
| **details** | [badge] | Backend (hex arch) | :8080 / :9090 admin | Book metadata CRUD |
| **reviews** | [badge] | Backend (hex arch) | :8080 / :9090 admin | User reviews; sync call to ratings service |
| **ratings** | [badge] | Backend (hex arch) | :8080 / :9090 admin | Star ratings |
| **notification** | [badge] | Backend (hex arch) | :8080 / :9090 admin | Event consumer audit log |
| **dlqueue** | [badge] | Backend (hex arch) | :8080 / :9090 admin | Dead letter queue for failed sensor deliveries; REST API for inspection, replay, and resolution |

Services without a tag yet render as "no releases" — acceptable initial state; the badge auto-populates on first tag.

### 3. OpenSSF Scorecard workflow

New file: `.github/workflows/scorecard.yml`. Based on the official OSSF Scorecard template — no customizations beyond repo-specific naming.

Spec:

- **Triggers:**
  - `schedule`: `cron: '0 6 * * 1'` (weekly Monday 06:00 UTC).
  - `push`: branches `[main]`.
  - `workflow_dispatch`: for manual runs.
- **Permissions (top-level):** `read-all`.
- **Job permissions:**
  - `security-events: write` — upload SARIF to Security tab.
  - `id-token: write` — OIDC-based publishing to scorecard API.
  - `contents: read` — repo checkout.
  - `actions: read` — token-permissions check.
- **Steps:**
  1. `actions/checkout@v4` with `persist-credentials: false`.
  2. `ossf/scorecard-action@v2` (pin to SHA in implementation), inputs: `results_file: results.sarif`, `results_format: sarif`, `publish_results: true`.
  3. `actions/upload-artifact@v4` to persist SARIF as a workflow artifact (5-day retention).
  4. `github/codeql-action/upload-sarif@v3` to upload to Security tab.
- **Estimated size:** ~40 lines.

Once the first workflow run completes and publishes to the scorecard API (~24h after first run), the badge in section 1 populates with the actual score.

## File changes summary

| File | Change |
|---|---|
| `README.md` | Insert centered badge block below H1 (section 1). Add **Release** column to Services table with 6 per-service badges (section 2). |
| `.github/workflows/scorecard.yml` | New file implementing section 3. |

## Verification

1. **Local render check:** GitHub web UI renders the README. All 7 top badges load without 404. Per-service badges render (either a tag or "no releases").
2. **Scorecard workflow:** First `workflow_dispatch` run completes green. SARIF uploads to Security tab. Within ~24h, the top-block Scorecard badge shows a numeric score.
3. **No broken links:** every `<a href>` in the new badge block resolves (HTTP 200).
4. **CI green:** existing CI (`ci.yml`) continues to pass unaffected.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Scorecard badge shows "not yet analyzed" immediately after merge | Documented as expected; badge populates within ~24h of first workflow run. |
| Go Report Card initial fetch slow (~30s) | First-ever visit triggers the scan; subsequent loads are cached. |
| pkg.go.dev indexing delay for freshly-tagged services | Non-blocking; badge links are static and the module itself is already indexed. |
| Scorecard action pinned to major version (`@v2`) drifts | Implementation should pin to a specific commit SHA, following OSSF recommendation. |
| Badge row wraps on narrow viewports | Accepted; shields.io badges are small and `<p align="center">` wraps gracefully. |

## Out of scope / future work

- Per-service README badges (individual `services/<name>/README.md` files do not currently exist; can be a future enhancement once services grow standalone docs).
- Code coverage badge (requires adding `codecov`/`coveralls` integration).
- OpenSSF Best Practices (CII) badge (requires manual self-assessment submission at bestpractices.dev).
