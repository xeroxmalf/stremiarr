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

# 1) Ensure no service runs as root without good reason
if grep -E 'user:\s*root' "$COMPOSE"; then
  echo "WARN: A service runs as root (check if necessary)"
fi

# 2) Ensure no service uses privileged: true (except if required)
if grep -E 'privileged:\s*true' "$COMPOSE"; then
  echo "WARN: A service uses privileged mode (may be risky)"
fi

# 3) Ensure no suspiciously wide network_mode (host is expected, but warn if added randomly)
count=$(grep -cE 'network_mode:\s*host' "$COMPOSE")
if [ "$count" -gt 3 ]; then
  echo "WARN: Many services use host network (count=$count)"
fi

# 4) Ensure no hardcoded tokens (simple heuristic)
if grep -iE '(token|apikey|api_key|password|secret)' "$COMPOSE" | grep -v -E '(\$\{|#)' | grep -v -E '^\s*#' | grep -q .; then
  echo "WARN: Potential hardcoded secret/token in docker-compose.yml"
fi

# 5) Ensure restart policies are set (avoid accidental restart: "no")
while IFS= read -r line; do
  if echo "$line" | grep -qE "restart:\s*(no|\"\")|restart:\s*none"; then
    fail "Service uses restart: no/none in production"
  fi
done < "$COMPOSE"

# 6) Ensure no suspiciously wide volume mounts (e.g., / or /etc)
if grep -E '^\s+-\s+/(?!docker)' "$COMPOSE" | grep -v -E '^\s*#'; then
  echo "WARN: Very wide host volume mount detected (e.g., /) in docker-compose.yml"
fi

# 7) Ensure no service exposes sensitive ports to the internet (we only validate configs, not actual firewall)
if grep -E 'expose:\s*(22|23|3389)' "$COMPOSE"; then
  echo "WARN: SSH/RDP port exposed (ensure it's not internet-facing)"
fi

# 8) Ensure logging is configured for key services (to avoid unbounded logs)
for svc in caddy rclone handoff comet postgres; do
  block=$(sed -n "/^  $svc:/,/^[a-z]/p" "$COMPOSE")
  if ! echo "$block" | grep -qE 'logging:'; then
    echo "WARN: No logging config for $svc (risk of unbounded logs)"
  fi
done

# 9) Ensure stop_grace_period is set for critical services
for svc in caddy rclone handoff comet postgres; do
  block=$(sed -n "/^  $svc:/,/^[a-z]/p" "$COMPOSE")
  if ! echo "$block" | grep -qE 'stop_grace_period:'; then
    echo "WARN: No stop_grace_period set for $svc (risk of abrupt termination)"
  fi
done

echo "Security/hardening checks passed"
exit 0
