#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE="$PROJECT/compose/docker-compose.yml"

echo "=== Quick Local Sanity Check ==="

# 1) Ensure compose config is valid
if docker compose -f "$COMPOSE" config >/dev/null 2>&1; then
  echo "OK: docker-compose config valid"
else
  echo "FAIL: docker-compose config invalid"
  exit 1
fi

# 2) Ensure required files exist
for f in caddy/Caddyfile rclone/rclone.conf handoff/src/main.go handoff/src/Dockerfile; do
  if [ -f "$PROJECT/$f" ]; then
    echo "OK: $f exists"
  else
    echo "FAIL: $f missing"
    exit 1
  fi
done

# 3) Run CI (static, fast)
echo ""
echo "Running CI (static checks only)..."
if "$SCRIPT_DIR/ci.sh"; then
  echo "OK: CI passed"
else
  echo "FAIL: CI failed"
  exit 1
fi

echo ""
echo "=== Quick sanity check: ALL OK ==="
exit 0
