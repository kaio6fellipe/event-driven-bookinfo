# Per-Service Independent Releases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate from a single monolithic GoReleaser release to independent per-service releases with automated tagging on PR merge.

**Architecture:** Per-service `.goreleaser.yaml` configs, a `workflow_dispatch`-based release workflow, and an auto-tag workflow triggered on PR merge. Uses `GORELEASER_CURRENT_TAG` env var (GoReleaser OSS) to handle prefixed tags.

**Tech Stack:** GoReleaser v2 (OSS), GitHub Actions, GitHub CLI (`gh`), conventional commits

**Spec:** `docs/superpowers/specs/2026-04-12-per-service-release-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `services/productpage/.goreleaser.yaml` | Create | GoReleaser config for productpage only |
| `services/details/.goreleaser.yaml` | Create | GoReleaser config for details only |
| `services/reviews/.goreleaser.yaml` | Create | GoReleaser config for reviews only |
| `services/ratings/.goreleaser.yaml` | Create | GoReleaser config for ratings only |
| `services/notification/.goreleaser.yaml` | Create | GoReleaser config for notification only |
| `.goreleaser.yaml` | Delete | Replaced by per-service configs |
| `.github/workflows/release.yml` | Rewrite | `workflow_dispatch` release for a single service |
| `.github/workflows/auto-tag.yml` | Create | Auto-tag + dispatch on PR merge |
| `.claude/rules/release.md` | Create | Release convention rules for Claude Code |
| `CLAUDE.md` | Modify | Add release process section |
| `README.md` | Modify | Update release docs, project structure tree |

---

### Task 1: Create GitHub Labels

**Files:** None (GitHub API only)

- [ ] **Step 1: Create `major` label**

```bash
gh label create major --description "Triggers major version bump on release" --color "D73A4A" --repo kaio6fellipe/event-driven-bookinfo
```

Expected: `Label "major" created`

- [ ] **Step 2: Create `minor` label**

```bash
gh label create minor --description "Triggers minor version bump on release" --color "FBCA04" --repo kaio6fellipe/event-driven-bookinfo
```

Expected: `Label "minor" created`

- [ ] **Step 3: Verify labels exist**

```bash
gh label list --repo kaio6fellipe/event-driven-bookinfo | grep -E "^(major|minor)"
```

Expected: Both labels listed with their colors.

- [ ] **Step 4: Commit — N/A (no file changes)**

---

### Task 2: Create Per-Service GoReleaser Configs

**Files:**
- Create: `services/productpage/.goreleaser.yaml`
- Create: `services/details/.goreleaser.yaml`
- Create: `services/reviews/.goreleaser.yaml`
- Create: `services/ratings/.goreleaser.yaml`
- Create: `services/notification/.goreleaser.yaml`
- Delete: `.goreleaser.yaml` (root)

- [ ] **Step 1: Create `services/productpage/.goreleaser.yaml`**

```yaml
version: 2
project_name: productpage

builds:
  - id: productpage
    binary: productpage
    main: ./services/productpage/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: productpage
    builds:
      - productpage
    name_template: "productpage_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: productpage-amd64
    ids:
      - productpage
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.productpage
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=productpage"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: productpage-arm64
    ids:
      - productpage
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.productpage
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=productpage"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/productpage:{{ .Tag }}-arm64"

changelog:
  use: github
```

- [ ] **Step 2: Create `services/details/.goreleaser.yaml`**

Same structure as productpage, replacing all occurrences of `productpage` with `details`:
- `project_name: details`
- `id: details` / `binary: details` / `main: ./services/details/cmd/`
- `builds: [details]` in archives
- Image templates: `.../details:{{ .Tag }}-amd64` etc.
- `dockerfile: build/Dockerfile.details`
- Label title: `details`

```yaml
version: 2
project_name: details

builds:
  - id: details
    binary: details
    main: ./services/details/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: details
    builds:
      - details
    name_template: "details_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: details-amd64
    ids:
      - details
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/details:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.details
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=details"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: details-arm64
    ids:
      - details
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/details:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.details
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=details"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/details:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/details:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/details:{{ .Tag }}-arm64"

