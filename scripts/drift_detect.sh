#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SNAP_DIR="$PROJECT/.snapshots"

if [ ! -f "$SNAP_DIR/.last" ]; then
  echo "No snapshot found. Run: scripts/snapshot.sh"
  exit 0
fi

LAST_SNAP=$(cat "$SNAP_DIR/.last")

if [ ! -d "$LAST_SNAP" ]; then
  echo "Last snapshot directory missing: $LAST_SNAP"
  exit 1
fi

FILES=(
  "compose/docker-compose.yml"
  "compose/.env"
  "caddy/Caddyfile"
  "rclone/rclone.conf"
  "handoff/src/main.go"
  "handoff/src/Dockerfile"
)

DRIFT=false

echo "Drift detection vs snapshot: $LAST_SNAP"

for f in "${FILES[@]}"; do
  snap_file="$LAST_SNAP/$f"
  live_file="$PROJECT/$f"

  if [ ! -f "$snap_file" ]; then
    continue
  fi

  if [ ! -f "$live_file" ]; then
    echo "DRIFT: $f missing on disk (exists in snapshot)"
    DRIFT=true
    continue
  fi

  if ! diff -q "$snap_file" "$live_file" >/dev/null 2>&1; then
    echo "DRIFT: $f changed since snapshot"
    DRIFT=true
  fi
done

if [ "$DRIFT" = "true" ]; then
  echo ""
  echo "Config drift detected. Consider:"
  echo "  - Review: git diff"
  echo "  - Restore: scripts/rollback.sh"
  exit 1
else
  echo "No drift detected."
  exit 0
fi
