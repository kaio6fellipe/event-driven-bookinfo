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
