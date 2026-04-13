# README Badges Design

**Date:** 2026-04-13
**Status:** Approved
**Scope:** Main `README.md` enhancement, plus `.github/workflows/scorecard.yml`, plus per-service coverage publishing from `ci.yml` into `.github/badges/`.

## Problem

The main `README.md` lacks the "at-a-glance" signals common to production Go monorepos: build health, code quality, Go version, license, security posture, and release status per service. Visitors cannot quickly verify that CI is green, the module is indexable, or which service version is current.

## Goal

Add a badge set that mirrors the jit-runners aesthetic (centered, shields.io-based) while respecting this repo's monorepo reality: per-service release tags (`<service>-v<semver>`) rather than a single unified version.

## Non-goals

- Changing module path or repo name.
- Adding per-service README badges (only main README in scope).
- Adopting a third-party coverage service (Codecov, Coveralls). Coverage is published in-tree via `GITHUB_TOKEN`.
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

Final table shape (adds **Release** and **Coverage** columns — see section 4 for coverage badge source):

| Service | Release | Coverage | Type | Port | Description |
|---|---|---|---|---|---|
| **productpage** | [release badge] | [coverage badge] | BFF (Go + HTMX) | :8080 web / :9090 admin | Aggregator; fans out sync reads to details + reviews; renders HTML with HTMX; pending review cache via Redis |
| **details** | [release badge] | [coverage badge] | Backend (hex arch) | :8080 / :9090 admin | Book metadata CRUD |
| **reviews** | [release badge] | [coverage badge] | Backend (hex arch) | :8080 / :9090 admin | User reviews; sync call to ratings service |
| **ratings** | [release badge] | [coverage badge] | Backend (hex arch) | :8080 / :9090 admin | Star ratings |
| **notification** | [release badge] | [coverage badge] | Backend (hex arch) | :8080 / :9090 admin | Event consumer audit log |
| **dlqueue** | [release badge] | [coverage badge] | Backend (hex arch) | :8080 / :9090 admin | Dead letter queue for failed sensor deliveries; REST API for inspection, replay, and resolution |

Services without a tag yet render as "no releases" — acceptable initial state; the release badge auto-populates on first tag. Coverage badges render the value from `.github/badges/coverage-<service>.json` (section 4).

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

### 4. Per-service coverage badges (in-tree, no PAT)

Coverage is already generated by `make test-cover` (`coverage.out` + `coverage.html`). This section adds per-service extraction and in-tree publishing.

**Storage:** `.github/badges/coverage-<service>.json` on `main`. One file per service (6 files). Each file is a shields.io endpoint payload:

```json
{"schemaVersion":1,"label":"coverage","message":"82.3%","color":"green","cacheSeconds":60}
```

**Badge URL template** (used in Services table):
```
https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-<service>.json
```

**Badge link target:** `https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain` (same as CI badge — deeper link target unnecessary since coverage HTML is an ephemeral artifact).

**Color thresholds** (applied by CI script):

- `>= 80%` → `brightgreen`
- `>= 70%` → `green`
- `>= 60%` → `yellowgreen`
- `>= 50%` → `yellow`
- `>= 40%` → `orange`
- `< 40%` → `red`

**CI changes (`ci.yml`):**

