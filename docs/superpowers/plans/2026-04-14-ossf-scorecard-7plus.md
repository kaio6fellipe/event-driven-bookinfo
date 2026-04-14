# OSSF Scorecard 7+ Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Raise OSSF Scorecard from 6.9 to 7.0+ and add a PR quality gate that comments the score and blocks merges below threshold.

**Architecture:** Five targeted fixes to existing workflow files and GitHub settings, plus one new workflow for PR-level scorecard checking. All code changes ship in a single PR; ruleset updates apply post-merge via `gh api`.

**Tech Stack:** GitHub Actions YAML, OSSF Scorecard CLI v5.4.0, `actions/github-script` for PR commenting, `gh` CLI for ruleset updates.

**Spec:** `docs/superpowers/specs/2026-04-14-ossf-scorecard-7plus-design.md`

---

### Task 1: Fix helm-release.yml Token Permissions

**Files:**
- Modify: `.github/workflows/helm-release.yml:11-12`

- [ ] **Step 1: Move permissions from workflow level to job level**

In `.github/workflows/helm-release.yml`, replace the top-level `permissions` block and add job-level permissions:

```yaml
# Line 11-12: replace
permissions:
  contents: write

# With
permissions: {}
```

And add `permissions` to the job:

```yaml
# Line 14-16: replace
jobs:
  release:
    name: Release Chart
    runs-on: ubuntu-latest

# With
jobs:
  release:
    name: Release Chart
    runs-on: ubuntu-latest
    permissions:
      contents: write
```

- [ ] **Step 2: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/helm-release.yml'))" && echo "YAML valid"`

Expected: `YAML valid`

- [ ] **Step 3: Verify all workflows have restrictive top-level permissions**

Run: `for f in .github/workflows/*.yml; do echo "=== $f ==="; head -15 "$f" | grep -A2 "^permissions"; done`

Expected: Every workflow shows either `permissions: {}` or `permissions: read-all` at the top level. No workflow has write permissions at the top level.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/helm-release.yml
git commit -m "fix(ci): scope helm-release permissions to job level

Scorecard Token-Permissions check requires write permissions
scoped to individual jobs, not at the workflow level."
```

---

### Task 2: Pin govulncheck Version

**Files:**
- Modify: `.github/workflows/ci.yml:125`

- [ ] **Step 1: Pin govulncheck to v1.2.0**

In `.github/workflows/ci.yml`, replace line 125:

```yaml
# Before
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest

# After
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@v1.2.0
```

- [ ] **Step 2: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML valid"`

Expected: `YAML valid`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "fix(ci): pin govulncheck to v1.2.0

Scorecard Pinned-Dependencies check flags @latest as unpinned.
Dependabot github-actions ecosystem will keep this updated."
```

---

### Task 3: Add CODEOWNERS File

**Files:**
- Create: `.github/CODEOWNERS`

- [ ] **Step 1: Create CODEOWNERS file**

Create `.github/CODEOWNERS` with the following content:

```
# Default code owners for all files
* @kaio6fellipe
```

- [ ] **Step 2: Verify file is recognized by GitHub's CODEOWNERS parser**

Run: `cat .github/CODEOWNERS`

Expected: The file contains `* @kaio6fellipe` — a single rule matching all files, assigning the repository owner.

- [ ] **Step 3: Commit**

```bash
git add .github/CODEOWNERS
git commit -m "chore: add CODEOWNERS for branch protection scoring

Scorecard Branch-Protection check requires a CODEOWNERS file
and codeowner review enforcement in the ruleset."
```

---

### Task 4: Add Scorecard PR Check Workflow

**Files:**
- Create: `.github/workflows/scorecard-pr.yml`

- [ ] **Step 1: Create the workflow file**

Create `.github/workflows/scorecard-pr.yml`:

```yaml
name: Scorecard PR Check

on:
  pull_request:
    branches:
      - main

permissions: {}

