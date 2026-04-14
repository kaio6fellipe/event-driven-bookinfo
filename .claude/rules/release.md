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

## Chart Release Process

The `bookinfo-service` Helm chart has its own independent version lifecycle.

### Chart Versioning
- Chart version (`version` in `Chart.yaml`): bumped when templates or helpers change
- App version (`appVersion`): informational only — each service sets `image.tag` at install time
- Chart version is independent of service tags (`{service}-vX.Y.Z`)

### Workflows
- `helm-lint-test.yml`: runs on PRs touching `charts/**` — ct lint + ct install
- `helm-release.yml`: runs on main push touching `charts/**` — chart-releaser publishes to GitHub Pages

### Using the Chart
```bash
helm repo add bookinfo https://kaio6fellipe.github.io/event-driven-bookinfo
helm repo update
helm install ratings bookinfo/bookinfo-service -f deploy/ratings/values-local.yaml -n bookinfo
```
