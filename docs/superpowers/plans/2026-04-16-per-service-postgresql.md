# Per-Service PostgreSQL Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the shared PostgreSQL StatefulSet with per-service Bitnami PostgreSQL subchart instances deployed alongside each service via the bookinfo-service Helm chart.

**Architecture:** Add `bitnami/postgresql` as a conditional Helm subchart dependency (`postgresql.enabled`). When enabled, the chart auto-wires `DATABASE_URL`, `STORAGE_BACKEND`, and `RUN_MIGRATIONS` into the service ConfigMap. Each postgres-backed service gets its own PostgreSQL StatefulSet with right-sized resources (25m/64Mi requests, 100m/128Mi limits, 256Mi PVC). The shared `deploy/postgres/` is removed entirely.

**Tech Stack:** Helm 3, Bitnami PostgreSQL chart ~16, k3d, k6, Grafana

**Spec:** `docs/superpowers/specs/2026-04-16-per-service-postgresql-design.md`

---

### Task 1: Add Bitnami PostgreSQL Subchart Dependency

**Files:**
- Modify: `charts/bookinfo-service/Chart.yaml`

- [ ] **Step 1: Add the dependency block to Chart.yaml**

Append the `dependencies` block after the existing content:

```yaml
dependencies:
  - name: postgresql
    version: "~16"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: postgresql.enabled
```

Full file after edit:

```yaml
apiVersion: v2
name: bookinfo-service
description: Reusable Helm chart for event-driven-bookinfo microservices
type: application
version: 0.1.0
appVersion: "0.0.0"
maintainers:
  - name: kaio6fellipe
    url: https://github.com/kaio6fellipe
dependencies:
  - name: postgresql
    version: "~16"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: postgresql.enabled
```

- [ ] **Step 2: Run helm dependency update to fetch the subchart**

Run:

```bash
helm dependency update charts/bookinfo-service
```

Expected: Downloads the Bitnami PostgreSQL chart into `charts/bookinfo-service/charts/postgresql-16.x.x.tgz` and generates `Chart.lock`.

- [ ] **Step 3: Verify the dependency was added**

Run:

```bash
helm dependency list charts/bookinfo-service
```

Expected output includes a line like:

```
NAME        VERSION  REPOSITORY                                  STATUS
postgresql  ~16      oci://registry-1.docker.io/bitnamicharts     ok
```

- [ ] **Step 4: Add the downloaded chart archive to .gitignore or check it in**

The Bitnami chart tgz is ~60KB. Add to `.gitignore` so it's fetched fresh:

Check if `charts/bookinfo-service/charts/` is already gitignored:

```bash
grep -r 'charts/bookinfo-service/charts' .gitignore .helmignore 2>/dev/null
```

If not ignored, add to `charts/bookinfo-service/.gitignore`:

```
charts/
```

Keep `Chart.lock` checked in so `helm dependency build` can reproduce exact versions.

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/Chart.yaml charts/bookinfo-service/Chart.lock charts/bookinfo-service/.gitignore
git commit -m "feat(chart): add Bitnami PostgreSQL as conditional subchart dependency

Add postgresql ~16 from oci://registry-1.docker.io/bitnamicharts as an
optional dependency gated by postgresql.enabled (default false)."
```

---

### Task 2: Add PostgreSQL Default Values

**Files:**
- Modify: `charts/bookinfo-service/values.yaml`

- [ ] **Step 1: Append the postgresql section to values.yaml**

Add at the end of the file, after the `routes` section:

```yaml

# -- PostgreSQL (Bitnami subchart)
postgresql:
  enabled: false
  auth:
    username: "bookinfo"
    password: "bookinfo"
    database: ""
  primary:
    resources:
      requests:
        cpu: 25m
        memory: 64Mi
      limits:
        cpu: 100m
        memory: 128Mi
    persistence:
      enabled: true
      size: 256Mi
```

- [ ] **Step 2: Verify default values render correctly with postgresql disabled**

Run:

```bash
helm template test charts/bookinfo-service --set image.repository=test --set image.tag=test 2>&1 | grep -c "postgresql"
```

Expected: `0` — no PostgreSQL resources rendered when `postgresql.enabled` is false.

- [ ] **Step 3: Verify PostgreSQL resources render when enabled**

Run:

```bash
helm template test charts/bookinfo-service \
  --set image.repository=test --set image.tag=test \
  --set postgresql.enabled=true \
  --set postgresql.auth.database=testdb 2>&1 | grep "kind: StatefulSet"
