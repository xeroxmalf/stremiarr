#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SNAP_DIR="$PROJECT/.snapshots"
mkdir -p "$SNAP_DIR"

# Critical config files to track
FILES=(
  "compose/docker-compose.yml"
  "compose/.env"
  "caddy/Caddyfile"
  "rclone/rclone.conf"
  "handoff/src/main.go"
  "handoff/src/Dockerfile"
)

TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
SNAP="$SNAP_DIR/$TIMESTAMP"
mkdir -p "$SNAP"

echo "Creating snapshot: $SNAP"

for f in "${FILES[@]}"; do
  src="$PROJECT/$f"
  dst="$SNAP/$f"
  if [ -f "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src" "$dst"
    echo "  - $f"
  else
    echo "  - $f (not found, skipped)"
  fi
done

# Update pointer file (for drift_detect/rollback)
echo "$SNAP" > "$SNAP_DIR/.last"

echo "Snapshot saved to $SNAP"
