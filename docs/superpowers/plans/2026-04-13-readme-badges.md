# README Badges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add top-of-README badge block (CI, Go Reference, Go Report Card, Go version, License, OpenSSF Scorecard, Conventional Commits), per-service release + coverage columns in the Services table, an OSSF Scorecard workflow, and in-tree per-service coverage publishing from `ci.yml`.

**Architecture:** Static badges in `README.md` resolve via shields.io and service-native endpoints. Release badges use `shields.io/github/v/release` with tag filters (`<service>-v*`). Coverage badges use shields.io endpoint mode reading `.github/badges/coverage-<service>.json` served from `raw.githubusercontent.com/.../main/...`. CI writes these JSON files back to `main` via `stefanzweifel/git-auto-commit-action` using the built-in `GITHUB_TOKEN`, with `[skip ci]` to prevent loops.

**Tech Stack:** GitHub Actions, shields.io, OSSF Scorecard Action, `stefanzweifel/git-auto-commit-action`, Go 1.26.2 `go test -coverprofile` + `go tool cover -func`.

---

## File Structure

- **Create:** `.github/workflows/scorecard.yml` — weekly OSSF Scorecard scan, publishes to scorecard API and Security tab.
- **Create:** `.github/badges/coverage-productpage.json` — seed shields.io endpoint payload.
- **Create:** `.github/badges/coverage-details.json` — seed.
- **Create:** `.github/badges/coverage-reviews.json` — seed.
- **Create:** `.github/badges/coverage-ratings.json` — seed.
- **Create:** `.github/badges/coverage-notification.json` — seed.
- **Create:** `.github/badges/coverage-dlqueue.json` — seed.
- **Modify:** `README.md:1-2` — insert centered badge block between H1 and the Mermaid diagram.
- **Modify:** `README.md:170-177` — add **Release** and **Coverage** columns to the Services table.
- **Modify:** `.github/workflows/ci.yml:47-72` — replace monolithic coverage step with per-service extraction; keep existing combined artifact upload.
- **Modify:** `.github/workflows/ci.yml` — add new `coverage-badges` job at the end (only on `push` to `main`) that commits updated JSON.

---

## Task 1: Add top-of-README centered badge block

**Files:**
- Modify: `README.md:1-2`

- [ ] **Step 1: Open `README.md` and confirm the current first two lines**

Expected content at line 1-2:
```
<h1 align="center">Event-Driven Bookinfo</h1>

```

- [ ] **Step 2: Insert the centered badge block between the H1 and the Mermaid diagram**

Use Edit to replace:
```
<h1 align="center">Event-Driven Bookinfo</h1>

```mermaid
```

with:
```
<h1 align="center">Event-Driven Bookinfo</h1>

<p align="center">
  <a href="https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain"><img src="https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml/badge.svg" alt="CI Status" /></a>
  <a href="https://pkg.go.dev/github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://pkg.go.dev/badge/github.com/kaio6fellipe/event-driven-bookinfo.svg" alt="Go Reference" /></a>
  <a href="https://goreportcard.com/report/github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://goreportcard.com/badge/github.com/kaio6fellipe/event-driven-bookinfo" alt="Go Report Card" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/kaio6fellipe/event-driven-bookinfo?color=%239F50DA&label=Go" alt="Go Version" /></a>
  <a href="https://unlicense.org/"><img src="https://img.shields.io/badge/license-Unlicense-blue.svg" alt="License" /></a>
  <a href="https://securityscorecards.dev/viewer/?uri=github.com/kaio6fellipe/event-driven-bookinfo"><img src="https://api.securityscorecards.dev/projects/github.com/kaio6fellipe/event-driven-bookinfo/badge" alt="OpenSSF Scorecard" /></a>
  <a href="https://www.conventionalcommits.org"><img src="https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg" alt="Conventional Commits" /></a>
</p>

```mermaid
```

Note: preserve the existing Mermaid fence exactly. Use an Edit call with enough context (3+ lines) for uniqueness.

- [ ] **Step 3: Verify the edit**

