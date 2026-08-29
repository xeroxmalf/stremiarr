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
  if echo "$line" | grep -qE ':\d{2,5}'; then
    for port in $(echo "$line" | grep -oE ':\d{2,5}' | tr -d ':'); do
      # We cannot grep the file while reading it line-by-line using < "$COMPOSE" because it is bad practice (SC2094).
      # The line doesn't give us the service easily unless we track the current service block.
      [ -n "$current_service" ] && check_port "$port" "$current_service"
    done
  fi
  # Track current service
  if echo "$line" | grep -qE "^  [a-z].*:"; then
    current_service=$(echo "$line" | awk '{print $1}' | tr -d ':')
  fi
done < "$COMPOSE"

echo "Port validation passed"
exit 0
