#!/usr/bin/env bash
set -euo pipefail

BASE=http://localhost:8080
ADMIN=http://localhost:9090
DETAILS_BASE=http://localhost:8081
RATINGS_BASE=http://localhost:8083
REVIEWS_BASE=http://localhost:8082

# Health check
STATUS=$(curl -sf "$ADMIN/healthz" -o /dev/null -w "%{http_code}")
[ "$STATUS" = "200" ] || { echo "FAIL: healthz returned $STATUS"; exit 1; }

# Seed: create a book detail
DETAIL_RESP=$(curl -sf -X POST "$DETAILS_BASE/v1/details" \
    -H "Content-Type: application/json" \
    -d '{"title":"Distributed Systems","author":"Maarten Van Steen","isbn":"978-9081540629","pages":596,"language":"English","publisher":"CreateSpace"}')
DETAIL_ID=$(echo "$DETAIL_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ -n "$DETAIL_ID" ] || { echo "FAIL: could not seed detail, no id returned"; exit 1; }

# Seed: create a rating for that product
curl -sf -X POST "$RATINGS_BASE/v1/ratings" \
    -H "Content-Type: application/json" \
    -d "{\"product_id\":\"$DETAIL_ID\",\"reviewer\":\"e2e-user\",\"stars\":5}" > /dev/null

# Seed: create a review for that product
curl -sf -X POST "$REVIEWS_BASE/v1/reviews" \
    -H "Content-Type: application/json" \
    -d "{\"product_id\":\"$DETAIL_ID\",\"reviewer\":\"e2e-user\",\"text\":\"Excellent book!\",\"stars\":5}" > /dev/null

# GET / should return 200 and contain "Products"
BODY=$(curl -sf "$BASE/")
echo "$BODY" | grep -qi "Products" || { echo "FAIL: GET / does not contain 'Products'"; exit 1; }

# GET /v1/products should return 200 and a list
HTTP_CODE=$(curl -s -o /tmp/products-resp.json -w "%{http_code}" "$BASE/v1/products")
[ "$HTTP_CODE" = "200" ] || { echo "FAIL: GET /v1/products returned $HTTP_CODE"; exit 1; }

# GET /v1/products/{id} should return 200 and contain detail info
HTTP_CODE=$(curl -s -o /tmp/product-detail-resp.json -w "%{http_code}" "$BASE/v1/products/$DETAIL_ID")
[ "$HTTP_CODE" = "200" ] || { echo "FAIL: GET /v1/products/$DETAIL_ID returned $HTTP_CODE"; exit 1; }
python3 -c "
import sys,json
data=json.load(open('/tmp/product-detail-resp.json'))
assert data, 'FAIL: empty product detail response'
" || { echo "FAIL: /v1/products/$DETAIL_ID response is not valid JSON or is empty"; exit 1; }

# GET /products/{id} (HTML page) should return 200
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/products/$DETAIL_ID")
[ "$HTTP_CODE" = "200" ] || { echo "FAIL: GET /products/$DETAIL_ID returned $HTTP_CODE"; exit 1; }

echo "productpage: all tests passed"