Read lines 1-20 of `README.md`. Confirm:
- Line 1: `<h1 align="center">Event-Driven Bookinfo</h1>`
- Lines 3-12: the new `<p align="center">...</p>` block (exactly 9 lines: opening tag, 7 anchor lines, closing tag).
- Line 13: blank.
- Line 14: ` ```mermaid `.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -s -m "docs(readme): add top-of-readme badge block

Centered shields.io badges for CI, Go Reference, Go Report Card, Go
version, License, OpenSSF Scorecard, and Conventional Commits.
Matches the jit-runners aesthetic."
```

---

## Task 2: Seed per-service coverage JSON files

**Files:**
- Create: `.github/badges/coverage-productpage.json`
- Create: `.github/badges/coverage-details.json`
- Create: `.github/badges/coverage-reviews.json`
- Create: `.github/badges/coverage-ratings.json`
- Create: `.github/badges/coverage-notification.json`
- Create: `.github/badges/coverage-dlqueue.json`

- [ ] **Step 1: Create the directory and six seed files**

Each file has identical content (the badge label is fixed — only the per-file URL differs, not the content):

```json
{"schemaVersion":1,"label":"coverage","message":"pending","color":"lightgrey","cacheSeconds":60}
```

Create all six files:
- `.github/badges/coverage-productpage.json`
- `.github/badges/coverage-details.json`
- `.github/badges/coverage-reviews.json`
- `.github/badges/coverage-ratings.json`
- `.github/badges/coverage-notification.json`
- `.github/badges/coverage-dlqueue.json`

- [ ] **Step 2: Verify shields.io resolves the seed JSON**

Run (one URL verifies all — they share the same content format):
```bash
curl -s "https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-productpage.json" -o /tmp/badge.svg -w "%{http_code}\n"
```
Expected: `200` (this check is only meaningful after the seed files reach `main`; skip if working on a feature branch — the shields.io fetch happens at render time).

- [ ] **Step 3: Commit**

```bash
git add .github/badges/coverage-*.json
git commit -s -m "chore(badges): seed per-service coverage badge JSON

Shields.io endpoint payloads default to 'pending' (grey). CI overwrites
each file with the real percentage after merges to main."
```

---

## Task 3: Add Release + Coverage columns to the Services table

**Files:**
- Modify: `README.md:170-177`

- [ ] **Step 1: Confirm current table shape**

Read `README.md` lines 168-180. Current header:
```
| Service | Type | API Port | Admin Port | Description |
```

- [ ] **Step 2: Replace the table with the new 7-column version**

Use Edit to replace the block from line 170 to line 177 with:

```markdown
| Service | Release | Coverage | Type | API Port | Admin Port | Description |
|---|---|---|---|---|---|---|
| **productpage** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=productpage-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=productpage-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-productpage.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | BFF (Go + HTMX) | 8080 | 9090 | Aggregates details + reviews + ratings into an HTML product page. Fans out sync GET calls; pending review cache via Redis. |
| **details** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=details-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=details-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-details.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Backend | 8081 | 9091 | Book metadata CRUD. Event-written via `book-added` sensor. |
| **reviews** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=reviews-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=reviews-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-reviews.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Backend | 8082 | 9092 | User reviews. Makes sync GET to ratings service. Event-written via `review-submitted` sensor. |
| **ratings** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=ratings-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=ratings-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-ratings.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Backend | 8083 | 9093 | Star ratings per reviewer. Event-written via `rating-submitted` sensor. |
| **notification** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=notification-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=notification-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-notification.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Event consumer | 8084 | 9094 | Receives POST from sensors, stores audit log. Exposes GET for review. |
| **dlqueue** | [![release](https://img.shields.io/github/v/release/kaio6fellipe/event-driven-bookinfo?filter=dlqueue-v*&sort=semver&display_name=tag&label=release)](https://github.com/kaio6fellipe/event-driven-bookinfo/releases?q=dlqueue-v&expanded=true) | [![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/kaio6fellipe/event-driven-bookinfo/main/.github/badges/coverage-dlqueue.json)](https://github.com/kaio6fellipe/event-driven-bookinfo/actions/workflows/ci.yml?query=branch%3Amain) | Backend (hex arch) | 8085 | 9095 | Captures events failing sensor retry exhaustion; stores in PostgreSQL; supports replay via REST API |
```

