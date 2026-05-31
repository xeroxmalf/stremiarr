#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="$PROJECT/.backups"

KEEP_DAYS=14
KEEP_COUNT=7

if [ ! -d "$BACKUP_DIR" ]; then
  exit 0
fi

# Remove backups older than KEEP_DAYS
find "$BACKUP_DIR" -maxdepth 1 -mindepth 1 -type d -mtime +"$KEEP_DAYS" -exec rm -rf {} +

# If there are still more than KEEP_COUNT backups, remove oldest
remaining=$(find "$BACKUP_DIR" -maxdepth 1 -mindepth 1 -type d | sort | wc -l)
if [ "$remaining" -gt "$KEEP_COUNT" ]; then
  remove=$((remaining - KEEP_COUNT))
  find "$BACKUP_DIR" -maxdepth 1 -mindepth 1 -type d | sort | head -n "$remove" | xargs rm -rf
fi
