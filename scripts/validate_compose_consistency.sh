#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
PRIMARY="$PROJECT/compose/docker-compose.yml"
SECONDARY="$PROJECT/compose/latest.docker-compose.yml"

if [ ! -f "$PRIMARY" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

if [ ! -f "$SECONDARY" ]; then
  echo "latest.docker-compose.yml not found"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Extract service names from both
services_from() {
  grep -E "^  [a-zA-Z0-9_-]+:" "$1" \
    | sed 's/^  \([a-zA-Z0-9_-]*\):.*/\1/' \
    | sort
}

SVC_PRIMARY=$(services_from "$PRIMARY")
SVC_SECONDARY=$(services_from "$SECONDARY")

# 2) Ensure same set of services
if ! diff -q <(echo "$SVC_PRIMARY") <(echo "$SVC_SECONDARY") >/dev/null 2>&1; then
  echo "WARN: Service list mismatch between docker-compose files"
  diff <(echo "$SVC_PRIMARY") <(echo "$SVC_SECONDARY") || true
fi

# 3) Ensure both define the critical services
for svc in caddy rclone handoff postgres comet; do
  if ! echo "$SVC_PRIMARY" | grep -qx "$svc"; then
    fail "Critical service '$svc' missing from docker-compose.yml"
  fi
  if ! echo "$SVC_SECONDARY" | grep -qx "$svc"; then
    fail "Critical service '$svc' missing from latest.docker-compose.yml"
  fi
done

# 4) Ensure images for critical services are consistent (allow minor drift, warn only)
check_image() {
  local svc="$1"
  local primary_img
  primary_img=$(sed -n "/^  $svc:/,/^[a-z]/p" "$PRIMARY" | grep 'image:' | head -1 | awk '{print $2}' | xargs)
  local secondary_img
  secondary_img=$(sed -n "/^  $svc:/,/^[a-z]/p" "$SECONDARY" | grep 'image:' | head -1 | awk '{print $2}' | xargs)

  if [ -n "$primary_img" ] && [ -n "$secondary_img" ] && [ "$primary_img" != "$secondary_img" ]; then
    echo "WARN: Image mismatch for service '$svc': '$primary_img' vs '$secondary_img'"
  fi
}

for svc in caddy rclone postgres comet; do
  check_image "$svc"
done

echo "Compose consistency validation passed"
exit 0
