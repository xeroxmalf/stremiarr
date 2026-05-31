#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

if [ ! -f "$COMPOSE" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Extract all ports from docker-compose.yml
# Look for:
#   - "ports:" entries (host:container)
#   - Environment vars with PORT, HOST, etc.
#   - Healthcheck URLs

declare -A PORT_MAP

# Helper: check if port is used
check_port() {
  local port="$1"
  local service="$2"
  if [ -n "${PORT_MAP[$port]+x}" ]; then
    echo "FAIL: Port conflict detected: $port used by ${PORT_MAP[$port]} and $service"
    fail "Port conflict"
  fi
  PORT_MAP[$port]="$service"
}

# Extract ports from healthchecks and env vars
while IFS= read -r line; do
  # Match patterns like :PORT
  for port in $(echo "$line" | grep -oE ':\d{2,5}' | tr -d ':'); do
    # Determine service (previous service line)
    service=$(sed -n "/$line/,$/p" "$COMPOSE" | grep -E "^  [a-z].*:" | head -1 | awk '{print $1}' | tr -d ':')
    [ -n "$service" ] && check_port "$port" "$service"
  done
done < "$COMPOSE"

echo "Port validation passed"
exit 0
