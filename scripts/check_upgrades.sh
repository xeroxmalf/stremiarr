#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

if [ ! -f "$COMPOSE" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

echo "Checking for available Docker image updates..."

# 1) Extract images from docker-compose.yml
IMAGES=$(grep -E "^\s+image:" "$COMPOSE" | awk '{print $2}' | xargs)

# 2) For each image, check if it has updates
for img in $IMAGES; do
  # Skip if image is not a standard Docker image (e.g., local builds)
  if echo "$img" | grep -qE "latest$"; then
    # For latest tags, we can't easily check; skip
    echo "SKIP: $img (latest tag, cannot check)"
  else
    # Use docker pull --quiet to check if newer version exists
    if docker pull --quiet "$img" >/dev/null 2>&1; then
      echo "OK: $img (up to date)"
    else
      echo "WARN: Failed to check $img (network or auth issue?)"
    fi
  fi
done

echo "Upgrade check complete."
