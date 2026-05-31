#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

LIVE=false
DRIFT_HARD=false

for arg in "$@"; do
  case "$arg" in
    --live)         LIVE=true ;;
    --drift-hard)   DRIFT_HARD=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

PASS=0
FAIL=0
SKIP=0
FAILURES=()

run_test() {
  local name="$1"
  shift
  local output
  if output=$("$@" 2>&1); then
    PASS=$((PASS + 1))
    echo "PASS: $name"
  else
    FAIL=$((FAIL + 1))
    FAILURES+=("$name")
    echo "FAIL: $name"
    # Show last non-empty lines of output for debugging (max 30 lines)
    echo "$output" | grep -v '^\s*$' | tail -n 30 | sed 's/^/  /'
  fi
}

run_test_warn() {
  local name="$1"
  shift
  local output
  if output=$("$@" 2>&1); then
    PASS=$((PASS + 1))
    echo "PASS: $name"
  else
    echo "WARN: $name (non-blocking)"
    # Brief context (up to 10 lines)
    echo "$output" | grep -v '^\s*$' | tail -n 10 | sed 's/^/  /'
  fi
}

skip_test() {
  local name="$1"
  SKIP=$((SKIP + 1))
  echo "SKIP: $name"
}

echo "========================================"
echo " CI Suite for $PROJECT_DIR"
echo "========================================"

# 1) Basic filesystem checks
if [ -f "$PROJECT_DIR/compose/docker-compose.yml" ]; then
  run_test "docker-compose.yml exists" true
else
  run_test "docker-compose.yml exists" false
fi

if [ -f "$PROJECT_DIR/caddy/Caddyfile" ]; then
  run_test "Caddyfile exists" true
else
  run_test "Caddyfile exists" false
fi

if [ -f "$PROJECT_DIR/rclone/rclone.conf" ]; then
  run_test "rclone.conf exists" true
else
  run_test "rclone.conf exists" false
fi

if [ -f "$PROJECT_DIR/handoff/src/main.go" ]; then
  run_test "handoff main.go exists" true
else
  run_test "handoff main.go exists" false
fi

if [ -f "$PROJECT_DIR/compose/.env" ]; then
  run_test ".env exists" true
else
  run_test ".env exists" false
fi

# 2) Compose validation
if [ -x "$SCRIPT_DIR/validate_compose.sh" ]; then
  run_test "Validate docker-compose.yml" "$SCRIPT_DIR/validate_compose.sh" "$PROJECT_DIR"
else
  skip_test "Validate docker-compose.yml (script missing)"
fi

# 3) Caddyfile validation
if [ -x "$SCRIPT_DIR/validate_caddy.sh" ]; then
  run_test "Validate Caddyfile" "$SCRIPT_DIR/validate_caddy.sh" "$PROJECT_DIR"
else
  skip_test "Validate Caddyfile (script missing)"
fi

# 4) Caddyfile upstream/service alignment
if [ -x "$SCRIPT_DIR/validate_caddy_upstreams.sh" ]; then
  run_test "Validate Caddyfile upstream/service alignment" \
    "$SCRIPT_DIR/validate_caddy_upstreams.sh" "$PROJECT_DIR"
else
  skip_test "Validate Caddyfile upstreams (script missing)"
fi

# 5) rclone config validation
if [ -x "$SCRIPT_DIR/validate_rclone.sh" ]; then
  run_test "Validate rclone.conf" "$SCRIPT_DIR/validate_rclone.sh" "$PROJECT_DIR"
else
  skip_test "Validate rclone.conf (script missing)"
fi

# 6) Handoff Go checks
if [ -x "$SCRIPT_DIR/validate_handoff.sh" ]; then
  if command -v go >/dev/null 2>&1; then
    run_test "Validate handoff Go code (build + vet)" \
      "$SCRIPT_DIR/validate_handoff.sh" "$PROJECT_DIR"
  else
    skip_test "Validate handoff Go code (go not installed)"
  fi
else
  skip_test "Validate handoff Go code (script missing)"
fi

# 7) Environment variable consistency
if [ -x "$SCRIPT_DIR/validate_env.sh" ]; then
  run_test "Validate .env vs docker-compose.yml" \
    "$SCRIPT_DIR/validate_env.sh" "$PROJECT_DIR"
else
  skip_test "Validate .env vs docker-compose.yml (script missing)"
fi

# 8) Compose consistency (docker-compose.yml vs latest.docker-compose.yml)
if [ -x "$SCRIPT_DIR/validate_compose_consistency.sh" ]; then
  run_test_warn "Compose consistency (docker-compose vs latest)" \
    "$SCRIPT_DIR/validate_compose_consistency.sh" "$PROJECT_DIR"
else
  skip_test "Compose consistency (script missing)"
fi

# 9) Security & hardening checks
if [ -x "$SCRIPT_DIR/validate_security.sh" ]; then
  run_test "Security/hardening checks" \
    "$SCRIPT_DIR/validate_security.sh" "$PROJECT_DIR"
else
  skip_test "Security/hardening checks (script missing)"
fi

# 10) Port conflict detection
if [ -x "$SCRIPT_DIR/validate_ports.sh" ]; then
  run_test_warn "Port conflict detection" \
    "$SCRIPT_DIR/validate_ports.sh" "$PROJECT_DIR"
else
  skip_test "Port conflict detection (script missing)"
fi

# 11) TLS/cert validation
if [ -x "$SCRIPT_DIR/validate_tls.sh" ]; then
  run_test "TLS/cert validation" \
    "$SCRIPT_DIR/validate_tls.sh" "$PROJECT_DIR"
else
  skip_test "TLS/cert validation (script missing)"
fi

# 12) Config drift detection (if snapshot exists)
if [ -x "$SCRIPT_DIR/drift_detect.sh" ]; then
  if [ "$DRIFT_HARD" = "true" ]; then
    run_test "Config drift detection (hard)" \
      "$SCRIPT_DIR/drift_detect.sh" "$PROJECT_DIR"
  else
    run_test_warn "Config drift detection (warn)" \
      "$SCRIPT_DIR/drift_detect.sh" "$PROJECT_DIR"
  fi
else
  skip_test "Config drift detection (script missing)"
fi

# 13) Optional: Trivy security scan (if installed)
if [ -x "$SCRIPT_DIR/scan_security.sh" ] && command -v trivy >/dev/null 2>&1; then
  run_test_warn "Trivy security scan" \
    "$SCRIPT_DIR/scan_security.sh" "$PROJECT_DIR"
else
  skip_test "Trivy security scan (Trivy not installed)"
fi

# 14) Optional live health checks
if [ "$LIVE" = "true" ] && [ -x "$SCRIPT_DIR/health_checks.sh" ]; then
  echo ""
  echo "========================================"
  echo " LIVE Health Checks (--live enabled)"
  echo "========================================"
  run_test "Live health checks (safe GET-only)" \
    "$SCRIPT_DIR/health_checks.sh"
fi

echo ""
echo "========================================"
echo " Summary"
echo "========================================"
echo "PASS: $PASS  FAIL: $FAIL  SKIP: $SKIP"

if [ ${#FAILURES[@]} -gt 0 ]; then
  echo ""
  echo "Failed tests:"
  for t in "${FAILURES[@]}"; do
    echo "  - $t"
  done
  exit 1
fi

exit 0
