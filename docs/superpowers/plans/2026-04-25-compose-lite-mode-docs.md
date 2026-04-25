# Compose Lite-Mode Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a consistent "lite mode" disclaimer to `docker-compose.yml`, `README.md`, and `CLAUDE.md` making explicit that compose covers postgres + redis + 5 services only — Kafka, ingestion, Argo Events, and observability are k8s-only via `make run-k8s`.

**Architecture:** Three coordinated text edits in existing docs. No code, helm, or compose-service changes. Each edit reuses the same canonical wording so the message is consistent.

**Tech Stack:** Markdown + YAML comment text only.

**Spec reference:** `docs/superpowers/specs/2026-04-25-compose-lite-mode-docs-design.md`

**Repo:** `/Users/kaio.fellipe/Documents/git/others/go-http-server`
**Branch:** `feat/event-driven-notifications` (continuation of PR #54)

---

## File Structure

Three existing files modified, no new files.

```text
docker-compose.yml    # header comment block (lines 1-3) replaced with ~12-line lite-mode notice
README.md             # ~10-line callout under "Or use Docker Compose" (line 269) + ~5-line note in E2E section (line 568+)
CLAUDE.md             # ~9-line "Compose vs k8s scope" paragraph inserted between Build Commands and Helm Commands
```

Total diff: ~36 lines added, ~3 lines removed.

---

## Task 1: Update `docker-compose.yml` header comment

**Files:**

- Modify: `docker-compose.yml` (lines 1-3)

- [ ] **Step 1: Read current header**

```bash
head -10 /Users/kaio.fellipe/Documents/git/others/go-http-server/docker-compose.yml
```

Expected current first 3 lines:

```yaml
# Local development compose file.
# Usage: make run | make run-logs | make stop | make seed
#   or:  docker compose up --build
```

- [ ] **Step 2: Replace the header**

Replace the first 3 lines of `docker-compose.yml` with:

```yaml
# Local development compose file — lite mode.
#
# Scope: postgres + redis + 5 backend services + productpage.
# NOT included: Kafka, ingestion service, Argo Events, observability.
# Producers detect missing KAFKA_BROKERS and fall back to a no-op
# publisher; events are dropped silently.
#
# The full event-driven path (ingestion → Kafka → Argo Events sensor
# → notification HTTP trigger) requires `make run-k8s` (k3d cluster).
#
# Usage: make run | make run-logs | make stop | make seed
#   or:  docker compose up --build
```

The rest of the file is unchanged.

- [ ] **Step 3: Verify the file is still valid YAML**

Run:

```bash
docker compose -f /Users/kaio.fellipe/Documents/git/others/go-http-server/docker-compose.yml config >/dev/null
echo "compose config: $?"
```

Expected: `compose config: 0`. If non-zero, the YAML body was disturbed — revert and only edit the comment block.

- [ ] **Step 4: Commit**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git add docker-compose.yml
git commit -m "$(cat <<'EOF'
docs(compose): annotate docker-compose.yml as lite-mode dev environment

Postgres + redis + 5 backend services + productpage only. Kafka,
ingestion, Argo Events, and observability live in k8s (make run-k8s).
Producers detect missing KAFKA_BROKERS and fall back to a no-op
publisher; events are dropped silently.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Update `README.md` — `make run` section

**Files:**

- Modify: `README.md` (around line 269 — "Or use Docker Compose" section)

- [ ] **Step 1: Read the section**

```bash
sed -n '264,278p' /Users/kaio.fellipe/Documents/git/others/go-http-server/README.md
```

Expected output (lines 269-275 visible):

```markdown
**Or use Docker Compose** (PostgreSQL backend, all services with one command):

```bash
make run          # Start all services + PostgreSQL, seed databases
make run-logs     # Tail logs
make stop         # Stop and remove containers
```
```

- [ ] **Step 2: Insert the lite-mode callout block**

Find the line `make stop         # Stop and remove containers` followed by the closing fence ` ``` `. Immediately AFTER the closing fence, insert a blank line, then the callout, then another blank line:

```markdown
> **Lite mode.** Docker Compose runs the synchronous service mesh
> only: postgres + redis + 5 backend services + productpage. Kafka,
> the ingestion service, Argo Events, and the observability stack
> are NOT included. Producers detect missing `KAFKA_BROKERS` and
> fall back to a no-op publisher; events are dropped silently.
>
> The full event-driven path (ingestion polling Open Library →
> Kafka → Argo Events EventSource → Sensor → notification HTTP
> trigger) is exercised only via `make run-k8s` (k3d-based local
> cluster).
```

The result around lines 269-285 should read:

```markdown
**Or use Docker Compose** (PostgreSQL backend, all services with one command):

```bash
make run          # Start all services + PostgreSQL, seed databases
make run-logs     # Tail logs
make stop         # Stop and remove containers
```

> **Lite mode.** Docker Compose runs the synchronous service mesh
> only: postgres + redis + 5 backend services + productpage. Kafka,
> the ingestion service, Argo Events, and the observability stack
> are NOT included. Producers detect missing `KAFKA_BROKERS` and
> fall back to a no-op publisher; events are dropped silently.
>
> The full event-driven path (ingestion polling Open Library →
> Kafka → Argo Events EventSource → Sensor → notification HTTP
> trigger) is exercised only via `make run-k8s` (k3d-based local
> cluster).

---
```

(The trailing `---` is the existing horizontal rule that was already on line 277.)

- [ ] **Step 3: Verify markdown structure not broken**

Run a sanity check that the file still parses as expected by counting backtick-fenced blocks (should be unchanged from before the edit since we only added a blockquote, not a fenced block):

```bash
grep -c "^\`\`\`" /Users/kaio.fellipe/Documents/git/others/go-http-server/README.md
```

Expected: an even number (each fence has an open and close). Note the count before AND after; it must match.

> NOTE: do NOT commit yet. Task 3 also edits `README.md` — single commit covers both.

---

## Task 3: Update `README.md` — E2E Tests section

**Files:**

- Modify: `README.md` (around line 568 — "## E2E Tests" section)

- [ ] **Step 1: Read the section**

```bash
sed -n '566,584p' /Users/kaio.fellipe/Documents/git/others/go-http-server/README.md
```

Expected output (around lines 568-583):

```markdown
## E2E Tests

E2E tests spin up all five services via docker-compose and exercise each service's HTTP API with shell scripts.

```bash
# Run E2E tests (memory backend)
make e2e

# Run E2E tests with PostgreSQL backend
docker compose -f test/e2e/docker-compose.yml \
               -f test/e2e/docker-compose.postgres.yml up -d
bash test/e2e/run-tests.sh
```

Individual test scripts under `test/e2e/` cover health endpoints, CRUD operations, and cross-service integration (e.g., reviews fetching ratings).
```

- [ ] **Step 2: Append a lite-mode scope note after the "Individual test scripts ..." paragraph**

Find the line:

```markdown
Individual test scripts under `test/e2e/` cover health endpoints, CRUD operations, and cross-service integration (e.g., reviews fetching ratings).
```

Add an immediately-following blockquote, separated by one blank line:

```markdown
> **Scope.** `make e2e` covers HTTP-level acceptance tests
> (idempotency, validation, CRUD round-trips) under the lite-mode
> compose stack. The event chain (Kafka publish, Argo Events sensor,
> notification HTTP trigger) is verified end-to-end via Tempo trace
> inspection after `make run-k8s`.
```

The result around lines 582-590 should read:

```markdown
Individual test scripts under `test/e2e/` cover health endpoints, CRUD operations, and cross-service integration (e.g., reviews fetching ratings).

> **Scope.** `make e2e` covers HTTP-level acceptance tests
> (idempotency, validation, CRUD round-trips) under the lite-mode
> compose stack. The event chain (Kafka publish, Argo Events sensor,
> notification HTTP trigger) is verified end-to-end via Tempo trace
> inspection after `make run-k8s`.

---
```

(The trailing `---` is the existing horizontal rule that was already on line 584.)

- [ ] **Step 3: Verify the markdown still has even backtick-fenced count**

```bash
grep -c "^\`\`\`" /Users/kaio.fellipe/Documents/git/others/go-http-server/README.md
```

Expected: same even number as after Task 2 Step 3 (we added blockquotes only, no fenced blocks).

- [ ] **Step 4: Commit (covers Task 2 and Task 3 together)**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git add README.md
git commit -m "$(cat <<'EOF'
docs(readme): annotate compose + e2e as lite-mode

Adds two blockquote callouts: one under the docker-compose run section
explaining the lite-mode scope (no Kafka/ingestion/Argo Events/
observability) and pointing at make run-k8s for the full event-driven
path; one under the E2E Tests section noting that make e2e covers
HTTP-level tests only and the event chain is verified via Tempo
traces after make run-k8s.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Update `CLAUDE.md` — Compose vs k8s scope paragraph

**Files:**

- Modify: `CLAUDE.md` (insert between line 45 — end of `## Build Commands` block — and line 47 — `## Helm Commands` heading)

- [ ] **Step 1: Read the section**

```bash
sed -n '37,55p' /Users/kaio.fellipe/Documents/git/others/go-http-server/CLAUDE.md
```

Expected output (lines 37-54):

```markdown
## Build Commands

```bash
make build-all          # Build all 7 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 7 Docker images
```

## Helm Commands

```bash
helm dependency build charts/bookinfo-service  # Fetch subchart dependencies (run once after clone)
make helm-lint            # Lint chart with all per-service values files
make helm-template SERVICE=ratings  # Dry-run render for a specific service
helm upgrade --install ratings charts/bookinfo-service -f deploy/ratings/values-local.yaml -n bookinfo
```
```

- [ ] **Step 2: Insert the scope paragraph between Build Commands and Helm Commands**

Find the closing fence ` ``` ` of the Build Commands block (line 45). After that fence and the existing blank line at line 46, BEFORE the `## Helm Commands` heading at line 47, insert a new section:

```markdown
## Compose vs k8s scope

`make run` (docker-compose) is a lite development environment —
postgres + redis + 5 backend services + productpage. Compose does
NOT include Kafka, the ingestion service, Argo Events, or the
observability stack. Producers fall back to a no-op publisher when
`KAFKA_BROKERS` is unset; events are dropped. The full event-driven
flow (ingestion + Kafka + Argo Events sensors driving notifications)
and observability stack are exercised only via `make run-k8s`. Use
`make e2e` for HTTP-level acceptance tests; the event chain is
verified via Tempo traces after `make run-k8s`.

```

The result around lines 37-58 (after edit) should read:

```markdown
## Build Commands

```bash
make build-all          # Build all 7 service binaries to bin/
make test               # go test -race -count=1 ./...
make lint               # golangci-lint run
make e2e                # Docker Compose + shell smoke tests (memory backend)
make docker-build-all   # Build all 7 Docker images
```

## Compose vs k8s scope

`make run` (docker-compose) is a lite development environment —
postgres + redis + 5 backend services + productpage. Compose does
NOT include Kafka, the ingestion service, Argo Events, or the
observability stack. Producers fall back to a no-op publisher when
`KAFKA_BROKERS` is unset; events are dropped. The full event-driven
flow (ingestion + Kafka + Argo Events sensors driving notifications)
and observability stack are exercised only via `make run-k8s`. Use
`make e2e` for HTTP-level acceptance tests; the event chain is
verified via Tempo traces after `make run-k8s`.

## Helm Commands

```bash
...
```
```

- [ ] **Step 3: Verify markdown structure**

```bash
grep -c "^\`\`\`" /Users/kaio.fellipe/Documents/git/others/go-http-server/CLAUDE.md
```

Expected: same even number as before the edit (no fenced blocks added/removed).

```bash
grep -c "^## " /Users/kaio.fellipe/Documents/git/others/go-http-server/CLAUDE.md
```

Expected: incremented by exactly 1 vs before the edit (we added one new `##` heading).

- [ ] **Step 4: Commit**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(claude): document compose vs k8s scope boundary

Inserts a Compose vs k8s scope section between Build Commands and
Helm Commands explaining that make run (docker-compose) is the lite
dev path (postgres + redis + 5 services) and the full event-driven
flow (Kafka, ingestion, Argo Events, observability) is exercised
only via make run-k8s. Producers fall back to a no-op publisher
when KAFKA_BROKERS is unset.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Push to PR #54

- [ ] **Step 1: Verify branch + clean state**

```bash
cd /Users/kaio.fellipe/Documents/git/others/go-http-server
git branch --show-current
git status
```

Expected: on `feat/event-driven-notifications`, clean working tree.

- [ ] **Step 2: Inspect the three new commits**

```bash
git log --oneline -3
```

Expected (top to bottom):

1. `docs(claude): document compose vs k8s scope boundary`
2. `docs(readme): annotate compose + e2e as lite-mode`
3. `docs(compose): annotate docker-compose.yml as lite-mode dev environment`

- [ ] **Step 3: Push**

```bash
git push
```

Expected: branch updated; PR #54 picks up the three new docs commits.

- [ ] **Step 4: Watch CI**

```bash
gh pr checks 54 --watch --interval 30 --fail-fast
```

Expected: all checks green. Docs-only changes shouldn't trip any lint or test failures, but the chart-version-bump CI job from earlier may run on any PR push — should pass since chart files are untouched.

---

## Self-Review

**Spec coverage:**

- Decision 1 (document, do not extend compose) → all four tasks are docs-only ✔
- Decision 2 (same wording across all three files) → Tasks 1, 2/3, 4 reuse the canonical phrasing about scope, NoopPublisher fallback, and pointer to `make run-k8s` ✔
- Decision 3 (mention NoopPublisher behavior explicitly) → Tasks 1, 2, 4 all include "fall back to a no-op publisher; events are dropped" ✔
- Decision 4 (reference `make run-k8s` as the full-chain path) → Tasks 1, 2, 4 all point at `make run-k8s` ✔
- Component: `docker-compose.yml` header → Task 1 ✔
- Component: README.md run + e2e callouts → Tasks 2 + 3 ✔
- Component: CLAUDE.md scope paragraph → Task 4 ✔
- Acceptance criteria 1-5 → Tasks 1 (criterion 1), 2-3 (criterion 2), 4 (criterion 3), wording-consistency check is implicit in reuse (criterion 4), no code/helm/service edits (criterion 5) ✔

**Placeholder scan:** no "TBD", "TODO", "implement later", "similar to". The "verify the markdown still has even backtick-fenced count" instruction is a concrete sanity check, not a placeholder.

**Type consistency:** N/A — no types or method signatures involved. Wording consistency is the analogous concern: the phrases "Kafka, the ingestion service, Argo Events, and the observability stack", "no-op publisher", "events are dropped", and "make run-k8s" appear identically across Tasks 1, 2, and 4. Verified.
