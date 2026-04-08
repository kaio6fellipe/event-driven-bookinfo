#!/usr/bin/env bash
set -euo pipefail

BASE=http://localhost:8081
ADMIN=http://localhost:9091

# Health check
STATUS=$(curl -sf "$ADMIN/healthz" -o /dev/null -w "%{http_code}")
[ "$STATUS" = "200" ] || { echo "FAIL: healthz returned $STATUS"; exit 1; }

# Create a book detail
RESP=$(curl -sf -X POST "$BASE/v1/details" \
    -H "Content-Type: application/json" \
    -d '{"title":"The Go Programming Language","author":"Alan A. A. Donovan","year":2015,"type":"paperback","pages":380,"publisher":"Addison-Wesley","language":"English","isbn_10":"0134190440","isbn_13":"9780134190440"}')
ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ -n "$ID" ] || { echo "FAIL: no id in response"; exit 1; }

TITLE=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['title'])")
[ "$TITLE" = "The Go Programming Language" ] || { echo "FAIL: expected title 'The Go Programming Language', got $TITLE"; exit 1; }

# Create a second book to verify list
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1/details" \
    -H "Content-Type: application/json" \
    -d '{"title":"Concurrency in Go","author":"Katherine Cox-Buday","year":2017,"type":"paperback","pages":238,"publisher":"O Reilly","language":"English","isbn_10":"1491941197","isbn_13":"9781491941195"}')
[ "$HTTP_CODE" = "201" ] || { echo "FAIL: expected 201 for book creation, got $HTTP_CODE"; exit 1; }

# Get detail by ID
RESP=$(curl -sf "$BASE/v1/details/$ID")
GOT_TITLE=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['title'])")
[ "$GOT_TITLE" = "The Go Programming Language" ] || { echo "FAIL: GET /v1/details/$ID returned wrong title: $GOT_TITLE"; exit 1; }

# List all details
RESP=$(curl -sf "$BASE/v1/details")
COUNT=$(echo "$RESP" | python3 -c "import sys,json; data=json.load(sys.stdin); items=data if isinstance(data,list) else data.get('items',data.get('details',[])); print(len(items))")
[ "$COUNT" -ge 2 ] || { echo "FAIL: expected at least 2 books in list, got $COUNT"; exit 1; }

# Missing required field (empty title) should return 400
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/v1/details" \
    -H "Content-Type: application/json" \
    -d '{"title":"","author":"Test Author","year":2020,"type":"paperback","pages":100}')
[ "$HTTP_CODE" = "400" ] || { echo "FAIL: expected 400 for empty title, got $HTTP_CODE"; exit 1; }

echo "details: all tests passed"
