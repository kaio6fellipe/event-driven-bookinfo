#!/usr/bin/env bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Start docker-compose
echo -e "${YELLOW}Starting services...${NC}"
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up -d --build

# Cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}Stopping services...${NC}"
    docker compose -f "$SCRIPT_DIR/docker-compose.yml" down -v
}
trap cleanup EXIT

# Wait for services to be ready (poll health endpoints from host)
wait_for_service() {
    local name=$1 url=$2 max=30
    echo -n "Waiting for $name..."
    for i in $(seq 1 $max); do
        if curl -sf "$url" > /dev/null 2>&1; then
            echo -e " ${GREEN}ready${NC}"
            return 0
        fi
        sleep 1
        echo -n "."
    done
    echo -e " ${RED}timeout${NC}"
    return 1
}

wait_for_service "ratings"       "http://localhost:9093/healthz"
wait_for_service "details"       "http://localhost:9091/healthz"
wait_for_service "reviews"       "http://localhost:9092/healthz"
wait_for_service "notification"  "http://localhost:9094/healthz"
wait_for_service "productpage"   "http://localhost:9090/healthz"

# Run tests
FAILED=0
run_test() {
    local script=$1
    echo -e "\n${YELLOW}Running $script...${NC}"
    if bash "$SCRIPT_DIR/$script"; then
        echo -e "${GREEN}PASS: $script${NC}"
    else
        echo -e "${RED}FAIL: $script${NC}"
        FAILED=1
    fi
}

run_test test-ratings.sh
run_test test-details.sh
run_test test-reviews.sh
run_test test-notification.sh
run_test test-productpage.sh

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}All E2E tests passed!${NC}"
else
    echo -e "\n${RED}Some E2E tests failed.${NC}"
    exit 1
fi
