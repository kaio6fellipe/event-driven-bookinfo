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
  tags: {
    testid: `bookinfo-${new Date().toISOString().slice(0, 19)}`,
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
  const homeRes = http.get(`${BASE_URL}/`, { tags: { name: 'GET /' } });
  check(homeRes, { 'home 200': (r) => r.status === 200 });

  if (productId) {
    // Product page: productpage -> details + reviews + ratings
    const productRes = http.get(`${BASE_URL}/products/${productId}`, { tags: { name: 'GET /products/{id}' } });
    check(productRes, { 'product 200': (r) => r.status === 200 });

    // HTMX partials
    const detailsRes = http.get(`${BASE_URL}/partials/details/${productId}`, { tags: { name: 'GET /partials/details/{id}' } });
    check(detailsRes, { 'details 200': (r) => r.status === 200 });

    const reviewsRes = http.get(`${BASE_URL}/partials/reviews/${productId}`, { tags: { name: 'GET /partials/reviews/{id}' } });
    check(reviewsRes, { 'reviews 200': (r) => r.status === 200 });

    // --- Write path ---
    // Submit rating + review: productpage -> gateway -> EventSource -> Sensor -> write services
    const reviewers = ['alice', 'bob', 'carol', 'dave', 'eve'];
    const reviewer = reviewers[Math.floor(Math.random() * reviewers.length)];
    const stars = Math.floor(Math.random() * 5) + 1;

    const ratingRes = http.post(`${BASE_URL}/partials/rating`, {
      product_id: productId,
      reviewer: reviewer,
      stars: String(stars),
      text: `k6 load test review by ${reviewer}`,
    }, { tags: { name: 'POST /partials/rating' } });
    check(ratingRes, { 'rating 200': (r) => r.status === 200 });
  }

  // Small random sleep to add jitter between requests within an iteration
  sleep(Math.random() * 0.5);
}

export function teardown(data) {
  const productId = data.productId;
  if (!productId) return;

  // Phase 1: Wait for all pending reviews to be confirmed (no more "Processing" badges)
  console.log('Waiting for all pending reviews to be processed...');
  for (let attempt = 1; attempt <= 30; attempt++) {
    const res = http.get(`${BASE_URL}/partials/reviews/${productId}`,
      { tags: { name: 'teardown: check pending' } });
    if (res.status === 200 && !res.body.includes('Processing')) {
      console.log(`All reviews confirmed after ${attempt} checks.`);
      break;
    }
    if (attempt === 30) {
      console.log('Warning: some reviews still pending after 60s, proceeding with cleanup.');
    }
    sleep(2);
  }

  // Phase 2: Delete all k6-generated reviews
  console.log('Cleaning up k6-generated reviews...');
  const maxRounds = 5;
  let totalDeleted = 0;

  for (let round = 1; round <= maxRounds; round++) {
    let page = 1;
    let totalPages = 1;
    let roundDeleted = 0;

    while (page <= totalPages) {
      const res = http.get(`${BASE_URL}/v1/reviews/${productId}?page=${page}&page_size=100`,
        { tags: { name: 'teardown: GET reviews' } });

      if (res.status !== 200) {
        console.log(`Failed to fetch reviews page ${page}: status ${res.status}`);
        break;
      }

      const body = JSON.parse(res.body);
      totalPages = body.pagination.total_pages;

      for (const review of body.reviews) {
        if (review.text && review.text.startsWith('k6 load test review')) {
          http.post(`${BASE_URL}/v1/reviews/delete`,
            JSON.stringify({ review_id: review.id }),
            { headers: { 'Content-Type': 'application/json' }, tags: { name: 'teardown: DELETE review' } });
          roundDeleted++;
        }
      }

      page++;
    }

    totalDeleted += roundDeleted;
    if (roundDeleted === 0) {
      console.log(`Round ${round}: no k6 reviews found. Cleanup complete.`);
      break;
    }

    console.log(`Round ${round}: sent ${roundDeleted} delete requests. Waiting for async processing...`);
    sleep(10);
  }

  console.log(`Cleanup total: ${totalDeleted} delete requests sent.`);
}
