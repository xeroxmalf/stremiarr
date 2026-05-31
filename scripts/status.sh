#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

echo "=== Stack Status ==="

if [ ! -f "$COMPOSE" ]; then
  echo "No docker-compose.yml found in $PROJECT/compose"
  exit 1
fi

echo ""
echo "--- Containers (from docker-compose.yml) ---"
docker compose -f "$COMPOSE" ps

echo ""
echo "--- Quick Health (GET-only, local) ---"
HOST="127.0.0.1"

check() {
  local label="$1"
  local url="$2"
  if timeout 3 wget -q -O /dev/null --spider "$url" 2>/dev/null; then
    echo "  OK:   $label"
  else
    echo "  FAIL: $label"
  fi
}

check "caddy-metrics" "http://$HOST:2019/metrics"
check "rclone-http"   "http://$HOST:9933/"
check "comet"         "http://$HOST:8000/"

echo ""
echo "For full live checks: ./scripts/ci.sh --live"
echo "For config drift:     ./scripts/drift_detect.sh"
exit 0
