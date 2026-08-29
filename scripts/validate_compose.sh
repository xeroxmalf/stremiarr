#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

if [ ! -f "$COMPOSE" ]; then
  echo "docker-compose.yml not found at $COMPOSE"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) YAML syntax (use python if available)
if command -v python3 >/dev/null 2>&1; then
  if python3 -c "import yaml" >/dev/null 2>&1; then
    python3 -c "
import yaml, sys
with open('$COMPOSE') as f:
    try:
        yaml.safe_load(f)
    except yaml.YAMLError as e:
        print('YAML parse error:', e, file=sys.stderr)
        sys.exit(1)
" || fail "Invalid YAML in docker-compose.yml"
  else
    echo "WARN: python3 yaml module not found; skipping YAML parse validation"
  fi
else
  echo "WARN: python3 not found; skipping YAML parse validation"
fi

# 2) Check for required services (must be defined)
for svc in caddy rclone handoff postgres comet; do
  grep -qE "^  $svc:" "$COMPOSE" || fail "Missing service: $svc"
done

# 3) Ensure all services use restart: always (or unless-stopped)
# Avoid accidental restart: "no" in production
while IFS= read -r line; do
  # Only lines with restart: at service-level
  if echo "$line" | grep -qE "^\s+restart:\s*(no|\"\")|^\s+restart:\s*none"; then
    fail "Detected restart: no/none in $line"
  fi
done < "$COMPOSE"

# 4) No inline 'rm' or 'truncate' in commands (defensive check)
# Avoid false positives on postgres flags like -c maintenance_work_mem
if grep -E '(rm -rf /|truncate -s |dd if=/dev/zero)' "$COMPOSE" | grep -v -E '^\s*#'; then
  fail "Suspicious destructive command pattern found in docker-compose.yml"
fi

# 5) Ensure no obvious secrets hardcoded (except env references and bcrypt hashes)
# We'll allow:
#   - ${VAR} references
#   - bcrypt-like hashes starting with $2
if grep -E '(password|PASSWORD)' "$COMPOSE" | grep -v -E "(\\\$\\{|\\\$2a|\\\$2b)" | grep -q -v '^#'; then
  # If there's a password line not using env or bcrypt, that's suspect
  # But some lines legitimately include comments, so be lenient and log only if very obvious.
  # We'll treat this as a warning, not a hard fail.
  echo "WARN: Potential hardcoded password (non-env) detected in docker-compose.yml"
fi

# 6) Ensure volumes use absolute paths (not relative)
if grep -E '^\s+- \./|^\s+- \./' "$COMPOSE" | grep -v -E '^\s*#'; then
  echo "WARN: Relative volume path(s) detected in docker-compose.yml"
fi

# 7) Ensure all services use restart policy (not left blank)
while IFS= read -r svc_line; do
  svc_name=$(echo "$svc_line" | sed 's/^\s*//;s/:$//')
  # Extract block until next service or non-indented line
  block=$(sed -n "/^  $svc_name:/,/^[a-z]/p" "$COMPOSE" | head -n -1)
  if ! echo "$block" | grep -qE '^\s+restart:'; then
    fail "Service '$svc_name' is missing restart policy"
  fi
done < <(grep -E "^  [a-z].*:" "$COMPOSE" | grep -E '(caddy|rclone|handoff|postgres|comet):')

echo "docker-compose.yml validation passed"
exit 0
