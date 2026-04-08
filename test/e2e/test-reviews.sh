#!/usr/bin/env bash
set -euo pipefail

BASE=http://localhost:8082
ADMIN=http://localhost:9092
RATINGS_BASE=http://localhost:8083

# Health check
STATUS=$(curl -sf "$ADMIN/healthz" -o /dev/null -w "%{http_code}")
[ "$STATUS" = "200" ] || { echo "FAIL: healthz returned $STATUS"; exit 1; }

# Seed a rating for test-product so reviews can fetch it
curl -sf -X POST "$RATINGS_BASE/v1/ratings" \
    -H "Content-Type: application/json" \
    -d '{"product_id":"review-test-product","reviewer":"seed-user","stars":4}' > /dev/null

# Create a review
HTTP_CODE=$(curl -s -o /tmp/review-resp.json -w "%{http_code}" -X POST "$BASE/v1/reviews" \
    -H "Content-Type: application/json" \
    -d '{"product_id":"review-test-product","reviewer":"bob","text":"Great book!","stars":4}')
[ "$HTTP_CODE" = "201" ] || { echo "FAIL: expected 201 for review creation, got $HTTP_CODE"; exit 1; }

ID=$(python3 -c "import sys,json; print(json.load(open('/tmp/review-resp.json'))['id'])")
[ -n "$ID" ] || { echo "FAIL: no id in review response"; exit 1; }

# Get reviews for product
RESP=$(curl -sf "$BASE/v1/reviews/review-test-product")
REVIEWS_COUNT=$(echo "$RESP" | python3 -c "
import sys,json
data=json.load(sys.stdin)
reviews=data if isinstance(data,list) else data.get('reviews',data.get('items',[]))
print(len(reviews))
")
[ "$REVIEWS_COUNT" -ge 1 ] || { echo "FAIL: expected at least 1 review, got $REVIEWS_COUNT"; exit 1; }

echo "reviews: all tests passed"
