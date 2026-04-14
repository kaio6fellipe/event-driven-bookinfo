# OSSF Scorecard 7+ Hardening — Design Spec

**Date:** 2026-04-14
**Baseline:** 6.9/10 (after initial hardening from 3.6)
**Goal:** Raise OSSF Scorecard to 7.0+ and add a PR quality gate
**Approach:** Max all remaining fixable checks; add scorecard PR check workflow

## Current State (6.9/10)

| Check | Current | Target | Action |
|-------|---------|--------|--------|
| Token-Permissions | 0 | 10 | Fix `helm-release.yml` top-level permissions |
| Pinned-Dependencies | 8 | 9 | Pin `govulncheck@latest` to specific version |
| Branch-Protection | 8 | 10 | Add CODEOWNERS + enable codeowner review |
| Vulnerabilities | 8 | 10 | Update Go deps for GO-2026-4771/4772 |
| SAST | 7 | 10 | Add CodeQL as required status check (improves over time) |
| Code-Review | 0 | 0 | No change — solo project |
| Maintained | 0 | 0 | No change — time-based, auto-resolves |
| CII-Best-Practices | 0 | 0 | No change — not needed for 7+ |
| Contributors | 3 | 3 | No change — requires multi-org contributions |
| All others | 10 | 10 | Already maxed |

**Projected: ~7.6-7.9/10** (143/18 = 7.9 assuming all targets hit)

## Section 1: Workflow Permission Fix (Token-Permissions 0 -> 10)

### Problem

`helm-release.yml` has `permissions: contents: write` at the workflow level. Scorecard requires write permissions scoped to job level, not workflow level.

### Fix

Move permissions to job level, set top-level to `{}`:

```yaml
# Before
permissions:
  contents: write

jobs:
  release:
    steps: ...

# After
permissions: {}

jobs:
  release:
    permissions:
      contents: write
    steps: ...
```

All other workflows already follow this pattern correctly.

## Section 2: Pin govulncheck Version (Pinned-Dependencies 8 -> 9)

### Problem

`ci.yml` uses `go install golang.org/x/vuln/cmd/govulncheck@latest` — scorecard flags `@latest` as an unpinned dependency.

### Fix

Pin to a specific version:

```bash
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
```

### Not Fixable

`slsa-framework/slsa-github-generator@v2.1.0` is a reusable workflow that cannot be SHA-pinned. The SLSA generator uses the tag ref for OIDC token verification — their documentation explicitly prohibits SHA pinning. This is an accepted limitation; target is 9/10, not 10/10.

## Section 3: CODEOWNERS + Branch Protection (Branch-Protection 8 -> 10)

### Problem

Scorecard flags: "requires only 1 approval, lacks codeowner requirement."

### Fix — CODEOWNERS File

Create `.github/CODEOWNERS`:

```
# Default owners for everything
* @kaio6fellipe
```

### Fix — Ruleset Update

Update the existing `main` ruleset (ID: 14813558) via GitHub API to set `require_code_owner_review: true`.

Current ruleset pull_request rule parameters:
- `required_approving_review_count: 1` — keep
- `dismiss_stale_reviews_on_push: true` — keep
- `require_code_owner_review: false` — change to `true`
- `require_last_push_approval: true` — keep
- `allowed_merge_methods: ["squash"]` — keep

The 1-approval requirement stays — increasing to 2 would block a solo maintainer. CODEOWNERS + codeowner review is what scorecard specifically checks for.

## Section 4: Go Vulnerability Fixes (Vulnerabilities 8 -> 10)

### Problem

Two known unfixed vulnerabilities: `GO-2026-4771` and `GO-2026-4772`.

### Fix

```bash
govulncheck ./...          # identify affected packages
go get -u <affected-deps>  # update specific vulnerable modules
go mod tidy                # clean up
```

The `govulncheck` CI job validates the fix — it runs on every PR and fails on known vulns.

## Section 5: SAST Coverage (SAST 7 -> 10)

### Problem

"CodeQL configured but only covers 9 of 30 commits." The 30-commit window includes older commits that predate the CodeQL workflow.

### Fix

