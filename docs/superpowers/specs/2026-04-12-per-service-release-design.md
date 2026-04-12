# Per-Service Independent Releases with GoReleaser

**Date:** 2026-04-12
**Status:** Draft
**Scope:** Release pipeline — GoReleaser configs, GitHub Actions workflows, documentation

## Problem

Current release setup uses a single `.goreleaser.yaml` and a single `v*` tag to release all 5 services simultaneously. Services evolve independently and need independent versioning and releases.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| GoReleaser config structure | Per-service `.goreleaser.yaml` files | Idiomatic GoReleaser monorepo pattern for OSS; clean separation |
| Release trigger | `workflow_dispatch` (not tag-push) | OSS GoReleaser cannot trigger workflows from `GITHUB_TOKEN`-created tags; `workflow_dispatch` avoids PAT requirement |
| Auto-tagging | GitHub Actions workflow on PR merge | No external tooling; conventional commits + label override |
| Version bump detection | Hybrid: PR labels first, then conventional commits, default patch | Automatic 90% of the time, explicit override when needed |
| Version source of truth | Git tags | No version files to maintain or drift |
| `pkg/` change handling | Triggers release for ALL services | Shared packages affect all consumers; conservative and safe |
| GoReleaser edition | OSS (free) | Pro `monorepo` stanza not needed; `GORELEASER_CURRENT_TAG` env var provides the required tag override |

## Tag Convention

Format: `<service>-v<major>.<minor>.<patch>`

Examples:
- `details-v0.1.0`
- `reviews-v1.2.3`
- `productpage-v0.3.0`

Services: `productpage`, `details`, `reviews`, `ratings`, `notification`

## Version Resolution

1. Query latest tag: `git tag -l "<service>-v*" --sort=-v:refversion | head -1`
2. If no tag exists, default to `v0.0.0`
3. Increment based on bump type

## Bump Type Detection (Hybrid)

Priority order:
1. **PR labels:** `major` label → major bump, `minor` label → minor bump
2. **Conventional commits:** scan PR commit messages
   - `BREAKING CHANGE` in body or `!:` in subject → major
   - `feat(...)` → minor
   - `fix(...)`, `chore(...)`, everything else → patch
3. **Default:** patch (no matching commits, no labels)

## Workflow Architecture

### `.github/workflows/auto-tag.yml` — Triggered on PR Merge

**Trigger:** `pull_request: closed` + merged to `main`

**Steps:**
1. **Detect changed services** — diff PR commits against `main`:
   - `services/<name>/**` → that service
   - `pkg/**`, `go.mod`, `go.sum` → ALL 5 services
2. **Determine bump type** per service — hybrid logic (labels → commits → patch)
3. **Resolve current version** per service — query latest git tag
4. **Compute next version** — increment major/minor/patch
5. **Push tags** — `<service>-v<next>` using default `GITHUB_TOKEN`
6. **Trigger release** — call `release.yml` via `workflow_dispatch` per affected service

### `.github/workflows/release.yml` — Triggered by Dispatch

**Trigger:** `workflow_dispatch` with inputs:
- `service` (choice: productpage, details, reviews, ratings, notification)
- `tag` (string, e.g., `details-v0.1.0`)

**Steps:**
1. Checkout with full history (`fetch-depth: 0`)
2. Setup Go, QEMU, Docker Buildx, GHCR login
3. Strip service prefix from tag: `details-v0.1.0` → `v0.1.0`
4. Run GoReleaser:
   ```bash
   goreleaser release --clean -f services/<service>/.goreleaser.yaml
   ```
   With env: `GORELEASER_CURRENT_TAG=v0.1.0`

Also callable manually from GitHub UI for reruns or hotfixes.

## Per-Service GoReleaser Config

Each service gets `services/<name>/.goreleaser.yaml`:

