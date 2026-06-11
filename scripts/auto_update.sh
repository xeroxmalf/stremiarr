#!/usr/bin/env bash
set -euo pipefail

# This script pulls the latest Stremiarr codebase, rebuilds local containers,
# pulls remote images, and seamlessly restarts any updated services.

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"

echo "🔄 Fetching latest Stremiarr updates from git..."
cd "$PROJECT"
git pull origin main

echo "🔄 Pulling latest external Docker images..."
docker compose -f "$COMPOSE" pull

echo "🏗️ Rebuilding local images (Handoff)..."
docker compose -f "$COMPOSE" build

echo "🚀 Applying updates and restarting changed containers..."
docker compose -f "$COMPOSE" up -d --remove-orphans

echo "🧹 Pruning dangling docker images..."
docker image prune -f

echo "✅ Auto-update complete!"
