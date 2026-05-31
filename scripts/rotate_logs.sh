#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
LOG_DIR="$PROJECT/logs"
MAX_DAYS=14

if [ ! -d "$LOG_DIR" ]; then
  exit 0
fi

# Remove logs older than MAX_DAYS
echo "Cleaning up logs older than $MAX_DAYS days..."
find "$LOG_DIR" -type f -name "*.log" -mtime +"$MAX_DAYS" -print -delete

# Truncate large logs over 100MB
echo "Truncating logs larger than 100MB..."
find "$LOG_DIR" -type f -name "*.log" -size +100M -print -exec truncate -s 0 {} +
