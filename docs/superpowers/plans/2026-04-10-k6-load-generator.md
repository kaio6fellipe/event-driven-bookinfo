# k6 Load Generator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `make k8s-traffic` with a k6-based load generator that runs locally via Docker and continuously inside the cluster via CronJob, with Prometheus metrics and a Grafana dashboard.

**Architecture:** A single k6 JavaScript test script (`test/k6/bookinfo-traffic.js`) exercises all productpage routes with variable request rates. It's used by both a Docker-based local runner (`make k8s-load`) and a Kubernetes CronJob that runs 5-minute load sessions every 10 minutes. k6 pushes metrics to Prometheus via remote write, visualized in a dedicated Grafana dashboard.

**Tech Stack:** k6 (Grafana), Docker (`grafana/k6` image), Kubernetes CronJob, Prometheus remote write, Grafana dashboard

**Spec:** `docs/superpowers/specs/2026-04-10-k6-load-generator-design.md`

---

## File Map

| File | Action | Responsibility |
| --- | --- | --- |
| `test/k6/bookinfo-traffic.js` | Create | k6 test script with read + write paths, variable rate stages |
| `deploy/k6/base/configmap.yaml` | Create | k6 script as ConfigMap for in-cluster CronJob |
| `deploy/k6/base/cronjob.yaml` | Create | CronJob running `grafana/k6` every 10 minutes |
| `deploy/k6/overlays/local/kustomization.yaml` | Create | Kustomize overlay for local cluster |
| `deploy/observability/local/dashboards/k6-load-testing.json` | Create | Grafana k6 dashboard |
| `deploy/observability/local/dashboards/kustomization.yaml` | Modify | Add k6 dashboard ConfigMap |
| `Makefile` | Modify | Replace `k8s-traffic` with `k8s-load`, add `k8s-load-start`/`k8s-load-stop` |

---

### Task 1: Create k6 Test Script

**Files:**
- Create: `test/k6/bookinfo-traffic.js`

- [ ] **Step 1: Create the k6 test script**

Create `test/k6/bookinfo-traffic.js`:

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

// Configuration via environment variables with defaults
const BASE_URL = __ENV.BASE_URL || 'http://host.docker.internal:8080';
const BASE_RATE = parseInt(__ENV.BASE_RATE || '2', 10);
const DURATION = __ENV.DURATION || '30s';

// Parse duration string to seconds for stage calculation
function parseDuration(d) {
  const match = d.match(/^(\d+)(s|m|h)$/);
  if (!match) return 30;
  const val = parseInt(match[1], 10);
  switch (match[2]) {
    case 'h': return val * 3600;
    case 'm': return val * 60;
    default:  return val;
  }
}

const totalSeconds = parseDuration(DURATION);

// Build aleatory stages: cycle through variable rates
// Each stage is ~1/6 of total duration
function buildStages() {
  const segment = Math.max(Math.ceil(totalSeconds / 6), 1);
  return [
    { duration: `${segment}s`, target: BASE_RATE },                    // base
    { duration: `${segment}s`, target: Math.ceil(BASE_RATE * 2) },     // ramp up
    { duration: `${segment}s`, target: BASE_RATE },                    // base
    { duration: `${segment}s`, target: Math.max(1, Math.ceil(BASE_RATE * 0.5)) }, // low
    { duration: `${segment}s`, target: Math.ceil(BASE_RATE * 3) },     // spike
    { duration: `${segment}s`, target: BASE_RATE },                    // base
  ];
}

