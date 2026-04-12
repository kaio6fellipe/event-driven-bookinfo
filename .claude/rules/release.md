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