changelog:
  use: github
```

- [ ] **Step 3: Create `services/reviews/.goreleaser.yaml`**

Same structure, replacing with `reviews`:

```yaml
version: 2
project_name: reviews

builds:
  - id: reviews
    binary: reviews
    main: ./services/reviews/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: reviews
    builds:
      - reviews
    name_template: "reviews_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: reviews-amd64
    ids:
      - reviews
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.reviews
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=reviews"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: reviews-arm64
    ids:
      - reviews
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.reviews
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=reviews"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/reviews:{{ .Tag }}-arm64"

changelog:
  use: github
```

- [ ] **Step 4: Create `services/ratings/.goreleaser.yaml`**

Same structure, replacing with `ratings`:

```yaml
version: 2
project_name: ratings

builds:
  - id: ratings
    binary: ratings
    main: ./services/ratings/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: ratings
    builds:
      - ratings
    name_template: "ratings_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: ratings-amd64
    ids:
      - ratings
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.ratings
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=ratings"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: ratings-arm64
    ids:
      - ratings
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.ratings
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=ratings"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/ratings:{{ .Tag }}-arm64"

changelog:
  use: github
```

- [ ] **Step 5: Create `services/notification/.goreleaser.yaml`**

Same structure, replacing with `notification`:

```yaml
version: 2
project_name: notification

builds:
  - id: notification
    binary: notification
    main: ./services/notification/cmd/
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

archives:
  - id: notification
    builds:
      - notification
    name_template: "notification_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

dockers:
  - id: notification-amd64
    ids:
      - notification
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:{{ .Tag }}-amd64"
    dockerfile: build/Dockerfile.notification
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=notification"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

  - id: notification-arm64
    ids:
      - notification
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:{{ .Tag }}-arm64"
    dockerfile: build/Dockerfile.notification
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{ .Date }}"
      - "--label=org.opencontainers.image.title=notification"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.version={{ .Version }}"

docker_manifests:
  - name_template: "ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:{{ .Tag }}"
    image_templates:
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:{{ .Tag }}-amd64"
      - "ghcr.io/kaio6fellipe/event-driven-bookinfo/notification:{{ .Tag }}-arm64"

changelog:
  use: github
```

- [ ] **Step 6: Delete root `.goreleaser.yaml`**

```bash
git rm .goreleaser.yaml
```

- [ ] **Step 7: Verify configs with dry-run (one service)**

```bash
GORELEASER_CURRENT_TAG=v0.1.0 goreleaser check -f services/details/.goreleaser.yaml
```

Expected: `config is valid`

- [ ] **Step 8: Commit**

```bash
git add services/productpage/.goreleaser.yaml services/details/.goreleaser.yaml \
  services/reviews/.goreleaser.yaml services/ratings/.goreleaser.yaml \
  services/notification/.goreleaser.yaml
git commit -m "feat(release): add per-service goreleaser configs and remove root config

Split the monolithic .goreleaser.yaml into per-service configs under
services/<name>/.goreleaser.yaml. Each config builds, archives, and
produces multi-arch Docker images for a single service independently.
Uses GORELEASER_CURRENT_TAG env var for version resolution (GoReleaser OSS)."
```

---

### Task 3: Rewrite Release Workflow

**Files:**
- Rewrite: `.github/workflows/release.yml`

- [ ] **Step 1: Rewrite `.github/workflows/release.yml`**

```yaml
name: Release

on:
  workflow_dispatch:
    inputs:
      service:
        description: "Service to release"
        required: true
        type: choice
        options:
          - productpage
          - details
          - reviews
          - ratings
          - notification
      tag:
        description: "Full tag (e.g., details-v0.1.0)"
        required: true
        type: string

permissions:
  contents: write
  packages: write

