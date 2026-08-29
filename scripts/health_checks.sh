#!/usr/bin/env bash
set -euo pipefail

# Non-destructive health checks against live services.
# Uses same endpoints as docker-compose.yml healthchecks.
# Designed to be:
#   - Read-only
#   - Fast (short timeouts)
#   - Safe for production

TIMEOUT=5
HOST="${HOST:-127.0.0.1}"

FAIL=0
FAILURES=()

check_http() {
  local label="$1"
  local url="$2"
  local extra="${3:-}"

  # Use GET instead of --spider (HEAD) for better compatibility with FastAPI
  if [ -n "$extra" ]; then
    if ! timeout "$TIMEOUT" wget -q -O /dev/null --tries=1 "$extra" "$url" 2>/dev/null; then
      FAIL=$((FAIL + 1))
      FAILURES+=("$label")
      echo "FAIL: $label ($url)"
    else
      echo "OK:   $label"
    fi
  else
    if ! timeout "$TIMEOUT" wget -q -O /dev/null --tries=1 "$url" 2>/dev/null; then
      FAIL=$((FAIL + 1))
      FAILURES+=("$label")
      echo "FAIL: $label ($url)"
    else
      echo "OK:   $label"
    fi
  fi
}

echo "Running safe, read-only health checks..."

# Caddy
check_http "caddy-metrics" "http://127.0.0.1:2019/metrics"

# rclone
check_http "rclone-http" "http://127.0.0.1:9933/"

# handoff (uses public health endpoint)
check_http "handoff" "http://$HOST:9944/health"

# postgres readiness (if pg_isready is available)
if command -v pg_isready >/dev/null 2>&1; then
  if timeout "$TIMEOUT" pg_isready -U postgres -d comet -h "$HOST"; then
    echo "OK:   postgres-readiness"
  else
    FAIL=$((FAIL + 1))
    FAILURES+=("postgres-readiness")
    echo "FAIL: postgres-readiness"
  fi
else
  echo "SKIP: postgres-readiness (pg_isready not installed)"
fi

# comet
check_http "comet" "http://$HOST:8000/"

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "Health checks failed:"
  for f in "${FAILURES[@]}"; do
    echo "  - $f"
  done
  exit 1
fi

echo "All health checks passed."
exit 0
