#!/usr/bin/env bash
set -euo pipefail

BASE=http://localhost:8084
ADMIN=http://localhost:9094

# Health check
STATUS=$(curl -sf "$ADMIN/healthz" -o /dev/null -w "%{http_code}")
[ "$STATUS" = "200" ] || { echo "FAIL: healthz returned $STATUS"; exit 1; }

# Create a notification
HTTP_CODE=$(curl -s -o /tmp/notif-resp.json -w "%{http_code}" -X POST "$BASE/v1/notifications" \
    -H "Content-Type: application/json" \
    -d '{"recipient":"alice@example.com","subject":"New review posted","message":"A new review was submitted for your book.","channel":"email"}')
[ "$HTTP_CODE" = "201" ] || { echo "FAIL: expected 201 for notification creation, got $HTTP_CODE"; exit 1; }

ID=$(python3 -c "import sys,json; print(json.load(open('/tmp/notif-resp.json'))['id'])")
[ -n "$ID" ] || { echo "FAIL: no id in notification response"; exit 1; }

NOTIFICATION_STATUS=$(python3 -c "import sys,json; print(json.load(open('/tmp/notif-resp.json'))['status'])")
[ "$NOTIFICATION_STATUS" = "sent" ] || { echo "FAIL: expected status=sent, got $NOTIFICATION_STATUS"; exit 1; }

# Get notification by ID
RESP=$(curl -sf "$BASE/v1/notifications/$ID")
GOT_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ "$GOT_ID" = "$ID" ] || { echo "FAIL: GET /v1/notifications/$ID returned wrong id: $GOT_ID"; exit 1; }

# List notifications by recipient
RESP=$(curl -sf "$BASE/v1/notifications?recipient=alice@example.com")
COUNT=$(echo "$RESP" | python3 -c "
import sys,json
data=json.load(sys.stdin)
items=data if isinstance(data,list) else data.get('notifications',data.get('items',[]))
print(len(items))
")
[ "$COUNT" -ge 1 ] || { echo "FAIL: expected at least 1 notification for recipient, got $COUNT"; exit 1; }

echo "notification: all tests passed"