```yaml
version: 2
project_name: <service>

builds:
  - id: <service>
    binary: <service>
    main: ./services/<service>/cmd/
    env: [CGO_ENABLED=0]
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags: [-s -w]

archives:
  - id: <service>
    builds: [<service>]
    name_template: "<service>_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: <service>-amd64
    ids: [<service>]
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.<service>
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=<service>"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: <service>-arm64
    ids: [<service>]
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.<service>
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=<service>"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/<service>:{{ .Tag }}-arm64"

changelog:
  use: github
```

**Note:** `main` uses `./services/<service>/cmd/` because GoReleaser runs from the repository root regardless of the `-f` flag location.

## GitHub Labels

Created via `gh label create`:
- `major` — triggers major version bump (red)
- `minor` — triggers minor version bump (yellow)

## Validation Phase — First Release (v0.1.0)

After implementation, create the initial `v0.1.0` release for each service as a smoke test. This validates the full pipeline end-to-end.

### Step 1: Manual Dispatch Test (Single Service)

Pick one service (e.g., `details`) and test the release workflow manually:

```bash
# Create the initial tag locally
git tag details-v0.1.0
git push origin details-v0.1.0

# Trigger release via workflow_dispatch
gh workflow run release.yml -f service=details -f tag=details-v0.1.0

# Watch the workflow run
gh run watch
```

### Step 2: Verify Release Artifacts

```bash
# Check workflow completed successfully
gh run list --workflow=release.yml --limit=1

# Verify the GitHub release was created
gh release view details-v0.1.0

# Verify release assets (archives)
gh release view details-v0.1.0 --json assets --jq '.assets[].name'

# Verify Docker images were pushed to GHCR
gh api user/packages/container/event-driven-bookinfo%2Fdetails/versions --jq '.[0].metadata.container.tags'
```

### Step 3: Release Remaining Services

Once `details` validates successfully, release the other 4 services:

```bash
for svc in productpage reviews ratings notification; do
  git tag "${svc}-v0.1.0"
  git push origin "${svc}-v0.1.0"
  gh workflow run release.yml -f service="${svc}" -f tag="${svc}-v0.1.0"
  sleep 5  # avoid rate limiting
done

# Watch all runs
gh run list --workflow=release.yml --limit=5
```

### Step 4: Verify All Tags and Releases

```bash
# All 5 tags exist
git tag -l "*-v0.1.0"

# All 5 GitHub releases exist
gh release list --limit=5

# All Docker images pushed
for svc in productpage details reviews ratings notification; do
  echo "=== ${svc} ==="
  gh api user/packages/container/event-driven-bookinfo%2F${svc}/versions --jq '.[0].metadata.container.tags' 2>/dev/null || echo "not found"
done
```

### Step 5: Auto-Tag Workflow Test

Create a test PR that touches one service, merge it, and verify:

```bash
# After PR merge, check auto-tag workflow ran
gh run list --workflow=auto-tag.yml --limit=1

# Verify new tag was created (e.g., details-v0.1.1)
git fetch --tags
git tag -l "details-v*"

# Verify release workflow was dispatched
gh run list --workflow=release.yml --limit=1
```

### Rollback

If validation fails:

```bash
# Delete a bad release
gh release delete <service>-v0.1.0 --yes

# Delete the tag
git push --delete origin <service>-v0.1.0
git tag -d <service>-v0.1.0
```

## Files Changed

| File | Action |
|------|--------|
| `services/productpage/.goreleaser.yaml` | Create |
| `services/details/.goreleaser.yaml` | Create |
| `services/reviews/.goreleaser.yaml` | Create |
| `services/ratings/.goreleaser.yaml` | Create |
| `services/notification/.goreleaser.yaml` | Create |
| `.goreleaser.yaml` (root) | Delete |
| `.github/workflows/release.yml` | Rewrite |
| `.github/workflows/auto-tag.yml` | Create |
| `CLAUDE.md` | Add release process section |
| `README.md` | Add releasing section |
| `.claude/rules/release.md` | Create |