Important: the column separator count goes from 5 to 7; match the header and separator row.

- [ ] **Step 3: Verify Markdown renders correctly**

Run:
```bash
grep -n "| Service | Release | Coverage" README.md
```
Expected: one line (around line 170) with the new header.

Count pipes on the separator row and a body row to confirm both have the same structure (8 pipes each: leading, 6 between columns, trailing).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -s -m "docs(readme): add release + coverage columns to services table

Release badges use shields.io tag filter (<service>-v*). Coverage
badges use shields.io endpoint mode reading .github/badges/*.json.
Both link to the relevant releases page / CI workflow."
```

---

## Task 4: Create OSSF Scorecard workflow

**Files:**
- Create: `.github/workflows/scorecard.yml`

- [ ] **Step 1: Look up the current pinned SHA for `ossf/scorecard-action`**

Run:
```bash
gh api repos/ossf/scorecard-action/releases/latest --jq '"\(.tag_name) \(.target_commitish)"'
```
Note the latest tag (e.g., `v2.4.0`) and resolve the SHA:
```bash
gh api repos/ossf/scorecard-action/git/ref/tags/<tag> --jq '.object.sha'
```
Record the SHA — it will be pinned in the workflow.

- [ ] **Step 2: Create `.github/workflows/scorecard.yml`**

Content (replace `<SCORECARD_SHA>` with the SHA from step 1):

```yaml
name: OSSF Scorecard

on:
  schedule:
    - cron: '0 6 * * 1'
  push:
    branches:
      - main
  workflow_dispatch:

permissions: read-all

jobs:
  analysis:
    name: Scorecard analysis
    runs-on: ubuntu-latest
    permissions:
      security-events: write
      id-token: write
      contents: read
      actions: read
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          persist-credentials: false

      - name: Run Scorecard
        uses: ossf/scorecard-action@<SCORECARD_SHA>
        with:
          results_file: results.sarif
          results_format: sarif
          publish_results: true

      - name: Upload SARIF artifact
        uses: actions/upload-artifact@v4
        with:
          name: scorecard-sarif
          path: results.sarif
          retention-days: 5

      - name: Upload SARIF to Security tab
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: results.sarif
```

- [ ] **Step 3: Validate workflow syntax**

Run:
```bash
gh workflow view scorecard.yml --repo kaio6fellipe/event-driven-bookinfo 2>/dev/null || echo "Not yet pushed — checking locally"
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/scorecard.yml'))" && echo "YAML OK"
```
Expected: `YAML OK` (the `gh workflow view` call is expected to fail until the workflow is merged; ignore that output).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/scorecard.yml
git commit -s -m "ci: add OSSF Scorecard workflow

Weekly scan (Monday 06:00 UTC) plus push to main and manual
dispatch. Uploads SARIF to the Security tab and publishes results
to api.securityscorecards.dev so the badge renders."
```

---

## Task 5: Add per-service coverage extraction to `ci.yml`

**Files:**
- Modify: `.github/workflows/ci.yml:47-72`

- [ ] **Step 1: Confirm current coverage step**

Read `.github/workflows/ci.yml` lines 47-72. The current `test` job has:
- `Run tests with race detector` → `make test-race`
- `Run tests with coverage` → `make test-cover`
- `Upload coverage report` → uploads `coverage.out` + `coverage.html`.

- [ ] **Step 2: Replace the `Run tests with coverage` step with per-service extraction**

Use Edit to replace:
```yaml
      - name: Run tests with coverage
        run: make test-cover
```

with:
```yaml
      - name: Run tests with coverage (combined)
        run: make test-cover

      - name: Generate per-service coverage profiles
        run: |
          set -euo pipefail
          mkdir -p coverage
          for svc in productpage details reviews ratings notification dlqueue; do
            go test -coverprofile="coverage/coverage-$svc.out" "./services/$svc/..." || \
              echo "mode: set" > "coverage/coverage-$svc.out"
          done

      - name: Upload per-service coverage profiles
        uses: actions/upload-artifact@v4
        with:
          name: coverage-per-service
          path: coverage/coverage-*.out
          retention-days: 7
```

Notes:
- The combined `make test-cover` output (`coverage.out` + `coverage.html`) is retained for the existing artifact upload.
- The fallback `echo "mode: set" > ...` ensures services with no tests still produce a valid (empty) profile rather than failing the job.

- [ ] **Step 3: Verify the `Upload coverage report` step still exists unchanged**

Read the step following the new `Upload per-service coverage profiles` block. It must still read:
```yaml
      - name: Upload coverage report
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: |
            coverage.out
            coverage.html
          retention-days: 7
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -s -m "ci: extract per-service coverage profiles

Keeps the existing combined coverage.out + coverage.html artifact
and adds a per-service profile per Go service under coverage/. A
follow-up job converts these into shields.io endpoint JSON."
```

---

## Task 6: Add `coverage-badges` publishing job to `ci.yml`

**Files:**
- Modify: `.github/workflows/ci.yml` (append a new job at the bottom)

- [ ] **Step 1: Look up the current pinned SHA for `stefanzweifel/git-auto-commit-action`**

Run:
```bash
gh api repos/stefanzweifel/git-auto-commit-action/releases/latest --jq .tag_name
gh api repos/stefanzweifel/git-auto-commit-action/git/ref/tags/<tag_from_above> --jq '.object.sha'
```
Record the SHA.

- [ ] **Step 2: Append the `coverage-badges` job to `ci.yml`**

Add the following job after the existing `e2e-postgres` job (at the end of the file — replace `<GIT_AUTO_COMMIT_SHA>` with the SHA from step 1):

```yaml
  coverage-badges:
    name: Publish coverage badges
    runs-on: ubuntu-latest
    needs: [test]
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    permissions:
      contents: write
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download per-service coverage profiles
        uses: actions/download-artifact@v4
        with:
          name: coverage-per-service
          path: coverage

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.26.2"

      - name: Generate badge JSON
        run: |
          set -euo pipefail
          mkdir -p .github/badges
          color_for() {
            local pct=$1
            local n=${pct%.*}
            if   [ "$n" -ge 80 ]; then echo brightgreen
            elif [ "$n" -ge 70 ]; then echo green
            elif [ "$n" -ge 60 ]; then echo yellowgreen
            elif [ "$n" -ge 50 ]; then echo yellow
            elif [ "$n" -ge 40 ]; then echo orange
            else                       echo red
            fi
          }
          for svc in productpage details reviews ratings notification dlqueue; do
            profile="coverage/coverage-$svc.out"
            if [ ! -s "$profile" ] || [ "$(wc -l < "$profile")" -le 1 ]; then
              pct="0.0"
            else
              pct=$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/,"",$3); print $3}')
              if [ -z "$pct" ]; then pct="0.0"; fi
            fi
            color=$(color_for "$pct")
            printf '{"schemaVersion":1,"label":"coverage","message":"%s%%","color":"%s","cacheSeconds":60}\n' \
              "$pct" "$color" > ".github/badges/coverage-$svc.json"
            echo "$svc -> $pct% ($color)"
          done

      - name: Commit updated badges
        uses: stefanzweifel/git-auto-commit-action@<GIT_AUTO_COMMIT_SHA>
        with:
          commit_message: "chore(badges): update coverage [skip ci]"
          file_pattern: ".github/badges/coverage-*.json"
          commit_user_name: "github-actions[bot]"
          commit_user_email: "41898282+github-actions[bot]@users.noreply.github.com"
          commit_author: "github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>"
```

- [ ] **Step 3: Validate YAML**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML OK"
```
Expected: `YAML OK`.

- [ ] **Step 4: Sanity-check the color script locally**

Run (emulates the `color_for` function against boundary values):
```bash
bash -c '
color_for() {
  local pct=$1; local n=${pct%.*}
  if   [ "$n" -ge 80 ]; then echo brightgreen
  elif [ "$n" -ge 70 ]; then echo green
  elif [ "$n" -ge 60 ]; then echo yellowgreen
  elif [ "$n" -ge 50 ]; then echo yellow
  elif [ "$n" -ge 40 ]; then echo orange
  else                       echo red
  fi
}
for v in 0.0 39.9 40.0 49.9 50.0 59.9 60.0 69.9 70.0 79.9 80.0 100.0; do
  echo "$v -> $(color_for "$v")"
done
'
```
Expected output:
```
0.0 -> red
39.9 -> red
40.0 -> orange
49.9 -> orange
50.0 -> yellow
59.9 -> yellow
60.0 -> yellowgreen
69.9 -> yellowgreen
70.0 -> green
79.9 -> green
80.0 -> brightgreen
100.0 -> brightgreen
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -s -m "ci: publish per-service coverage badges from main

New coverage-badges job runs on push to main, downloads the per-service
coverage profiles, computes totals, maps to shields.io color, writes
.github/badges/coverage-<service>.json, and commits changes back to main
via git-auto-commit-action with [skip ci] to avoid loops. Uses the
built-in GITHUB_TOKEN; no PAT required."
```

---

## Task 7: Open PR and verify badges render

**Files:** none (verification only)

- [ ] **Step 1: Push the branch and open a PR**

```bash
git push -u origin <current-branch-name>
gh pr create --title "docs(readme): add badges (top block, per-service release + coverage, OSSF Scorecard)" --body "$(cat <<'EOF'
## Summary

- Adds a centered badge block below the H1 (CI, Go Reference, Go Report Card, Go version, License, OpenSSF Scorecard, Conventional Commits).
- Adds **Release** and **Coverage** columns to the Services table, one badge per service.
- Adds `.github/workflows/scorecard.yml` to publish OSSF Scorecard results.
- Extends `ci.yml` to generate per-service coverage profiles and publish badge JSON to `.github/badges/` on push to `main` via `GITHUB_TOKEN` (no PAT).

Spec: `docs/superpowers/specs/2026-04-13-readme-badges-design.md`
Plan: `docs/superpowers/plans/2026-04-13-readme-badges.md`

## Test plan

- [ ] GitHub web UI renders all 7 top badges with no 404 on the PR preview.
- [ ] Each per-service release and coverage badge renders in the Services table (coverage shows "pending" until first publish on `main`).
- [ ] `ci.yml` runs green on this PR (the `coverage-badges` job is skipped because it's gated on `push` to `main`).
- [ ] After merge: `coverage-badges` job succeeds on `main`, commits updated JSON with `[skip ci]`, no CI loop.
- [ ] Scorecard workflow: manually dispatch via `gh workflow run scorecard.yml`; job completes green and SARIF appears in the Security tab; within ~24h the top-block badge populates with a numeric score.
EOF
)"
```

- [ ] **Step 2: Wait for CI to finish on the PR**

```bash
gh pr checks --watch
```
Expected: all checks green except `coverage-badges` which should be **skipped** on the PR (its `if` condition excludes non-`main` events).

- [ ] **Step 3: Visually verify PR README preview**

Open the PR in the browser. Inspect the README tab. Confirm:
- All 7 top badges render (Scorecard may show "not yet analyzed" until the workflow has run — acceptable on the PR).
- Services table shows 7 columns with release + coverage badges rendered (coverage = "pending" grey).

- [ ] **Step 4: Merge the PR**

```bash
gh pr merge --squash --delete-branch
```

- [ ] **Step 5: Verify post-merge automation**

After the squash merge, monitor the `main` branch:

```bash
gh run list --workflow=ci.yml --branch=main --limit=1
gh run watch
```

Expected:
- `test` job passes.
- `coverage-badges` job runs (not skipped this time).
- A new commit from `github-actions[bot]` appears on `main` with message `chore(badges): update coverage [skip ci]`, modifying the six `.github/badges/coverage-*.json` files with real percentages.
- No follow-up CI run triggers from that commit (because of `[skip ci]`).

- [ ] **Step 6: Dispatch the Scorecard workflow once manually**

```bash
gh workflow run scorecard.yml --ref main
gh run watch
```

Expected: workflow completes green. Within ~24h, the top-block OSSF Scorecard badge renders a numeric score instead of "not yet analyzed".

- [ ] **Step 7: Final visual check on `main`**

Open `https://github.com/kaio6fellipe/event-driven-bookinfo` in the browser. Confirm:
- All 7 top badges render, including the Scorecard badge (score or "not yet analyzed").
- Release badges in the table show current tag for each service (or "no releases" if none exist yet).
- Coverage badges show real percentages with threshold colors.
