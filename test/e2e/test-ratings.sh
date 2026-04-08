#!/usr/bin/env bash
set -euo pipefail

BASE=http://localhost:8083
ADMIN=http://localhost:9093

# Health check
STATUS=$(curl -sf "$ADMIN/healthz" -o /dev/null -w "%{http_code}")
[ "$STATUS" = "200" ] || { echo "FAIL: healthz returned $STATUS"; exit 1; }

# Submit rating
RESP=$(curl -sf -X POST "$BASE/v1/ratings" \
    -H "Content-Type: application/json" \
    -d '{"product_id":"test-product","reviewer":"alice","stars":5}')
ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ -n "$ID" ] || { echo "FAIL: no id in response"; exit 1; }

# Get ratings for product
RESP=$(curl -sf "$BASE/v1/ratings/test-product")
COUNT=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['count'])")
[ "$COUNT" -ge 1 ] || { echo "FAIL: expected count >= 1, got $COUNT"; exit 1; }

# Invalid rating (stars=0) should return 400
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1/ratings" \
    -H "Content-Type: application/json" \
    -d '{"product_id":"test-product","reviewer":"bob","stars":0}')
[ "$HTTP_CODE" = "400" ] || { echo "FAIL: expected 400 for invalid stars, got $HTTP_CODE"; exit 1; }

echo "ratings: all tests passed"