```

Expected: Two StatefulSet entries — one for the PostgreSQL subchart, plus potentially one from the service itself if rendered. At minimum, one StatefulSet from the Bitnami subchart should appear.

- [ ] **Step 4: Commit**

```bash
git add charts/bookinfo-service/values.yaml
git commit -m "feat(chart): add PostgreSQL default values for Bitnami subchart

Disabled by default. When enabled, deploys a per-service PostgreSQL
instance with right-sized resources (25m/64Mi req, 100m/128Mi lim,
256Mi PVC)."
```

---

### Task 3: Auto-Wire DATABASE_URL in ConfigMap

**Files:**
- Modify: `charts/bookinfo-service/templates/configmap.yaml`

- [ ] **Step 1: Add the postgresql auto-wiring block**

Edit `charts/bookinfo-service/templates/configmap.yaml` to inject `STORAGE_BACKEND`, `RUN_MIGRATIONS`, and `DATABASE_URL` when postgresql is enabled. Place the block **before** the `config` range loop so explicit config values can override:

```yaml
{{/* charts/bookinfo-service/templates/configmap.yaml */}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "bookinfo-service.fullname" . }}
  labels:
    {{- include "bookinfo-service.labels" . | nindent 4 }}
data:
  SERVICE_NAME: {{ include "bookinfo-service.serviceName" . | quote }}
  HTTP_PORT: {{ .Values.ports.http | quote }}
  ADMIN_PORT: {{ .Values.ports.admin | quote }}
  {{- if .Values.postgresql.enabled }}
  STORAGE_BACKEND: "postgres"
  RUN_MIGRATIONS: "true"
  DATABASE_URL: {{ printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable" .Values.postgresql.auth.username .Values.postgresql.auth.password .Release.Name .Values.postgresql.auth.database | quote }}
  {{- end }}
  {{- range $key, $value := .Values.config }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
  {{- with .Values.observability.otelEndpoint }}
  OTEL_EXPORTER_OTLP_ENDPOINT: {{ . | quote }}
  {{- end }}
  {{- with .Values.observability.pyroscopeAddress }}
  PYROSCOPE_SERVER_ADDRESS: {{ . | quote }}
  {{- end }}
```

- [ ] **Step 2: Verify auto-wired values render correctly**

Run:

```bash
helm template ratings charts/bookinfo-service \
  --set image.repository=test --set image.tag=test \
  --set postgresql.enabled=true \
  --set postgresql.auth.database=bookinfo_ratings 2>&1 | grep -A5 "kind: ConfigMap"
```

Expected: The ConfigMap `data` section contains:
- `STORAGE_BACKEND: "postgres"`
- `RUN_MIGRATIONS: "true"`
- `DATABASE_URL: "postgres://bookinfo:bookinfo@ratings-postgresql:5432/bookinfo_ratings?sslmode=disable"`

- [ ] **Step 3: Verify explicit config overrides auto-wired values**

Run:

```bash
helm template ratings charts/bookinfo-service \
  --set image.repository=test --set image.tag=test \
  --set postgresql.enabled=true \
  --set postgresql.auth.database=bookinfo_ratings \
  --set config.STORAGE_BACKEND=custom 2>&1 | grep "STORAGE_BACKEND"
```

Expected: Two `STORAGE_BACKEND` entries in the raw template — the auto-wired `"postgres"` and the config override `"custom"`. In a real ConfigMap, the last key wins, so `"custom"` takes effect. This is correct behavior.

- [ ] **Step 4: Verify no auto-wiring when postgresql disabled**

Run:

```bash
helm template test charts/bookinfo-service \
  --set image.repository=test --set image.tag=test 2>&1 | grep "DATABASE_URL"
```

Expected: No output — `DATABASE_URL` is not present when `postgresql.enabled` is false.

- [ ] **Step 5: Commit**

```bash
git add charts/bookinfo-service/templates/configmap.yaml
git commit -m "feat(chart): auto-wire DATABASE_URL when PostgreSQL subchart is enabled

Injects STORAGE_BACKEND, RUN_MIGRATIONS, and DATABASE_URL into the
ConfigMap when postgresql.enabled is true. Placed before the config
range loop so explicit config values take precedence."
```

---

### Task 4: Update Values Schema

**Files:**
- Modify: `charts/bookinfo-service/values.schema.json`

- [ ] **Step 1: Add the postgresql property to the schema**

Add the `postgresql` property inside the top-level `properties` object, after the `routes` property:

```json
    "postgresql": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean" },
        "auth": {
          "type": "object",
          "properties": {
            "username": { "type": "string" },
            "password": { "type": "string" },
            "database": { "type": "string" }
          }
        },
        "primary": {
          "type": "object",
          "properties": {
            "resources": { "type": "object" },
            "persistence": {
              "type": "object",
              "properties": {
                "enabled": { "type": "boolean" },
                "size": { "type": "string" }
              }
            }
          }
        }
      }
    }
```

- [ ] **Step 2: Validate the JSON is well-formed**

Run:

```bash
python3 -m json.tool charts/bookinfo-service/values.schema.json > /dev/null
```

Expected: No output (valid JSON).

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/values.schema.json
git commit -m "feat(chart): add postgresql to values schema

Validates the postgresql.enabled, auth, and primary.resources/persistence
fields. Additional Bitnami values pass through without schema enforcement."
```

---

### Task 5: Add CI Test Values for PostgreSQL Path

**Files:**
- Create: `charts/bookinfo-service/ci/values-details-postgres.yaml`

- [ ] **Step 1: Create the CI test values file**

```yaml
# charts/bookinfo-service/ci/values-details-postgres.yaml
# ct lint test: PostgreSQL subchart enabled with auto-wired DATABASE_URL
serviceName: details
fullnameOverride: details
image:
  repository: event-driven-bookinfo/details
  tag: latest

postgresql:
  enabled: true
  auth:
    database: "bookinfo_details"
```

- [ ] **Step 2: Verify ct lint passes with the new values file**

Run:

```bash
helm lint charts/bookinfo-service -f charts/bookinfo-service/ci/values-details-postgres.yaml
```

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 3: Commit**

```bash
git add charts/bookinfo-service/ci/values-details-postgres.yaml
git commit -m "test(chart): add CI values file for PostgreSQL subchart path

Exercises postgresql.enabled=true with auto-wired DATABASE_URL to ensure
ct lint validates the postgres-enabled template rendering."
```

---

### Task 6: Simplify details values-local.yaml

**Files:**
- Modify: `deploy/details/values-local.yaml`

- [ ] **Step 1: Replace manual postgres config with subchart toggle**

Remove `STORAGE_BACKEND`, `DATABASE_URL`, and `RUN_MIGRATIONS` from `config:` and add `postgresql:` section.

Before (lines 8-12):

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_details?sslmode=disable"
  RUN_MIGRATIONS: "true"
```

After:

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_details"

config:
  LOG_LEVEL: "debug"
```

- [ ] **Step 2: Verify the rendered ConfigMap has correct DATABASE_URL**

Run:

```bash
helm template details charts/bookinfo-service -f deploy/details/values-local.yaml --namespace bookinfo 2>&1 | grep "DATABASE_URL"
```

Expected: `DATABASE_URL: "postgres://bookinfo:bookinfo@details-postgresql:5432/bookinfo_details?sslmode=disable"`

- [ ] **Step 3: Commit**

```bash
git add deploy/details/values-local.yaml
git commit -m "refactor(details): use PostgreSQL subchart instead of shared instance

Replace manual DATABASE_URL with postgresql.enabled=true. The chart
auto-wires STORAGE_BACKEND, RUN_MIGRATIONS, and DATABASE_URL from
the subchart configuration."
```

---

### Task 7: Simplify ratings values-local.yaml

**Files:**
- Modify: `deploy/ratings/values-local.yaml`

- [ ] **Step 1: Replace manual postgres config with subchart toggle**

Remove `STORAGE_BACKEND`, `DATABASE_URL`, and `RUN_MIGRATIONS` from `config:` and add `postgresql:` section.

Before (lines 8-12):

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_ratings?sslmode=disable"
  RUN_MIGRATIONS: "true"
```

After:

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_ratings"

config:
  LOG_LEVEL: "debug"
```

- [ ] **Step 2: Verify the rendered ConfigMap**

Run:

```bash
helm template ratings charts/bookinfo-service -f deploy/ratings/values-local.yaml --namespace bookinfo 2>&1 | grep "DATABASE_URL"
```

Expected: `DATABASE_URL: "postgres://bookinfo:bookinfo@ratings-postgresql:5432/bookinfo_ratings?sslmode=disable"`

- [ ] **Step 3: Commit**

```bash
git add deploy/ratings/values-local.yaml
git commit -m "refactor(ratings): use PostgreSQL subchart instead of shared instance"
```

---

### Task 8: Simplify reviews values-local.yaml

**Files:**
- Modify: `deploy/reviews/values-local.yaml`

- [ ] **Step 1: Replace manual postgres config with subchart toggle**

Remove `STORAGE_BACKEND`, `DATABASE_URL`, and `RUN_MIGRATIONS` from `config:` and add `postgresql:` section. Keep `RATINGS_SERVICE_URL` in `config:`.

Before (lines 8-13):

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_reviews?sslmode=disable"
  RUN_MIGRATIONS: "true"
  RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"
```

After:

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_reviews"

config:
  LOG_LEVEL: "debug"
  RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"
```

- [ ] **Step 2: Verify the rendered ConfigMap**

Run:

```bash
helm template reviews charts/bookinfo-service -f deploy/reviews/values-local.yaml --namespace bookinfo 2>&1 | grep -E "DATABASE_URL|RATINGS_SERVICE_URL"
```

Expected:
- `DATABASE_URL: "postgres://bookinfo:bookinfo@reviews-postgresql:5432/bookinfo_reviews?sslmode=disable"`
- `RATINGS_SERVICE_URL: "http://ratings.bookinfo.svc.cluster.local"`

- [ ] **Step 3: Commit**

```bash
git add deploy/reviews/values-local.yaml
git commit -m "refactor(reviews): use PostgreSQL subchart instead of shared instance"
```

---

### Task 9: Simplify notification values-local.yaml

**Files:**
- Modify: `deploy/notification/values-local.yaml`

- [ ] **Step 1: Replace manual postgres config with subchart toggle**

Before (lines 8-12):

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_notification?sslmode=disable"
  RUN_MIGRATIONS: "true"
```

After:

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_notification"

config:
  LOG_LEVEL: "debug"
```

- [ ] **Step 2: Verify the rendered ConfigMap**

Run:

```bash
helm template notification charts/bookinfo-service -f deploy/notification/values-local.yaml --namespace bookinfo 2>&1 | grep "DATABASE_URL"
```

Expected: `DATABASE_URL: "postgres://bookinfo:bookinfo@notification-postgresql:5432/bookinfo_notification?sslmode=disable"`

- [ ] **Step 3: Commit**

```bash
git add deploy/notification/values-local.yaml
git commit -m "refactor(notification): use PostgreSQL subchart instead of shared instance"
```

---

### Task 10: Simplify dlqueue values-local.yaml

**Files:**
- Modify: `deploy/dlqueue/values-local.yaml`

- [ ] **Step 1: Replace manual postgres config with subchart toggle**

Before (lines 8-12):

```yaml
config:
  LOG_LEVEL: "debug"
  STORAGE_BACKEND: "postgres"
  DATABASE_URL: "postgres://bookinfo:bookinfo@postgres.bookinfo.svc.cluster.local:5432/bookinfo_dlqueue?sslmode=disable"
  RUN_MIGRATIONS: "true"
```

After:

```yaml
postgresql:
  enabled: true
  auth:
    database: "bookinfo_dlqueue"

config:
  LOG_LEVEL: "debug"
```

- [ ] **Step 2: Verify the rendered ConfigMap**

Run:

```bash
helm template dlqueue charts/bookinfo-service -f deploy/dlqueue/values-local.yaml --namespace bookinfo 2>&1 | grep "DATABASE_URL"
```

Expected: `DATABASE_URL: "postgres://bookinfo:bookinfo@dlqueue-postgresql:5432/bookinfo_dlqueue?sslmode=disable"`

- [ ] **Step 3: Commit**

```bash
git add deploy/dlqueue/values-local.yaml
git commit -m "refactor(dlqueue): use PostgreSQL subchart instead of shared instance"
```

---

### Task 11: Remove Shared PostgreSQL Deployment

**Files:**
- Delete: `deploy/postgres/local/statefulset.yaml`
- Delete: `deploy/postgres/local/service.yaml`
- Delete: `deploy/postgres/local/init-configmap.yaml`
- Delete: `deploy/postgres/local/kustomization.yaml`
- Delete: `deploy/postgres/local/` (directory)
- Delete: `deploy/postgres/` (directory, if empty)

- [ ] **Step 1: Delete all files in deploy/postgres/**

Run:

```bash
rm -rf deploy/postgres/
```

- [ ] **Step 2: Verify deletion**

Run:

```bash
ls deploy/postgres/ 2>&1
```

Expected: `ls: deploy/postgres/: No such file or directory`

- [ ] **Step 3: Commit**

```bash
git add -A deploy/postgres/
git commit -m "refactor: remove shared PostgreSQL deployment

Each service now deploys its own PostgreSQL instance via the Bitnami
subchart. The shared StatefulSet, Service, init ConfigMap, and
kustomization are no longer needed."
```

---

### Task 12: Update Makefile — Remove Shared Postgres Deploy Step

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Remove the shared PostgreSQL deploy step from k8s-deploy**

In the `k8s-deploy` target, remove lines 369-373:

```makefile
	@printf "$(BOLD)[3/6] Deploying PostgreSQL...$(NC)\n"
	@$(KUBECTL) apply -k deploy/postgres/local/
	@$(KUBECTL) wait statefulset/postgres -n $(K8S_NS_BOOKINFO) \
		--for=jsonpath='{.status.readyReplicas}'=1 --timeout=120s
	@printf "  $(GREEN)PostgreSQL ready.$(NC)\n"
```

Also update the step numbers. The remaining steps become:

- `[1/5] Building Docker images...` (was [1/6])
- `[2/5] Importing images to k3d...` (was [2/6])
- `[3/5] Deploying Redis...` (was [4/6], note the label already says [5/5] — fix this)
- `[4/5] Deploying services via Helm...` (was [5/5])
- Waiting for deployments remains unnumbered

Note: The current Makefile already has a numbering error (jumps from [2/6] to [4/6] to [5/5]). Fix all step labels to be sequential [1/N] through [N/N].

After edit, the `k8s-deploy` target should read:

```makefile
.PHONY: k8s-deploy
k8s-deploy: ##@Kubernetes Build images, import to k3d, deploy apps + HTTPRoutes
	$(k8s-guard)
	@printf "\n$(BOLD)═══ Application Layer ═══$(NC)\n\n"
	@$(KUBECTL) create namespace $(K8S_NS_BOOKINFO) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@printf "$(BOLD)[1/4] Building Docker images...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Building $$svc...\n"; \
		docker build -f build/Dockerfile.$$svc -t event-driven-bookinfo/$$svc:local . || exit 1; \
	done
	@printf "$(BOLD)[2/4] Importing images to k3d...$(NC)\n"
	@for svc in $(SERVICES); do \
		k3d image import event-driven-bookinfo/$$svc:local -c $(K8S_CLUSTER) || exit 1; \
	done
	@printf "  $(GREEN)Images imported.$(NC)\n"
	@printf "$(BOLD)[3/4] Deploying Redis...$(NC)\n"
	@$(HELM) repo add bitnami https://charts.bitnami.com/bitnami --force-update > /dev/null 2>&1
	@$(HELM) upgrade --install redis bitnami/redis \
		--namespace $(K8S_NS_BOOKINFO) \
		--values deploy/redis/local/redis-values.yaml \
		--wait --timeout 120s
	@printf "  $(GREEN)Redis ready.$(NC)\n"
	@printf "$(BOLD)[4/4] Deploying services via Helm...$(NC)\n"
	@for svc in $(SERVICES); do \
		printf "  Installing $$svc...\n"; \
		$(HELM) upgrade --install $$svc charts/bookinfo-service \
			--namespace $(K8S_NS_BOOKINFO) \
			-f deploy/$$svc/values-local.yaml || exit 1; \
	done
	@printf "\n$(BOLD)Waiting for deployments...$(NC)\n"
	@for dep in productpage details details-write reviews reviews-write ratings ratings-write notification dlqueue dlqueue-write ingestion; do \
		$(KUBECTL) wait deployment/$$dep -n $(K8S_NS_BOOKINFO) \
			--for=condition=Available --timeout=120s || true; \
	done
	@printf "\n$(GREEN)$(BOLD)Application layer complete.$(NC)\n\n"
```

- [ ] **Step 2: Verify Makefile syntax**

Run:

```bash
make help 2>&1 | head -5
```

Expected: Help output renders without errors.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "refactor: remove shared PostgreSQL deploy step from Makefile

PostgreSQL is now deployed per-service via Bitnami subchart. Remove the
kubectl apply -k deploy/postgres/local/ step and renumber remaining
deploy steps."
```

---

### Task 13: Update Makefile — Retarget Seed Commands

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update k8s-seed to target per-service PostgreSQL pods**

The seed command currently runs:

```makefile
$(KUBECTL) exec -n $(K8S_NS_BOOKINFO) statefulset/postgres -- \
    psql -U bookinfo -d bookinfo_$$svc -c "$$(cat $$seed_file)" > /dev/null 2>&1;
```

Change to target each service's Bitnami PostgreSQL pod:

```makefile
$(KUBECTL) exec -n $(K8S_NS_BOOKINFO) statefulset/$$svc-postgresql -- \
    psql -U bookinfo -d bookinfo_$$svc -c "$$(cat $$seed_file)" > /dev/null 2>&1;
```

Full `k8s-seed` target after edit:

```makefile
.PHONY: k8s-seed
k8s-seed: ##@Kubernetes Seed databases in k8s PostgreSQL
	$(k8s-guard)
	@printf "\n$(BOLD)Seeding databases...$(NC)\n\n"
	@for svc in details ratings reviews notification; do \
		seed_file="services/$$svc/seeds/seed.sql"; \
		if [ -f "$$seed_file" ]; then \
			$(KUBECTL) exec -n $(K8S_NS_BOOKINFO) statefulset/$$svc-postgresql -- \
				psql -U bookinfo -d bookinfo_$$svc -c "$$(cat $$seed_file)" > /dev/null 2>&1; \
			printf "  $(GREEN)%-14s$(NC) seeded\n" "$$svc"; \
		fi; \
	done
	@printf "\n"
```

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -m "refactor: retarget k8s-seed to per-service PostgreSQL pods

Each service now has its own PostgreSQL StatefulSet named
{service}-postgresql. Update the exec target accordingly."
```

---

### Task 14: Update Makefile — Fix helm-lint to Build Dependencies First

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add helm dependency build before helm lint**

The `helm-lint` target needs to build dependencies first, otherwise `helm lint` will fail when the Bitnami subchart isn't downloaded. Add a dependency build step:

```makefile
.PHONY: helm-lint
helm-lint: ##@Helm Lint the bookinfo-service chart
	helm dependency build charts/bookinfo-service
	helm lint charts/bookinfo-service
	@for svc in $(SERVICES); do \
		if [ -f deploy/$$svc/values-local.yaml ]; then \
			printf "  Linting with $$svc values...\n"; \
			helm lint charts/bookinfo-service -f deploy/$$svc/values-local.yaml || exit 1; \
		fi; \
	done
	@printf "$(GREEN)All lints passed.$(NC)\n"
```

- [ ] **Step 2: Run helm-lint to verify**

Run:

```bash
make helm-lint
```

Expected: All lints pass for all services.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "fix(helm): add dependency build step before helm lint

The Bitnami PostgreSQL subchart must be downloaded before linting.
Add helm dependency build to the helm-lint target."
```

---

### Task 15: Update CLAUDE.md Documentation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update Deploy Structure section**

In the `## Deploy Structure` section, remove the `postgres/local/` entry and add a note about the subchart:

Find:

```
└── postgres/local/              # StatefulSet, Service, init ConfigMap
```

Replace with:

```
└── redis/local/                 # Helm values: Bitnami Redis
```

Wait — `redis/local/` is already listed. Just remove the `postgres/local/` line entirely.

- [ ] **Step 2: Update Architecture section**

In the `## Architecture` section, find the bullet about storage:

```
- **Storage**: swappable via `STORAGE_BACKEND` env var — `memory` (default, single replica) or `postgres` (horizontally scalable)
```

Add after it:

```
- **Per-service PostgreSQL**: each postgres-backed service deploys its own Bitnami PostgreSQL instance via the Helm chart subchart (`postgresql.enabled: true`). DATABASE_URL is auto-wired.
```

- [ ] **Step 3: Update the Helm Commands section**

Add a note about dependency build:

After `make helm-lint` line, before the `helm upgrade` line:

```bash
helm dependency build charts/bookinfo-service  # Fetch subchart dependencies (run once)
```

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for per-service PostgreSQL migration

Remove shared postgres references, document Bitnami subchart pattern,
add helm dependency build command."
```

---

### Task 16: Validation — Tear Down and Rebuild Cluster

- [ ] **Step 1: Tear down existing cluster**

Run:

```bash
make stop-k8s
```

Expected: `Cluster deleted.` or `Cluster 'bookinfo-local' does not exist.`

- [ ] **Step 2: Build the full cluster from scratch**

Run:

```bash
make run-k8s
```

Expected: All steps complete successfully. Watch for:
- No `deploy/postgres/local/` errors (step removed)
- Each service's Helm install includes a PostgreSQL StatefulSet
- All deployments reach `Available` condition
- Seed commands target `{service}-postgresql` pods

This will take several minutes. Monitor the output for errors.

- [ ] **Step 3: Verify all pods are running**

Run:

```bash
kubectl --context=k3d-bookinfo-local get pods -n bookinfo
```

Expected: Each postgres-backed service has its own `{service}-postgresql-0` pod alongside the service pods. For example:
- `details-postgresql-0` (Running)
- `ratings-postgresql-0` (Running)
- `reviews-postgresql-0` (Running)
- `notification-postgresql-0` (Running)
- `dlqueue-postgresql-0` (Running)
- No `postgres-0` pod (shared instance removed)

- [ ] **Step 4: Verify PostgreSQL resource sizing**

Run:

```bash
kubectl --context=k3d-bookinfo-local get pods -n bookinfo -l app.kubernetes.io/name=postgresql -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].resources}{"\n"}{end}'
```

Expected: Each pod shows `requests: {cpu: 25m, memory: 64Mi}` and `limits: {cpu: 100m, memory: 128Mi}`.

---

### Task 17: Validation — Manual Smoke Tests

- [ ] **Step 1: Test details service (POST + GET)**

```bash
# POST a book through the Gateway (goes through EventSource -> Sensor -> details-write)
curl -s -X POST http://localhost:8080/v1/details \
  -H "Content-Type: application/json" \
  -d '{
    "title": "PostgreSQL Migration Test",
    "author": "Test Author",
    "year": 2026,
    "type": "technical",
    "pages": 100,
    "publisher": "Test Publisher",
    "language": "en",
    "isbn10": "1234567890",
    "isbn13": "1234567890123"
  }'
```

Wait ~10 seconds for event processing, then:

```bash
# GET all details through the Gateway (goes to details-read)
curl -s http://localhost:8080/v1/details | python3 -m json.tool
```

Expected: The posted book appears in the response.

- [ ] **Step 2: Test ratings service (POST + GET)**

```bash
curl -s -X POST http://localhost:8080/v1/ratings \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": "test-product-1",
    "reviewer": "smoke-tester",
    "stars": 5
  }'
```

Wait ~10 seconds, then:

```bash
curl -s "http://localhost:8080/v1/ratings?product_id=test-product-1" | python3 -m json.tool
```

Expected: The posted rating appears.

- [ ] **Step 3: Test reviews service (POST + GET)**

```bash
curl -s -X POST http://localhost:8080/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": "test-product-1",
    "reviewer": "smoke-tester",
    "text": "Great book for testing per-service PostgreSQL!"
  }'
```

Wait ~10 seconds, then:

```bash
curl -s "http://localhost:8080/v1/reviews?product_id=test-product-1" | python3 -m json.tool
```

Expected: The posted review appears.

- [ ] **Step 4: Verify notification service received events**

```bash
curl -s http://localhost:8080/v1/notifications | python3 -m json.tool
```

Expected: Notifications for the book-added, rating-submitted, and review-submitted events.

- [ ] **Step 5: Verify DLQ service is operational**

```bash
curl -s http://localhost:8080/v1/events | python3 -m json.tool
```

Expected: Empty list `[]` or any existing DLQ events — no errors.

---

### Task 18: Validation — k6 Load Test

- [ ] **Step 1: Run k6 load test**

Run:

```bash
make k8s-load DURATION=60s BASE_RATE=2
```

Expected: k6 completes without HTTP error thresholds being breached. Watch for:
- `http_req_failed` rate near 0%
- No connection refused errors
- All checks pass

- [ ] **Step 2: If errors occur, check per-service postgres logs**

Run for each failing service:

```bash
kubectl --context=k3d-bookinfo-local logs -n bookinfo statefulset/{service}-postgresql --tail=50
```

---

### Task 19: Validation — Grafana Observability Check

- [ ] **Step 1: Open Grafana and check service dashboards**

Open `http://localhost:3000` (admin/admin).

Check the following:
- Service metrics dashboard: no error rate spikes for any service
- HTTP request duration: no unusual latency increases
- Active connections: each service has its own postgres connection pool

- [ ] **Step 2: Check Tempo for distributed traces**

In Grafana, navigate to Explore > Tempo. Search for traces from the smoke test requests. Verify:
- Traces flow through Gateway -> EventSource -> Sensor -> write service -> PostgreSQL
- No broken spans or error traces

- [ ] **Step 3: Check Loki for postgres connection errors**

In Grafana, navigate to Explore > Loki. Query:

```
{namespace="bookinfo"} |= "connection refused" or |= "FATAL" or |= "could not connect"
```

Expected: No results related to PostgreSQL connection failures.

---

### Task 20: Open PR and Ensure CI Green

- [ ] **Step 1: Create feature branch and push**

```bash
git checkout -b feat/per-service-postgresql
git push -u origin feat/per-service-postgresql
```

Note: If you've been committing on `main`, create the branch from the first commit of this work and cherry-pick or rebase.

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat(chart): per-service PostgreSQL via Bitnami subchart" --body "$(cat <<'EOF'
## Summary

- Add Bitnami PostgreSQL (~16) as a conditional Helm subchart dependency
- Auto-wire DATABASE_URL, STORAGE_BACKEND, RUN_MIGRATIONS in ConfigMap when postgresql.enabled=true
- Right-size resources: 25m/64Mi requests, 100m/128Mi limits, 256Mi PVC per instance
- Simplify 5 service values-local.yaml files (details, ratings, reviews, notification, dlqueue)
- Remove shared deploy/postgres/ StatefulSet
- Update Makefile: remove shared postgres step, retarget seed commands, add helm dependency build

## Test plan

- [ ] `make helm-lint` passes for all services
- [ ] `helm template` renders correct DATABASE_URL for each service
- [ ] `make stop-k8s && make run-k8s` deploys successfully with per-service PostgreSQL pods
- [ ] Manual smoke tests: POST/GET roundtrip for details, ratings, reviews
- [ ] `make k8s-load DURATION=60s` completes without errors
- [ ] Grafana: no error spikes, traces flowing, no postgres connection errors in Loki
- [ ] CI checks green (helm-lint-test, golangci-lint, go test)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Monitor CI checks**

Run:

```bash
gh pr checks --watch
```

Expected: All checks pass (helm-lint-test, ci, codeql-analysis).

- [ ] **Step 4: If CI fails, diagnose and fix**

Common issues:
- `helm lint` fails: missing `helm dependency build` in CI — check if the `helm-lint-test.yml` workflow runs `helm dependency build` before `ct lint`. If not, add it.
- Schema validation fails: ensure `values.schema.json` is valid JSON with the postgresql property.
- Go tests fail: unlikely since no Go code changed, but check if any test depends on the shared postgres hostname.

Fix any issues, commit, and push. The PR checks will re-run automatically.