jobs:
  release:
    name: Release ${{ inputs.service }}
    runs-on: ubuntu-latest
    steps:
      - name: Validate tag format
        run: |
          TAG="${{ inputs.tag }}"
          SERVICE="${{ inputs.service }}"
          if [[ ! "$TAG" =~ ^${SERVICE}-v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "::error::Tag '$TAG' does not match expected format '${SERVICE}-vX.Y.Z'"
            exit 1
          fi

      - name: Extract version from tag
        id: version
        run: |
          TAG="${{ inputs.tag }}"
          VERSION="${TAG#${{ inputs.service }}-}"
          echo "version=$VERSION" >> "$GITHUB_OUTPUT"
          echo "Releasing ${{ inputs.service }} at $VERSION"

      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.26.2"

      - name: Set up QEMU (multi-platform Docker builds)
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean -f services/${{ inputs.service }}/.goreleaser.yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GORELEASER_CURRENT_TAG: ${{ steps.version.outputs.version }}
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "valid YAML"
```

Expected: `valid YAML`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "feat(release): rewrite release workflow for per-service dispatch

Replace tag-push trigger with workflow_dispatch. Accepts service name
and tag as inputs, strips the service prefix for GORELEASER_CURRENT_TAG,
and runs the per-service .goreleaser.yaml config. Can be triggered
manually from GitHub UI or by the auto-tag workflow."
```

---

### Task 4: Create Auto-Tag Workflow

**Files:**
- Create: `.github/workflows/auto-tag.yml`

- [ ] **Step 1: Create `.github/workflows/auto-tag.yml`**

```yaml
name: Auto Tag

on:
  pull_request:
    types: [closed]
    branches: [main]

permissions:
  contents: write
  actions: write

jobs:
  auto-tag:
    name: Auto Tag Services
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Detect changed services
        id: changes
        run: |
          BASE_SHA="${{ github.event.pull_request.base.sha }}"
          HEAD_SHA="${{ github.event.pull_request.head.sha }}"
          CHANGED_FILES=$(git diff --name-only "$BASE_SHA" "$HEAD_SHA")

          SERVICES=()
          ALL_SERVICES=(productpage details reviews ratings notification)

          # Check if shared packages changed — triggers all services
          if echo "$CHANGED_FILES" | grep -qE '^(pkg/|go\.mod|go\.sum)'; then
            SERVICES=("${ALL_SERVICES[@]}")
          else
            # Check each service directory
            for svc in "${ALL_SERVICES[@]}"; do
              if echo "$CHANGED_FILES" | grep -q "^services/${svc}/"; then
                SERVICES+=("$svc")
              fi
            done
          fi

          if [ ${#SERVICES[@]} -eq 0 ]; then
            echo "No service changes detected, skipping"
            echo "services=" >> "$GITHUB_OUTPUT"
          else
            # Output as JSON array for matrix consumption
            JSON=$(printf '%s\n' "${SERVICES[@]}" | jq -R . | jq -sc .)
            echo "services=$JSON" >> "$GITHUB_OUTPUT"
            echo "Changed services: $JSON"
          fi

      - name: Determine bump type
        id: bump
        if: steps.changes.outputs.services != ''
        run: |
          BUMP="patch"

          # Priority 1: PR labels
          LABELS='${{ toJson(github.event.pull_request.labels.*.name) }}'
          if echo "$LABELS" | jq -e 'index("major")' > /dev/null 2>&1; then
            BUMP="major"
          elif echo "$LABELS" | jq -e 'index("minor")' > /dev/null 2>&1; then
            BUMP="minor"
          else
            # Priority 2: Conventional commits
            BASE_SHA="${{ github.event.pull_request.base.sha }}"
            HEAD_SHA="${{ github.event.pull_request.head.sha }}"
            COMMITS=$(git log --format="%s%n%b" "$BASE_SHA".."$HEAD_SHA")

            if echo "$COMMITS" | grep -qE '(BREAKING CHANGE|^[a-z]+(\(.+\))?!:)'; then
              BUMP="major"
            elif echo "$COMMITS" | grep -qE '^feat(\(.+\))?:'; then
              BUMP="minor"
            fi
          fi

          echo "bump=$BUMP" >> "$GITHUB_OUTPUT"
          echo "Bump type: $BUMP"

      - name: Tag and dispatch releases
        if: steps.changes.outputs.services != ''
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          BUMP="${{ steps.bump.outputs.bump }}"
          SERVICES='${{ steps.changes.outputs.services }}'

          for SERVICE in $(echo "$SERVICES" | jq -r '.[]'); do
            # Get latest tag for this service
            LATEST_TAG=$(git tag -l "${SERVICE}-v*" --sort=-v:refversion | head -1)

            if [ -z "$LATEST_TAG" ]; then
              CURRENT="0.0.0"
            else
              CURRENT="${LATEST_TAG#${SERVICE}-v}"
            fi

            # Parse version components
            IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT"

            # Increment based on bump type
            case "$BUMP" in
              major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
              minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
              patch) PATCH=$((PATCH + 1)) ;;
            esac

            NEXT_VERSION="${MAJOR}.${MINOR}.${PATCH}"
            NEXT_TAG="${SERVICE}-v${NEXT_VERSION}"

            echo "Tagging ${SERVICE}: ${CURRENT} -> ${NEXT_VERSION} (${BUMP})"

            # Create and push tag
            git tag "$NEXT_TAG"
            git push origin "$NEXT_TAG"

            # Dispatch release workflow
            gh workflow run release.yml \
              -f service="$SERVICE" \
              -f tag="$NEXT_TAG"

            echo "Dispatched release for ${NEXT_TAG}"
          done
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/auto-tag.yml'))" && echo "valid YAML"
```

Expected: `valid YAML`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/auto-tag.yml
git commit -m "feat(release): add auto-tag workflow for PR merge

Triggers on PR merge to main. Detects which services changed (including
pkg/ changes triggering all services), determines bump type from PR
labels or conventional commits, creates prefixed tags, and dispatches
the release workflow for each affected service."
```

---

### Task 5: Create Release Rules for Claude Code

**Files:**
- Create: `.claude/rules/release.md`

- [ ] **Step 1: Create `.claude/rules/release.md`**

```markdown
# Release Process

## Tag Convention
- Tags follow `<service>-v<major>.<minor>.<patch>` format (e.g., `details-v0.1.0`)
- Services: productpage, details, reviews, ratings, notification
- Tags are created automatically by the auto-tag workflow on PR merge

## Version Bumps
- **major**: PR label `major` OR commit with `BREAKING CHANGE` / `!:` suffix
- **minor**: PR label `minor` OR commit starting with `feat(...)`
- **patch**: default — `fix(...)`, `chore(...)`, or any other conventional commit

## Conventional Commit Impact on Releases
- Scope your commits to the service: `feat(reviews):`, `fix(ratings):`, `chore(productpage):`
- Commits affect version bumps — `feat` = minor, `fix` = patch, `!:` = major
- `pkg/` changes trigger releases for ALL services

## GoReleaser
- Per-service configs live at `services/<name>/.goreleaser.yaml`
- Uses GoReleaser OSS with `GORELEASER_CURRENT_TAG` env var for version resolution
- No root `.goreleaser.yaml` — each service is released independently

## Workflows
- `auto-tag.yml`: triggered on PR merge to main, detects changes, creates tags, dispatches release
- `release.yml`: triggered by `workflow_dispatch`, builds and releases a single service
- Manual release: `gh workflow run release.yml -f service=<name> -f tag=<name>-v<X.Y.Z>`
```

- [ ] **Step 2: Commit**

```bash
git add .claude/rules/release.md
git commit -m "docs(claude): add release process rules

Defines tag convention, version bump logic, conventional commit impact,
GoReleaser config location, and workflow descriptions."
```

---

### Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add release process section after "Git Workflow" section**

Insert after line 160 (after the closing ``` of the Git Workflow code block):

```markdown

## Release Process

Each service is released independently with its own version tag.

**Tag format**: `<service>-v<major>.<minor>.<patch>` (e.g., `details-v0.1.0`, `reviews-v1.2.3`)

**Auto-release on PR merge to `main`:**
1. `auto-tag.yml` detects which services changed (`services/<name>/` paths + `pkg/`/`go.mod`/`go.sum` triggers all)
2. Determines bump type: PR labels (`major`/`minor`) → conventional commits → default `patch`
3. Creates prefixed tag and dispatches `release.yml` via `workflow_dispatch`

**Manual release:** `gh workflow run release.yml -f service=<name> -f tag=<name>-v<X.Y.Z>`

**GoReleaser configs:** `services/<name>/.goreleaser.yaml` (per-service, GoReleaser OSS with `GORELEASER_CURRENT_TAG`)

**Version source of truth:** git tags (`git tag -l "<service>-v*"`)
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): add release process section to CLAUDE.md

Documents per-service release flow, tag convention, auto-tagging,
manual release, and GoReleaser config locations."
```

---

### Task 7: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the project structure tree**

In the project structure section (~line 557), replace:

```
├── .goreleaser.yaml            # GoReleaser v2 multi-binary release config
```

with:

```
├── .github/workflows/
│   ├── release.yml             # Per-service release via workflow_dispatch
│   └── auto-tag.yml            # Auto-tag on PR merge, dispatches release
```

- [ ] **Step 2: Add "Releasing" section before "Contributing"**

Insert before the `## Contributing` section (~line 564):

```markdown
## Releasing

Each service is versioned and released independently. Releases are fully automated on PR merge to `main`.

### How It Works

1. **PR merged to `main`** → `auto-tag.yml` runs
2. **Detects changed services** — file paths under `services/<name>/`; changes to `pkg/`, `go.mod`, or `go.sum` trigger all 5 services
3. **Determines version bump** — PR labels (`major`/`minor`) take priority, then conventional commit prefixes (`feat` → minor, `fix` → patch, `BREAKING CHANGE` → major), default is `patch`
4. **Creates tag** — e.g., `details-v0.2.0`
5. **Dispatches release** — `release.yml` builds binaries, Docker images (multi-arch), and creates a GitHub release

### Tag Format

```
<service>-v<major>.<minor>.<patch>
```

Examples: `details-v0.1.0`, `reviews-v1.2.3`, `productpage-v0.3.0`

### Manual Release

```bash
# Trigger a release for a specific service
gh workflow run release.yml -f service=details -f tag=details-v0.1.0
```

### Version Bump Labels

| Label | Effect |
|-------|--------|
| `major` | Major version bump (breaking change) |
| `minor` | Minor version bump (new feature) |
| *(none)* | Determined by conventional commits, default patch |

### GoReleaser Configs

Per-service configs at `services/<name>/.goreleaser.yaml`. Uses GoReleaser OSS with `GORELEASER_CURRENT_TAG` environment variable for version resolution.

---

```

- [ ] **Step 3: Update the GHCR images section**

At ~line 238, update the text from:

```
Images are tagged `event-driven-bookinfo/<service>:latest` locally. Released images are pushed to GitHub Container Registry:
```

to:

```
Images are tagged `event-driven-bookinfo/<service>:latest` locally. Released images are pushed to GitHub Container Registry with the service version tag:
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add releasing section and update project structure in README

Documents per-service release flow, tag format, manual release command,
version bump labels, and GoReleaser config locations. Updates project
structure tree to reflect per-service configs."
```

---

### Task 8: Validate — First Release (details-v0.1.0)

**Files:** None (GitHub API + git tags)

- [ ] **Step 1: Push the branch and ensure all changes are on `main`**

All previous commits must be merged to `main` before testing. The release workflow reads configs from the default branch.

- [ ] **Step 2: Create and push the initial tag for details**

```bash
git tag details-v0.1.0
git push origin details-v0.1.0
```

- [ ] **Step 3: Trigger the release workflow**

```bash
gh workflow run release.yml -f service=details -f tag=details-v0.1.0
```

- [ ] **Step 4: Watch the workflow run**

```bash
gh run list --workflow=release.yml --limit=1
gh run watch
```

Expected: workflow completes successfully.

- [ ] **Step 5: Verify the GitHub release**

```bash
gh release view details-v0.1.0
```

Expected: release exists with title containing `details` and version `v0.1.0`.

- [ ] **Step 6: Verify release assets**

```bash
gh release view details-v0.1.0 --json assets --jq '.assets[].name'
```

Expected output (4 archives):
```
details_0.1.0_darwin_amd64.tar.gz
details_0.1.0_darwin_arm64.tar.gz
details_0.1.0_linux_amd64.tar.gz
details_0.1.0_linux_arm64.tar.gz
```

- [ ] **Step 7: Verify Docker images on GHCR**

```bash
gh api user/packages/container/event-driven-bookinfo%2Fdetails/versions \
  --jq '.[0].metadata.container.tags'
```

Expected: tags include `v0.1.0-amd64`, `v0.1.0-arm64`, `v0.1.0`.

- [ ] **Step 8: If validation fails, rollback**

```bash
gh release delete details-v0.1.0 --yes
git push --delete origin details-v0.1.0
git tag -d details-v0.1.0
```

Then debug and fix the issue before retrying.

---

### Task 9: Validate — Release Remaining Services (v0.1.0)

**Files:** None (GitHub API + git tags)

- [ ] **Step 1: Tag and release remaining 4 services**

```bash
for svc in productpage reviews ratings notification; do
  git tag "${svc}-v0.1.0"
  git push origin "${svc}-v0.1.0"
  gh workflow run release.yml -f service="${svc}" -f tag="${svc}-v0.1.0"
  echo "Dispatched release for ${svc}-v0.1.0"
  sleep 5
done
```

- [ ] **Step 2: Wait for all workflows to complete**

```bash
gh run list --workflow=release.yml --limit=5
```

Expected: all 4 runs show `completed` status.

- [ ] **Step 3: Verify all 5 tags exist**

```bash
git tag -l "*-v0.1.0"
```

Expected:
```
details-v0.1.0
notification-v0.1.0
productpage-v0.1.0
ratings-v0.1.0
reviews-v0.1.0
```

- [ ] **Step 4: Verify all 5 GitHub releases exist**

```bash
gh release list --limit=5
```

Expected: 5 releases listed.

- [ ] **Step 5: Verify all Docker images**

```bash
for svc in productpage details reviews ratings notification; do
  echo "=== ${svc} ==="
  gh api user/packages/container/event-driven-bookinfo%2F${svc}/versions \
    --jq '.[0].metadata.container.tags' 2>/dev/null || echo "NOT FOUND"
done
```

Expected: each service shows `v0.1.0-amd64`, `v0.1.0-arm64`, `v0.1.0` tags.

- [ ] **Step 6: Rollback any failed releases**

```bash
# For any failed service:
# gh release delete <service>-v0.1.0 --yes
# git push --delete origin <service>-v0.1.0
# git tag -d <service>-v0.1.0
```

---

### Task 10: Validate — Auto-Tag Workflow (End-to-End)

**Files:** None (test PR)

- [ ] **Step 1: Create a test branch with a trivial change to one service**

```bash
git checkout -b test/auto-tag-validation
echo "// auto-tag test" >> services/details/cmd/main.go
git add services/details/cmd/main.go
git commit -m "fix(details): trivial change for auto-tag validation"
git push -u origin test/auto-tag-validation
```

- [ ] **Step 2: Create and merge a test PR**

```bash
gh pr create --title "fix(details): auto-tag validation test" \
  --body "Trivial change to validate auto-tag workflow. Will be reverted." \
  --base main
```

Merge the PR:
```bash
gh pr merge --squash --delete-branch
```

- [ ] **Step 3: Verify auto-tag workflow ran**

```bash
gh run list --workflow=auto-tag.yml --limit=1
```

Expected: one run with `completed` status.

- [ ] **Step 4: Verify new tag was created**

```bash
git fetch --tags
git tag -l "details-v*"
```

Expected: `details-v0.1.0` and `details-v0.1.1` (patch bump from `fix(details):`)

- [ ] **Step 5: Verify release workflow was dispatched**

```bash
gh run list --workflow=release.yml --limit=1
```

Expected: a new run for `details-v0.1.1`.

- [ ] **Step 6: Revert the test change**

```bash
git checkout main
git pull
git revert HEAD --no-edit
git push
```

- [ ] **Step 7: Final verification — all releases intact**

```bash
gh release list --limit=10
git tag -l "*-v*" --sort=-v:refversion
```