Add `Analyze Go` (the CodeQL job name) as a required status check in the `main` ruleset. This ensures every future PR has a CodeQL run before merge.

No workflow file changes needed — the `codeql.yml` workflow already triggers on `push: main` and `pull_request: main`.

The SAST score will improve naturally as new commits with CodeQL results replace old uncovered commits in the 30-commit window. After ~21 more PRs merge, the ratio reaches 30/30.

## Section 6: Scorecard PR Check Workflow (Quality Gate)

### Purpose

Prevent score regressions by running the scorecard CLI on every PR and failing if the score drops below 7.0.

### Workflow: `.github/workflows/scorecard-pr.yml`

**Trigger:** `pull_request` targeting `main`

**Permissions:**
```yaml
permissions: {}

jobs:
  scorecard-check:
    permissions:
      contents: read
      pull-requests: write   # comment on PR
      issues: read           # scorecard reads issue data
      checks: read           # scorecard reads check results
```

**Steps:**

1. **Checkout** — `actions/checkout` with `persist-credentials: false`

2. **Install scorecard CLI** — download v5.4.0 binary for linux/amd64, verify SHA256 checksum against published `scorecard_checksums.txt`, extract to PATH.

3. **Run scorecard** — execute against the GitHub API (not local):
   ```bash
   GITHUB_AUTH_TOKEN=${{ secrets.GITHUB_TOKEN }} \
     scorecard --repo=github.com/${{ github.repository }} \
     --format=json --show-details > scorecard.json
   ```
   Extract overall score to job output.

4. **Comment on PR** — use `actions/github-script` to:
   - Parse `scorecard.json`
   - Build markdown table with all checks, scores, and truncated reasons
   - Post/update a single PR comment (use comment marker to avoid duplicates)
   - Include pass/fail indicator and link to full report on securityscorecards.dev

5. **Enforce threshold** — fail the job if overall score < threshold (default 7.0, configurable via `workflow_dispatch` input).

### PR Comment Format

```markdown
## OpenSSF Scorecard — 7.9/10 :white_check_mark:

| Check | Score | Details |
|-------|-------|---------|
| Binary-Artifacts | 10/10 | no binary artifacts found |
| Branch-Protection | 10/10 | branch protection settings found |
| CI-Tests | 10/10 | 30 out of 30 merged PRs checked |
| ... | ... | ... |

> Threshold: 7.0 | [Full report](https://securityscorecards.dev/viewer/?uri=github.com/kaio6fellipe/event-driven-bookinfo)
```

When score < threshold:
```markdown
## OpenSSF Scorecard — 6.5/10 :x:

...same table...

> :rotating_light: Score 6.5 is below threshold 7.0 — this check will fail.
```

### Caveat

The scorecard scores the current repo state (main branch + GitHub settings), not the post-merge state. This acts as a quality gate: if the score is below 7, no PRs can merge until repo-level fixes bring it back up. It cannot preview what a specific PR's diff would do to the score.

### Rate Limiting

The scorecard CLI makes GitHub API calls. To avoid rate limiting on repos with high PR volume, the workflow only runs on PRs targeting `main` (not feature-to-feature branches).

## Implementation Order

1. Fix `helm-release.yml` permissions (trivial, unblocks Token-Permissions)
2. Pin `govulncheck` version in `ci.yml`
3. Add `.github/CODEOWNERS`
4. Update Go dependencies for vulnerability fixes
5. Add scorecard PR check workflow (`scorecard-pr.yml`)
6. Update ruleset via GitHub API (codeowner review + CodeQL required check)

Steps 1-5 are code changes in a single PR. Step 6 is a GitHub API call executed after the PR merges (to avoid blocking the PR itself with the new requirements).

## Out of Scope

- **CII-Best-Practices badge:** Requires external questionnaire at bestpractices.coreinfrastructure.org — not needed for 7+
- **Code-Review:** Solo project, no second reviewer available. Score stays at 0.
- **Maintained:** Time-based check, auto-resolves after 90 days of activity
- **Contributors:** Requires contributions from multiple organizations
- **SLSA generator SHA pinning:** Reusable workflow cannot be SHA-pinned per SLSA project requirements
