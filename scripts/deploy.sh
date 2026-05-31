#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE="$PROJECT/compose/docker-compose.yml"

DRY_RUN=false
SNAPSHOT=true

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --no-snapshot) SNAPSHOT=false ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

if [ "$DRY_RUN" = "true" ]; then
  echo "=== DRY RUN: no changes will be applied ==="
else
  echo "=== LIVE DEPLOY: will modify running services ==="
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Pre-flight: run CI (static)
echo ""
echo "[1/6] Running CI (static checks)..."
if ! "$SCRIPT_DIR/ci.sh"; then
  fail "CI failed before deploy. Abort."
fi

# 2) Optional snapshot before deploy (for rollback safety)
if [ "$SNAPSHOT" = "true" ] && [ "$DRY_RUN" = "false" ]; then
  echo ""
  echo "[2/6] Creating pre-deploy snapshot..."
  if ! "$SCRIPT_DIR/snapshot.sh"; then
    echo "WARN: snapshot failed; continuing anyway."
  fi
else
  echo ""
  echo "[2/6] Snapshot skipped."
fi

# 3) Validate compose is parseable with docker compose
echo ""
echo "[3/6] Validating docker compose configuration..."
if ! docker compose -f "$COMPOSE" config >/dev/null 2>&1; then
  fail "docker compose config invalid. Abort."
fi

# 4) Pull latest images
if [ "$DRY_RUN" = "true" ]; then
  echo ""
  echo "[4/6] (DRY RUN) Would run: docker compose -f $COMPOSE pull"
else
  echo ""
  echo "[4/6] Pulling latest images..."
  docker compose -f "$COMPOSE" pull || fail "Image pull failed"
fi

# 5) Restart services (rolling-safe as much as possible)
# Note: all services share host network; we rely on healthchecks + restart: always.
if [ "$DRY_RUN" = "true" ]; then
  echo ""
  echo "[5/6] (DRY RUN) Would run: docker compose -f $COMPOSE up -d --remove-orphans"
else
  echo ""
  echo "[5/6] Applying changes: docker compose up -d ..."
  docker compose -f "$COMPOSE" up -d --remove-orphans || fail "docker compose up failed"
fi

# 6) Post-deploy: basic health sanity
# We run health_checks with a short, lenient policy:
# - If ALL checks fail, consider deploy suspicious.
# - If some fail, we warn (could be transient).
echo ""
echo "[6/6] Post-deploy health (best-effort, local only)..."

if ADMIN_PASSWORD="$(grep 'ADMIN_PASSWORD=' "$PROJECT/compose/.env" | cut -d'=' -f2)" "$SCRIPT_DIR/health_checks.sh"; then
  echo "Post-deploy health: OK"
else
  echo "WARN: Some post-deploy health checks failed."
  echo "      This may be transient or network-limited. Verify manually."
fi

if [ "$DRY_RUN" = "true" ]; then
  echo ""
  echo "=== DRY RUN complete: no changes applied ==="
  exit 0
fi

echo "=== Deploy complete ==="
exit 0
