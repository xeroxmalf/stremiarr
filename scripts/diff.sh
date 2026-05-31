#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
PRIMARY="$PROJECT/compose/docker-compose.yml"
SECONDARY="$PROJECT/compose/latest.docker-compose.yml"

if [ ! -f "$PRIMARY" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

if [ ! -f "$SECONDARY" ]; then
  echo "latest.docker-compose.yml not found"
  exit 1
fi

echo "=== Diff: docker-compose.yml vs latest.docker-compose.yml ==="
diff -u "$SECONDARY" "$PRIMARY" || true
echo "=== End of diff ==="
