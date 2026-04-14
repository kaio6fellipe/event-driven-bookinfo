# k6 DLQ Resilience Test — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a k6 test that validates the DLQ lifecycle end-to-end: induce failures, verify DLQ capture, replay events, verify data was processed.

**Architecture:** Two-phase k6 script (`PHASE=inject` and `PHASE=replay`) orchestrated by a Makefile target that handles kubectl scale operations between phases. DLQ API accessed via port-forward.

**Tech Stack:** k6, kubectl, Makefile

**Spec:** `docs/superpowers/specs/2026-04-14-k6-dlq-resilience-test-design.md`

---

## File Map

### New Files

| File | Responsibility |
|---|---|
| `test/k6/dlq-resilience.js` | k6 test script with inject + replay phases |

### Modified Files

| File | Change |
|---|---|
| `Makefile` | Add `k8s-dlq-test` target |

---

## Task 1: Create k6 DLQ Resilience Test Script

**Files:**
- Create: `test/k6/dlq-resilience.js`

- [ ] **Step 1: Create the k6 test script**

Create `test/k6/dlq-resilience.js`:

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://host.docker.internal:8080';
const DLQ_URL = __ENV.DLQ_URL || 'http://host.docker.internal:18085';
const PHASE = __ENV.PHASE || 'inject'; // 'inject' or 'replay'

// Single iteration — this is a functional test, not a load test
export const options = {
  iterations: 1,
  vus: 1,
  thresholds: {
    checks: ['rate==1.0'],
  },
  tags: {
    testid: `dlq-resilience-${PHASE}-${new Date().toISOString().slice(0, 19)}`,
  },
};

// Test product IDs — unique to avoid collision with other data
const TEST_PRODUCTS = ['dlq-test-p1', 'dlq-test-p2', 'dlq-test-p3'];

export default function () {
  if (PHASE === 'inject') {
    injectPhase();
  } else if (PHASE === 'replay') {
    replayPhase();
  } else {
    console.error(`Unknown PHASE: ${PHASE}. Use 'inject' or 'replay'.`);
  }
}

// Phase 1: Submit ratings while ratings-write is down, verify DLQ capture
function injectPhase() {
  console.log('=== DLQ Inject Phase ===');
  console.log(`Submitting ${TEST_PRODUCTS.length} ratings (ratings-write should be scaled to 0)...`);

  // Submit ratings through the gateway (POST → EventSource → Sensor → ratings-write FAILS)
  for (const productId of TEST_PRODUCTS) {
    const res = http.post(
      `${BASE_URL}/v1/ratings`,
      JSON.stringify({
        product_id: productId,
        stars: 5,
        reviewer: 'dlq-resilience-test',
      }),
      {
        headers: { 'Content-Type': 'application/json' },
        tags: { name: 'POST /v1/ratings (inject)' },
      }
    );
    check(res, {
      'submit accepted': (r) => r.status === 200 || r.body === 'success',
    });
  }

  // Wait for sensor retries to exhaust (3 steps × exponential backoff ≈ 14s, +6s margin)
  console.log('Waiting 20s for sensor retry exhaustion...');
  sleep(20);

  // Query DLQ for pending events from ratings-sensor
  console.log('Checking DLQ for captured events...');
  const dlqRes = http.get(
    `${DLQ_URL}/v1/events?status=pending&sensor_name=ratings-sensor&limit=50`,
    { tags: { name: 'GET /v1/events (verify DLQ)' } }
  );

  const dlqOk = check(dlqRes, {
    'dlq query 200': (r) => r.status === 200,
  });

  if (!dlqOk) {
    console.error(`DLQ query failed: status=${dlqRes.status}, body=${dlqRes.body}`);
    return;
  }

  const dlqData = JSON.parse(dlqRes.body);
  console.log(`DLQ events found: ${dlqData.total_items}`);

  check(dlqData, {
    'dlq events found': (d) => d.total_items >= TEST_PRODUCTS.length,
  });

  // Verify event metadata on first event
  if (dlqData.items && dlqData.items.length > 0) {
    const event = dlqData.items[0];
    check(event, {
      'dlq status pending': (e) => e.status === 'pending',
      'dlq sensor correct': (e) => e.sensor_name === 'ratings-sensor',
      'dlq trigger correct': (e) => e.failed_trigger === 'create-rating',
    });
    console.log(`Sample event: id=${event.id}, trigger=${event.failed_trigger}, status=${event.status}`);
  }

  console.log('=== Inject Phase Complete ===');
}