jobs:
  scorecard-check:
    name: Scorecard Check
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
      issues: read
      checks: read
    env:
      SCORECARD_VERSION: "5.4.0"
      SCORECARD_CHECKSUM: "e5183aeaa5aa548fbb7318a6deb3e1038be0ef9aca24e655422ae88dfbe67502"
      SCORE_THRESHOLD: "7.0"
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
        with:
          persist-credentials: false

      - name: Install scorecard CLI
        run: |
          set -euo pipefail
          TARBALL="scorecard_${SCORECARD_VERSION}_linux_amd64.tar.gz"
          curl -sLO "https://github.com/ossf/scorecard/releases/download/v${SCORECARD_VERSION}/${TARBALL}"
          echo "${SCORECARD_CHECKSUM}  ${TARBALL}" | sha256sum --check --strict
          tar xzf "${TARBALL}" scorecard
          chmod +x scorecard
          sudo mv scorecard /usr/local/bin/scorecard
          rm "${TARBALL}"

      - name: Run scorecard
        id: scorecard
        env:
          GITHUB_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          set -euo pipefail
          scorecard --repo="github.com/${{ github.repository }}" \
            --format=json --show-details > scorecard.json
          SCORE=$(jq -r '.score' scorecard.json)
          echo "score=${SCORE}" >> "$GITHUB_OUTPUT"
          echo "Scorecard overall score: ${SCORE}"

      - name: Comment on PR
        uses: actions/github-script@60a0d83039c74a4aee543508d2ffcb1c3799cdea # v7
        with:
          script: |
            const fs = require('fs');
            const data = JSON.parse(fs.readFileSync('scorecard.json', 'utf8'));
            const score = data.score;
            const threshold = parseFloat('${{ env.SCORE_THRESHOLD }}');
            const passed = score >= threshold;
            const icon = passed ? ':white_check_mark:' : ':x:';
            const repo = '${{ github.repository }}';

            // Build check table sorted by name
            const checks = data.checks
              .sort((a, b) => a.name.localeCompare(b.name))
              .map(c => {
                const s = c.score === -1 ? 'N/A' : `${c.score}/10`;
                // Truncate reason to 80 chars
                const reason = (c.reason || '').length > 80
                  ? c.reason.substring(0, 77) + '...'
                  : (c.reason || '');
                return `| ${c.name} | ${s} | ${reason} |`;
              })
              .join('\n');

            let body = `## OpenSSF Scorecard — ${score}/10 ${icon}\n\n`;
            body += `| Check | Score | Details |\n`;
            body += `|-------|-------|---------|`;
            body += `\n${checks}\n\n`;

            if (!passed) {
              body += `> :rotating_light: Score ${score} is below threshold ${threshold} — this check will fail.\n\n`;
            }

            body += `> Threshold: ${threshold} | [Full report](https://securityscorecards.dev/viewer/?uri=github.com/${repo})\n`;

            // Marker to find/update this comment
            const marker = '<!-- ossf-scorecard-pr-check -->';
            body = marker + '\n' + body;

            // Find existing comment
            const { data: comments } = await github.rest.issues.listComments({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number,
            });
            const existing = comments.find(c => c.body.includes(marker));

            if (existing) {
              await github.rest.issues.updateComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                comment_id: existing.id,
                body: body,
              });
            } else {
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.issue.number,
                body: body,
              });
            }

      - name: Enforce threshold
        run: |
          SCORE="${{ steps.scorecard.outputs.score }}"
          THRESHOLD="${{ env.SCORE_THRESHOLD }}"
          if [ "$(echo "${SCORE} < ${THRESHOLD}" | bc -l)" -eq 1 ]; then
            echo "::error::OpenSSF Scorecard score ${SCORE} is below threshold ${THRESHOLD}"
            exit 1
          fi
          echo "Scorecard score ${SCORE} meets threshold ${THRESHOLD}"
```

- [ ] **Step 2: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/scorecard-pr.yml'))" && echo "YAML valid"`

Expected: `YAML valid`

- [ ] **Step 3: Verify permissions follow the project pattern**

Run: `head -12 .github/workflows/scorecard-pr.yml`

Expected: Top-level `permissions: {}` with write permissions scoped to the job.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/scorecard-pr.yml
git commit -m "feat(ci): add scorecard PR quality gate

Runs OSSF Scorecard CLI on every PR, comments the score
breakdown on the PR, and fails the check if below 7.0.

Uses scorecard v5.4.0 with SHA256 checksum verification."
```

---

### Task 5: Update Ruleset via GitHub API

This task runs AFTER the PR from Tasks 1-4 is merged, to avoid blocking that PR itself with the new requirements.

**Files:** None (GitHub API calls only)

- [ ] **Step 1: Enable codeowner review requirement**

Run:

```bash
gh api --method PUT repos/kaio6fellipe/event-driven-bookinfo/rulesets/14813558 \
  --input - <<'EOF'
{
  "name": "main",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "exclude": [],
      "include": ["~DEFAULT_BRANCH"]
    }
  },
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"},
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "required_reviewers": [],
        "require_code_owner_review": true,
        "require_last_push_approval": true,
        "required_review_thread_resolution": false,
        "allowed_merge_methods": ["squash"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          {"context": "E2E Tests", "integration_id": 15368},
          {"context": "E2E Tests (PostgreSQL)", "integration_id": 15368},
          {"context": "Lint", "integration_id": 15368},
          {"context": "Vet", "integration_id": 15368},
          {"context": "Test", "integration_id": 15368},
          {"context": "Build", "integration_id": 15368},
          {"context": "Docker Build + Scan", "integration_id": 15368},
          {"context": "Analyze Go", "integration_id": 15368}
        ]
      }
    }
  ],
  "bypass_actors": [
    {"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "pull_request"}
  ]
}
EOF
```

Expected: HTTP 200 response with the updated ruleset JSON showing `require_code_owner_review: true` and `Analyze Go` in the required status checks.

- [ ] **Step 2: Verify the ruleset update**

Run:

```bash
gh api repos/kaio6fellipe/event-driven-bookinfo/rulesets/14813558 \
  --jq '.rules[] | select(.type == "pull_request") | .parameters.require_code_owner_review'
```

Expected: `true`

- [ ] **Step 3: Verify CodeQL is a required check**

Run:

```bash
gh api repos/kaio6fellipe/event-driven-bookinfo/rulesets/14813558 \
  --jq '.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context'
```

Expected: Output includes `Analyze Go` alongside the existing checks.

---

### Task 6: Verify Score Improvement

This task runs after the PR is merged and the ruleset is updated.

- [ ] **Step 1: Trigger a fresh scorecard analysis**

Run:

```bash
gh workflow run "OSSF Scorecard" --ref main
```

Expected: Workflow dispatched successfully.

- [ ] **Step 2: Wait for the scorecard run to complete**

Run:

```bash
gh run list --workflow=scorecard.yml --limit=1
```

Expected: Most recent run shows `completed` with `success` status.

- [ ] **Step 3: Verify the score on the public dashboard**

Check: `https://securityscorecards.dev/viewer/?uri=github.com/kaio6fellipe/event-driven-bookinfo`

Expected: Overall score >= 7.0. Specific improvements:
- Token-Permissions: 10/10 (was 0)
- Pinned-Dependencies: 9/10 (was 8)
- Branch-Protection: 10/10 (was 8)

Note: SAST will improve gradually as new commits accumulate CodeQL results. Vulnerabilities stays at 8/10 until upstream pgx fix is released (GO-2026-4771, GO-2026-4772 have `Fixed in: N/A`).
