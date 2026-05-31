#!/usr/bin/env bash
# Stremiarr Consolidated Logging
# Tails logs from all services in the stack.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR/compose"

echo "================================================================================"
echo " 📜 Stremiarr Consolidated Logs (All Services)"
echo " Press Ctrl+C to stop."
echo "================================================================================"

docker compose logs --follow --tail 100
