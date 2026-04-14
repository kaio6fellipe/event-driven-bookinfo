# k6 DLQ Resilience Test — Design Specification

**Date:** 2026-04-14
**Status:** Draft
**Scope:** New k6 test script + Makefile target for DLQ lifecycle validation

## Overview

A dedicated k6 test that validates the dead letter queue lifecycle end-to-end: induce failures by scaling down ratings-write, submit events through the gateway, verify DLQ capture, scale back up, replay events, and verify the data was successfully processed.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Test isolation | Separate k6 script, not mixed into main load test | DLQ test requires failure injection (scale-down); mixing into happy-path test would pollute results |
| Failure injection target | ratings-write | Simplest service, single endpoint, two triggers (create-rating + notify-rating-submitted), cleanest verification |
| Failure method | `kubectl scale --replicas=0` | Deterministic, reversible, no code changes needed |
| Scope | Full cycle: ingest + replay + verify | Replay is the whole point of DLQ — testing ingest alone covers half the system |
| DLQ API access | kubectl port-forward during test | DLQ isn't exposed via Gateway HTTPRoutes; port-forward is the simplest approach |
| Orchestration | Makefile target wrapping kubectl + k6 | kubectl handles scale operations; k6 handles HTTP testing with structured checks |

## Test Flow

```
1. [Makefile] Scale ratings-write to 0 replicas
2. [Makefile] Start port-forward to dlqueue service (port 18085)
3. [Makefile] Run k6 dlq-resilience.js
4. [k6 setup] Wait for ratings-write to be fully down
5. [k6 default] Submit ratings through gateway (POST /v1/ratings)
6. [k6 default] Wait ~20s for sensor retry exhaustion
7. [k6 default] Query DLQ API: GET /v1/events?status=pending&sensor_name=ratings-sensor
8. [k6 default] Verify events landed with correct metadata
9. [Makefile] Scale ratings-write back to 1, wait for ready
10. [k6 default] Replay events: POST /v1/events/{id}/replay
11. [k6 default] Verify ratings created: GET /v1/ratings/{product_id}
12. [k6 default] Verify DLQ status transitioned to replayed
13. [k6 teardown] Resolve all test DLQ events via batch resolve
14. [Makefile] Kill port-forward
```

## New Files

### `test/k6/dlq-resilience.js`

k6 test script with three phases:

**Setup:** Verify ratings-write is down (GET /v1/ratings/dlq-test-product returns timeout or error through gateway write path).

**Default function (single iteration):**
1. Submit 3 ratings with unique product IDs through the gateway
2. Sleep 20s for sensor retries to exhaust
3. Query DLQ API for pending events from `ratings-sensor`
4. Verify event count >= 3 (create-rating triggers; notify triggers may also land)
5. Verify event metadata: `sensor_name`, `failed_trigger`, `status: pending`
6. Sleep 5s for ratings-write scale-up to complete (Makefile scales between k6 phases)
7. Replay each pending event via `POST /v1/events/{id}/replay`
8. Sleep 5s for async processing
9. Verify ratings exist via `GET /v1/ratings/{product_id}`
10. Verify DLQ events transitioned to `replayed` status

**Teardown:** Batch resolve all test DLQ events.

**Environment variables:**
- `BASE_URL` — gateway URL (default: `http://localhost:8080`)
- `DLQ_URL` — DLQ service URL via port-forward (default: `http://localhost:18085`)
- `PHASE` — controls which phase to run: `inject` (steps 1-4), `replay` (steps 6-10). The Makefile runs k6 twice with different PHASE values, doing the scale-up between them.

### Makefile target

```makefile
k8s-dlq-test:
    # 1. Scale down ratings-write
    # 2. Port-forward dlqueue
    # 3. Run k6 PHASE=inject (submit + verify DLQ)
    # 4. Scale up ratings-write, wait for ready
    # 5. Run k6 PHASE=replay (replay + verify data)
    # 6. Kill port-forward
```

## k6 Checks

| Check | Phase | What it verifies |
|---|---|---|
| `submit accepted` | inject | Gateway accepted the POST (EventSource received it) |
| `dlq events found` | inject | Events landed in DLQ after retry exhaustion |
| `dlq status pending` | inject | Events are in pending state |
| `dlq sensor correct` | inject | Events attributed to ratings-sensor |
| `dlq trigger correct` | inject | `failed_trigger` is `create-rating` |
| `replay 200` | replay | Replay API returned success |
| `rating exists` | replay | Rating data was created after replay |
| `dlq status replayed` | replay | Events transitioned from pending to replayed |

## Timing Budget

| Phase | Duration | What happens |
|---|---|---|
| Scale down | ~5s | ratings-write terminates |
| Submit ratings | ~1s | 3 POST requests through gateway |
| Retry exhaustion | ~20s | Sensor retries 3× with exponential backoff |
| DLQ verification | ~2s | Query DLQ API |
| Scale up + ready | ~15s | ratings-write starts, passes readiness probe |
| Replay | ~3s | 3 replay requests |
| Data verification | ~5s | Check ratings exist |
| **Total** | **~50s** | |

## What Stays Unchanged

- `test/k6/bookinfo-traffic.js` — existing load test untouched
- All Helm chart templates — no changes
- DLQ service code — no changes
- Sensor retry configuration — tested as-is