export const options = {
  discardResponseBodies: true,
  scenarios: {
    bookinfo: {
      executor: 'ramping-arrival-rate',
      startRate: BASE_RATE,
      timeUnit: '1s',
      preAllocatedVUs: 5,
      maxVUs: 20,
      stages: buildStages(),
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.1'],
  },
};

// Discover a product ID from the home page
function discoverProductId() {
  const res = http.get(`${BASE_URL}/`);
  if (res.status === 200 && res.body) {
    const match = res.body.match(/\/products\/([^"]+)/);
    if (match) return match[1];
  }
  return null;
}

// Cache the product ID across iterations (discovered in setup)
export function setup() {
  const productId = discoverProductId();
  return { productId };
}

export default function (data) {
  const productId = data.productId;

  // --- Read path ---
  // Home page: productpage -> details fan-out
  const homeRes = http.get(`${BASE_URL}/`);
  check(homeRes, { 'home 200': (r) => r.status === 200 });

  if (productId) {
    // Product page: productpage -> details + reviews + ratings
    http.get(`${BASE_URL}/products/${productId}`);

    // HTMX partials
    http.get(`${BASE_URL}/partials/details/${productId}`);
    http.get(`${BASE_URL}/partials/reviews/${productId}`);

    // --- Write path ---
    // Submit rating + review: productpage -> gateway -> EventSource -> Sensor -> write services
    const reviewers = ['alice', 'bob', 'carol', 'dave', 'eve'];
    const reviewer = reviewers[Math.floor(Math.random() * reviewers.length)];
    const stars = Math.floor(Math.random() * 5) + 1;

    http.post(`${BASE_URL}/partials/rating`, {
      product_id: productId,
      reviewer: reviewer,
      stars: String(stars),
      text: `k6 load test review by ${reviewer}`,
    });
  }

  // Small random sleep to add jitter between requests within an iteration
  sleep(Math.random() * 0.5);
}
```

- [ ] **Step 2: Verify the script parses correctly with Docker**

```bash
docker run --rm -v $(pwd)/test/k6:/scripts grafana/k6 inspect /scripts/bookinfo-traffic.js
```

Expected: JSON output showing the script's options (scenarios, thresholds) without errors.

- [ ] **Step 3: Commit**

```bash
git add test/k6/bookinfo-traffic.js
git commit -m "feat(k6): add bookinfo traffic load test script

Exercises all productpage routes (read + write paths) with configurable
ramping-arrival-rate stages for aleatory traffic volume."
```

---

### Task 2: Replace `make k8s-traffic` with `make k8s-load`

**Files:**
- Modify: `Makefile:464-491` (Traffic section)

- [ ] **Step 1: Replace the Traffic section in the Makefile**

Replace the entire `# ─── Traffic` section (from line 464 to the blank line before `# ─── Help`) with:

```makefile
# ─── Load Testing ────────────────────────────────────────────────────────────

DURATION  ?= 30s
BASE_RATE ?= 2

.PHONY: k8s-load
k8s-load: ##@Kubernetes Run k6 load test via Docker (default 30s). Usage: DURATION=5m BASE_RATE=3 make k8s-load
	@printf "$(BOLD)Running k6 load test (duration=$(DURATION), rate=$(BASE_RATE) req/s)...$(NC)\n"
	@docker run --rm \
		-v $(CURDIR)/test/k6:/scripts \
		-e BASE_URL=http://host.docker.internal:8080 \
		-e DURATION=$(DURATION) \
		-e BASE_RATE=$(BASE_RATE) \
		-e K6_PROMETHEUS_RW_SERVER_URL=http://host.docker.internal:9090/api/v1/write \
		grafana/k6 run -o experimental-prometheus-rw /scripts/bookinfo-traffic.js

.PHONY: k8s-load-start
k8s-load-start: ##@Kubernetes Deploy k6 CronJob for continuous background load
	$(k8s-guard)
	@printf "$(BOLD)Deploying k6 load generator CronJob...$(NC)\n"
	@$(KUBECTL) apply -k deploy/k6/overlays/local/
	@printf "$(GREEN)k6 CronJob deployed. Runs every 10 minutes in namespace $(K8S_NS_BOOKINFO).$(NC)\n"
	@printf "Use $(CYAN)make k8s-load-stop$(NC) to remove.\n"

.PHONY: k8s-load-stop
k8s-load-stop: ##@Kubernetes Remove k6 CronJob from the cluster
	$(k8s-guard)
	@printf "$(BOLD)Removing k6 load generator CronJob...$(NC)\n"
	@$(KUBECTL) delete -k deploy/k6/overlays/local/ --ignore-not-found
	@printf "$(GREEN)k6 CronJob removed.$(NC)\n"
```

- [ ] **Step 2: Verify the Makefile parses correctly**

```bash
make help | grep k8s-load
```

Expected: Three entries: `k8s-load`, `k8s-load-start`, `k8s-load-stop`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat(makefile): replace k8s-traffic with k8s-load using k6 Docker runner

Adds k8s-load (local Docker k6 run), k8s-load-start (deploy CronJob),
and k8s-load-stop (remove CronJob). Removes old k8s-traffic target."
```

---

### Task 3: Create Kubernetes CronJob Manifests

**Files:**
- Create: `deploy/k6/base/configmap.yaml`
- Create: `deploy/k6/base/cronjob.yaml`
- Create: `deploy/k6/base/kustomization.yaml`
- Create: `deploy/k6/overlays/local/kustomization.yaml`

- [ ] **Step 1: Create the base kustomization**

Create `deploy/k6/base/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - cronjob.yaml

configMapGenerator:
  - name: k6-bookinfo-traffic
    options:
      disableNameSuffixHash: true
    files:
      - bookinfo-traffic.js=../../../test/k6/bookinfo-traffic.js
```

- [ ] **Step 2: Create the CronJob manifest**

Create `deploy/k6/base/cronjob.yaml`:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k6-load-generator
  labels:
    app: k6-load-generator
    part-of: event-driven-bookinfo
spec:
  schedule: "*/10 * * * *"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 1
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      backoffLimit: 0
      ttlSecondsAfterFinished: 300
      template:
        metadata:
          labels:
            app: k6-load-generator
        spec:
          restartPolicy: Never
          containers:
            - name: k6
              image: grafana/k6:latest
              command:
                - k6
                - run
                - -o
                - experimental-prometheus-rw
                - /scripts/bookinfo-traffic.js
              env:
                - name: BASE_URL
                  value: "http://gateway.envoy-gateway-system.svc.cluster.local"
                - name: DURATION
                  value: "5m"
                - name: BASE_RATE
                  value: "1"
                - name: K6_PROMETHEUS_RW_SERVER_URL
                  value: "http://prometheus-kube-prometheus-prometheus.observability.svc.cluster.local:9090/api/v1/write"
              volumeMounts:
                - name: k6-scripts
                  mountPath: /scripts
                  readOnly: true
              resources:
                requests:
                  cpu: 50m
                  memory: 64Mi
                limits:
                  cpu: 200m
                  memory: 128Mi
          volumes:
            - name: k6-scripts
              configMap:
                name: k6-bookinfo-traffic
```

- [ ] **Step 3: Create the local overlay**

Create `deploy/k6/overlays/local/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: bookinfo

resources:
  - ../../base
```

- [ ] **Step 4: Verify kustomize renders correctly**

```bash
kubectl kustomize deploy/k6/overlays/local/
```

Expected: Valid YAML output with CronJob and ConfigMap in the `bookinfo` namespace.

- [ ] **Step 5: Commit**

```bash
git add deploy/k6/
git commit -m "feat(k6): add CronJob for continuous in-cluster load generation

Runs k6 every 10 minutes with 5m duration and 1 req/s base rate.
Script mounted from ConfigMap, metrics pushed to Prometheus."
```

---

### Task 4: Add k6 Grafana Dashboard

**Files:**
- Create: `deploy/observability/local/dashboards/k6-load-testing.json`
- Modify: `deploy/observability/local/dashboards/kustomization.yaml`

- [ ] **Step 1: Download the official k6 Prometheus dashboard and save it**

Fetch the official k6 + Prometheus remote write dashboard (Grafana dashboard ID 19665) and save it. Use the Grafana.com API to download:

```bash
curl -s https://grafana.com/api/dashboards/19665/revisions/latest/download | python3 -c "
import json, sys
d = json.load(sys.stdin)
# Patch the datasource to use the local Prometheus uid
for panel in d.get('panels', []):
    if 'datasource' in panel:
        panel['datasource'] = {'type': 'prometheus', 'uid': 'prometheus'}
    for target in panel.get('targets', []):
        if 'datasource' in target:
            target['datasource'] = {'type': 'prometheus', 'uid': 'prometheus'}
# Remove id so Grafana auto-assigns
d.pop('id', None)
d['uid'] = 'k6-load-testing'
d['title'] = 'k6 Load Testing'
print(json.dumps(d, indent=2))
" > deploy/observability/local/dashboards/k6-load-testing.json
```

If the download or patching fails, create a minimal custom dashboard. The key panels are:

- Request Rate: `sum(rate(k6_http_reqs_total[1m]))`
- Error Rate: `sum(rate(k6_http_reqs_total{expected_response="false"}[1m]))`
- P95 Latency: `histogram_quantile(0.95, sum(rate(k6_http_req_duration_seconds_bucket[1m])) by (le))`
- Active VUs: `k6_vus`
- Iterations: `sum(rate(k6_iterations_total[1m]))`

- [ ] **Step 2: Add the dashboard to the kustomization**

In `deploy/observability/local/dashboards/kustomization.yaml`, add the k6 dashboard ConfigMap entry:

```yaml
  - name: grafana-dashboard-k6-load-testing
    options:
      disableNameSuffixHash: true
      labels:
        grafana_dashboard: "1"
    files:
      - k6-load-testing.json
```

Append this to the existing `configMapGenerator` list.

- [ ] **Step 3: Verify kustomize renders with the new dashboard**

```bash
kubectl kustomize deploy/observability/local/dashboards/ | head -20
```

Expected: ConfigMap for `grafana-dashboard-k6-load-testing` appears in the output.

- [ ] **Step 4: Commit**

```bash
git add deploy/observability/local/dashboards/k6-load-testing.json deploy/observability/local/dashboards/kustomization.yaml
git commit -m "feat(observability): add k6 load testing Grafana dashboard

Official k6 + Prometheus remote write dashboard (ID 19665) patched
for local Prometheus datasource UID."
```

---

### Task 5: Deploy and Verify

**Prerequisites:** The k3d cluster must be running (`make k8s-status`).

- [ ] **Step 1: Run `make k8s-load` locally**

```bash
make k8s-load
```

Expected: k6 runs for 30 seconds via Docker, shows progress output with request rate, latency, and check results. All requests should succeed (check rate ~100%).

- [ ] **Step 2: Verify k6 metrics in Prometheus**

```bash
curl -s 'http://localhost:9090/api/v1/query?query=k6_http_reqs_total' | python3 -c "
import json, sys
data = json.load(sys.stdin)
results = data.get('data', {}).get('result', [])
print(f'k6 metric series: {len(results)}')
for r in results[:3]:
    print(f'  {r[\"metric\"].get(\"url\",\"?\")} count={r[\"value\"][1]}')
"
```

Expected: Non-zero k6 HTTP request metrics.

- [ ] **Step 3: Apply Grafana dashboards**

```bash
kubectl --context=k3d-bookinfo-local apply -k deploy/observability/local/dashboards/
```

Expected: ConfigMaps applied including `grafana-dashboard-k6-load-testing`.

- [ ] **Step 4: Verify k6 dashboard in Grafana**

Open `http://localhost:3000` and navigate to Dashboards. The "k6 Load Testing" dashboard should appear and show data from the previous `make k8s-load` run.

- [ ] **Step 5: Deploy the CronJob for continuous load**

```bash
make k8s-load-start
```

Expected: Output confirms CronJob deployed to `bookinfo` namespace.

- [ ] **Step 6: Verify CronJob is scheduled**

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get cronjobs
```

Expected: `k6-load-generator` CronJob with schedule `*/10 * * * *`.

- [ ] **Step 7: Wait for a CronJob run and verify**

Wait up to 10 minutes for the first CronJob execution, or trigger it manually:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo create job k6-manual-test --from=cronjob/k6-load-generator
```

Then check the job:

```bash
kubectl --context=k3d-bookinfo-local -n bookinfo get jobs -l app=k6-load-generator
kubectl --context=k3d-bookinfo-local -n bookinfo logs job/k6-manual-test
```

Expected: k6 output showing successful requests against the in-cluster gateway.

- [ ] **Step 8: Verify service graph stability**

After the k6 job completes, check the Grafana Tempo service graph. With continuous load, all edges should be consistently visible including `gateway -> eventsource` and database virtual nodes.

- [ ] **Step 9: Test `make k8s-load-stop`**

```bash
make k8s-load-stop
```

Expected: CronJob removed from the cluster.

- [ ] **Step 10: Commit any fixes from verification**

If any adjustments were needed during verification, commit them now.
