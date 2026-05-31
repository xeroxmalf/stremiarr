#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

if [ ! -f "$COMPOSE" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

if ! command -v trivy >/dev/null 2>&1; then
  echo "Trivy not installed; skipping security scan"
  exit 0
fi

echo "Running Trivy security scan on Docker images..."

# 1) Extract images from docker-compose.yml
IMAGES=$(grep -E "^\s+image:" "$COMPOSE" | awk '{print $2}' | xargs)

# 2) Scan each image for vulnerabilities
for img in $IMAGES; do
  echo "Scanning: $img"
  if ! trivy image --security-cfg /dev/null --ignore-unfixed --exit-code 1 "$img" >/dev/null 2>&1; then
    echo "WARN: $img has vulnerabilities (run Trivy locally for details)"
  else
    echo "OK: $img"
  fi
done

echo "Security scan complete."