1. Replace the monolithic `make test-cover` step with a per-service coverage matrix (or a single step that loops over the 6 services and emits one profile per service).
2. Retain the existing artifact upload for backwards compatibility (`coverage.out` + `coverage.html` remain available as a combined profile).
3. Add a new publishing job `coverage-badges`:
   - Runs **only on `push` to `main`** (not PRs — forks can't write, and PR churn should not mutate the badge).
   - `needs: [test]`.
   - Permission `contents: write` (scoped to just this job; other jobs keep `contents: read`).
   - Script: for each service in `productpage details reviews ratings notification dlqueue`:
     - Run `go test -coverprofile=coverage-$svc.out ./services/$svc/...`.
     - Extract total with `go tool cover -func=coverage-$svc.out | awk '/total:/ {print $3}'` → strip trailing `%`.
     - Pick color via threshold mapping.
     - Write `.github/badges/coverage-$svc.json` with the shields.io endpoint payload.
   - Commit only changed files via `stefanzweifel/git-auto-commit-action@v5` with:
     - `commit_message: "chore(badges): update coverage [skip ci]"`
     - `file_pattern: ".github/badges/coverage-*.json"`
     - `commit_user_name: github-actions[bot]`
     - `commit_user_email: 41898282+github-actions[bot]@users.noreply.github.com`
   - `[skip ci]` marker prevents the commit from re-triggering CI loops.

**Seed files:** commit placeholder JSON for each of the 6 services in the initial PR so the badge URLs resolve before the first CI publish completes:

```json
{"schemaVersion":1,"label":"coverage","message":"pending","color":"lightgrey","cacheSeconds":60}
```

**Thresholds and color mapping** live in a small shell snippet in the workflow (inlined, ~15 lines) — no separate script file needed. If the logic grows, move to `scripts/coverage-badge.sh`.

## File changes summary

| File | Change |
|---|---|
| `README.md` | Insert centered badge block below H1 (section 1). Add **Release** + **Coverage** columns to Services table with 6 per-service badges each (sections 2, 4). |
| `.github/workflows/scorecard.yml` | New file implementing section 3. |
| `.github/workflows/ci.yml` | Add per-service coverage extraction and a new `coverage-badges` job that commits updated JSON to `main` (section 4). |
| `.github/badges/coverage-*.json` | 6 new seed files, one per service, committed in the initial PR (section 4). |

## Verification

1. **Local render check:** GitHub web UI renders the README. All 7 top badges load without 404. Per-service release and coverage badges render (release shows a tag or "no releases"; coverage shows the seed "pending" value on first view, then a percentage after the first CI publish on `main`).
2. **Scorecard workflow:** First `workflow_dispatch` run completes green. SARIF uploads to Security tab. Within ~24h, the top-block Scorecard badge shows a numeric score.
3. **Coverage publish:** after merging to `main`, the `coverage-badges` job commits updated `.github/badges/coverage-*.json`; raw URLs return the new JSON within minutes; shields.io endpoint renders the current percentage.
4. **No broken links:** every `<a href>` in the new badge block resolves (HTTP 200).
5. **CI green:** existing CI (`ci.yml`) continues to pass unaffected; the new `coverage-badges` job is green on `main`.
6. **No CI loop:** the badge-update commit carries `[skip ci]` and does not re-trigger CI.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Scorecard badge shows "not yet analyzed" immediately after merge | Documented as expected; badge populates within ~24h of first workflow run. |
| Go Report Card initial fetch slow (~30s) | First-ever visit triggers the scan; subsequent loads are cached. |
| pkg.go.dev indexing delay for freshly-tagged services | Non-blocking; badge links are static and the module itself is already indexed. |
| Scorecard action pinned to major version (`@v2`) drifts | Implementation should pin to a specific commit SHA, following OSSF recommendation. |
| Badge row wraps on narrow viewports | Accepted; shields.io badges are small and `<p align="center">` wraps gracefully. |
| Bot-authored badge commits clutter `main` history | Single squashed commit per CI run (only when coverage values actually change — `git-auto-commit-action` no-ops if files are unchanged). Marker `[skip ci]` prevents loops. |
| `raw.githubusercontent.com` caches stale JSON | `cacheSeconds: 60` in each endpoint payload tells shields.io to re-fetch; GitHub's raw CDN TTL is ~5 min, so badges update within minutes of a publish. |
| Coverage extraction fails for a service with no tests | Script treats empty `./services/<svc>/...` coverage as `0.0%` and emits a red badge rather than failing the job. |
| `contents: write` scope expanded on `coverage-badges` job | Scoped to that single job; other jobs keep `contents: read`. Job only triggers on `push` to `main`, never on PRs. |

## Out of scope / future work

- Per-service README badges (individual `services/<name>/README.md` files do not currently exist; can be a future enhancement once services grow standalone docs).
- Third-party coverage integrations (Codecov, Coveralls) — in-tree publishing chosen instead for zero-dependency simplicity.
- OpenSSF Best Practices (CII) badge (requires manual self-assessment submission at bestpractices.dev).
