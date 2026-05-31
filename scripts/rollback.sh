#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SNAP_DIR="$PROJECT/.snapshots"
BACKUP_DIR="$SNAP_DIR/.rollback-backup-$(date -u +"%Y%m%dT%H%M%SZ")"

if [ ! -f "$SNAP_DIR/.last" ]; then
  echo "No snapshot found. Run: scripts/snapshot.sh"
  exit 1
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

echo "Proposed rollback source: $LAST_SNAP"
echo ""
echo "Files that would be restored:"

RESTORE_COUNT=0
for f in "${FILES[@]}"; do
  snap_file="$LAST_SNAP/$f"
  live_file="$PROJECT/$f"

  if [ ! -f "$snap_file" ]; then
    continue
  fi

  if [ -f "$live_file" ] && diff -q "$snap_file" "$live_file" >/dev/null 2>&1; then
    echo "  - $f (unchanged, no restore)"
    continue
  fi

  echo "  - $f (will be overwritten)"
  RESTORE_COUNT=$((RESTORE_COUNT + 1))
done

if [ "$RESTORE_COUNT" -eq 0 ]; then
  echo "Nothing to restore."
  exit 0
fi

echo ""
echo "WARNING: This will overwrite live configs."
echo "A backup of current files will be created at:"
echo "  $BACKUP_DIR"
echo ""

# Ask for confirmation
read -rp "Continue with rollback? (type YES to confirm): " confirm
if [ "$confirm" != "YES" ]; then
  echo "Rollback aborted."
  exit 0
fi

# Backup current configs
mkdir -p "$BACKUP_DIR"

for f in "${FILES[@]}"; do
  live_file="$PROJECT/$f"
  if [ -f "$live_file" ]; then
    dest="$BACKUP_DIR/$f"
    mkdir -p "$(dirname "$dest")"
    cp "$live_file" "$dest"
  fi
done

echo "Backup created at $BACKUP_DIR"

# Apply rollback
for f in "${FILES[@]}"; do
  snap_file="$LAST_SNAP/$f"
  live_file="$PROJECT/$f"

  if [ ! -f "$snap_file" ]; then
    continue
  fi

  mkdir -p "$(dirname "$live_file")"
  cp "$snap_file" "$live_file"
  echo "Restored: $f"
done

echo "Rollback complete."