// Phase 2: Replay DLQ events (ratings-write should be back up), verify data
function replayPhase() {
  console.log('=== DLQ Replay Phase ===');

  // Fetch pending events from ratings-sensor
  const dlqRes = http.get(
    `${DLQ_URL}/v1/events?status=pending&sensor_name=ratings-sensor&limit=50`,
    { tags: { name: 'GET /v1/events (pre-replay)' } }
  );

  check(dlqRes, { 'dlq query 200': (r) => r.status === 200 });
  const dlqData = JSON.parse(dlqRes.body);
  console.log(`Pending DLQ events to replay: ${dlqData.total_items}`);

  if (dlqData.total_items === 0) {
    console.error('No pending DLQ events found — inject phase may not have run.');
    return;
  }

  // Replay each event
  const eventIds = [];
  for (const event of dlqData.items) {
    const replayRes = http.post(
      `${DLQ_URL}/v1/events/${event.id}/replay`,
      null,
      { tags: { name: 'POST /v1/events/{id}/replay' } }
    );
    check(replayRes, {
      'replay 200': (r) => r.status === 200,
    });
    eventIds.push(event.id);
    console.log(`Replayed event ${event.id} (trigger: ${event.failed_trigger})`);
  }

  // Wait for async processing (replay → EventSource → Sensor → ratings-write)
  console.log('Waiting 10s for replayed events to process...');
  sleep(10);

  // Verify ratings were created
  console.log('Verifying ratings were created after replay...');
  for (const productId of TEST_PRODUCTS) {
    const ratingsRes = http.get(
      `${BASE_URL}/v1/ratings/${productId}`,
      { tags: { name: 'GET /v1/ratings/{id} (verify)' } }
    );
    check(ratingsRes, {
      'rating exists': (r) => {
        if (r.status !== 200) return false;
        const data = JSON.parse(r.body);
        return data.count > 0;
      },
    });
  }

  // Verify DLQ events transitioned to replayed
  console.log('Verifying DLQ event status...');
  for (const eventId of eventIds) {
    const eventRes = http.get(
      `${DLQ_URL}/v1/events/${eventId}`,
      { tags: { name: 'GET /v1/events/{id} (verify status)' } }
    );
    if (eventRes.status === 200) {
      const event = JSON.parse(eventRes.body);
      check(event, {
        'dlq status replayed': (e) => e.status === 'replayed',
      });
    }
  }

  // Cleanup: batch resolve all test DLQ events
  console.log('Cleaning up: resolving test DLQ events...');
  if (eventIds.length > 0) {
    const resolveRes = http.post(
      `${DLQ_URL}/v1/events/batch/resolve`,
      JSON.stringify({
        ids: eventIds,
        resolved_by: 'k6-dlq-resilience-test',
      }),
      {
        headers: { 'Content-Type': 'application/json' },
        tags: { name: 'POST /v1/events/batch/resolve' },
      }
    );
    if (resolveRes.status === 200) {
      const result = JSON.parse(resolveRes.body);
      console.log(`Resolved ${result.affected_count} DLQ events.`);
    }
  }

  console.log('=== Replay Phase Complete ===');
}
```

- [ ] **Step 2: Verify script syntax**

Run:
```bash
docker run --rm -v $(pwd)/test/k6:/scripts grafana/k6 inspect /scripts/dlq-resilience.js 2>&1 | head -5
```

Expected: JSON output showing the test configuration (no syntax errors).

- [ ] **Step 3: Commit**

```bash
git add test/k6/dlq-resilience.js
git commit -m "test(k6): add DLQ resilience test script with inject + replay phases"
```

---

## Task 2: Add Makefile Target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add k8s-dlq-test target**

Read the Makefile first. Insert this target in the `# ─── Load Testing ───` section, after the `k8s-load-stop` target and before the `# ─── Helm ───` section:

```makefile
.PHONY: k8s-dlq-test
k8s-dlq-test: ##@Kubernetes Run DLQ resilience test: inject failures, verify DLQ, replay, verify data
	$(k8s-guard)
	@printf "\n$(BOLD)═══ DLQ Resilience Test ═══$(NC)\n\n"
	@printf "$(BOLD)[1/6] Scaling down ratings-write...$(NC)\n"
	@$(KUBECTL) scale deployment/ratings-write -n $(K8S_NS_BOOKINFO) --replicas=0
	@$(KUBECTL) wait deployment/ratings-write -n $(K8S_NS_BOOKINFO) \
		--for=jsonpath='{.status.replicas}'=0 --timeout=30s
	@printf "  $(GREEN)ratings-write scaled to 0.$(NC)\n"
	@printf "$(BOLD)[2/6] Starting port-forward to DLQ service...$(NC)\n"
	@$(KUBECTL) port-forward svc/dlqueue -n $(K8S_NS_BOOKINFO) 18085:80 &
	@sleep 2
	@printf "  $(GREEN)DLQ accessible at localhost:18085.$(NC)\n"
	@printf "$(BOLD)[3/6] Inject phase: submitting events + verifying DLQ capture...$(NC)\n"
	@docker run --rm \
		-v $(CURDIR)/test/k6:/scripts \
		-e BASE_URL=http://host.docker.internal:8080 \
		-e DLQ_URL=http://host.docker.internal:18085 \
		-e PHASE=inject \
		grafana/k6 run /scripts/dlq-resilience.js || \
		($(KUBECTL) scale deployment/ratings-write -n $(K8S_NS_BOOKINFO) --replicas=1; \
		 kill %% 2>/dev/null; exit 1)
	@printf "$(BOLD)[4/6] Restoring ratings-write...$(NC)\n"
	@$(KUBECTL) scale deployment/ratings-write -n $(K8S_NS_BOOKINFO) --replicas=1
	@$(KUBECTL) wait deployment/ratings-write -n $(K8S_NS_BOOKINFO) \
		--for=condition=Available --timeout=60s
	@printf "  $(GREEN)ratings-write restored.$(NC)\n"
	@printf "$(BOLD)[5/6] Replay phase: replaying events + verifying data...$(NC)\n"
	@docker run --rm \
		-v $(CURDIR)/test/k6:/scripts \
		-e BASE_URL=http://host.docker.internal:8080 \
		-e DLQ_URL=http://host.docker.internal:18085 \
		-e PHASE=replay \
		grafana/k6 run /scripts/dlq-resilience.js || \
		(kill %% 2>/dev/null; exit 1)
	@printf "$(BOLD)[6/6] Cleaning up...$(NC)\n"
	@-kill %% 2>/dev/null
	@printf "\n$(GREEN)$(BOLD)DLQ resilience test complete.$(NC)\n\n"
```

- [ ] **Step 2: Verify target appears in help**

Run: `make help | grep dlq-test`
Expected: `k8s-dlq-test` appears with description

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat(make): add k8s-dlq-test target for DLQ resilience testing"
```

---

## Task 3: E2E Validation

Run the DLQ resilience test on the live cluster.

**Files:** None (runtime validation)

- [ ] **Step 1: Verify the cluster is running**

Run: `make k8s-status 2>&1 | head -5`
Expected: Pods in Running state. If cluster is down, run `make run-k8s` first.

- [ ] **Step 2: Run the DLQ resilience test**

Run: `make k8s-dlq-test`

Expected output flow:
1. `[1/6]` ratings-write scaled to 0
2. `[2/6]` port-forward started
3. `[3/6]` Inject phase: 3 ratings submitted, 20s wait, DLQ events found (>= 3), all checks pass
4. `[4/6]` ratings-write restored
5. `[5/6]` Replay phase: events replayed, ratings verified, DLQ status transitioned, cleanup done
6. `[6/6]` Complete

All k6 checks should pass (`checks: 100.00%`).

- [ ] **Step 3: Verify ratings-write is back to normal**

Run:
```bash
kubectl --context=k3d-bookinfo-local get deployment ratings-write -n bookinfo -o jsonpath='{.status.readyReplicas}'
```
Expected: `1`

- [ ] **Step 4: Fix any issues**

If the inject phase fails: check if ratings-write actually scaled down. Check sensor logs for retry behavior. The 20s wait might be too short if the cluster is slow — increase if needed.

If the replay phase fails: check if ratings-write is fully ready. Check DLQ service logs. The replay goes through the full EventSource → Sensor → ratings-write pipeline, so all components must be healthy.

Commit any fixes.

- [ ] **Step 5: Run the normal load test to confirm no side effects**

Run: `make k8s-load DURATION=30s BASE_RATE=2 2>&1 | grep -E 'checks_succeeded|http_req_failed'`
Expected: 100% checks, 0% failures
