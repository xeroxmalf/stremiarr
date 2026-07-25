#!/usr/bin/env bash
set -euo pipefail

# Safe: prunes dangling images, stopped containers, unused networks, and build cache.
# Designed to run weekly via cron.

echo "🧹 Pruning dangling Docker resources..."
docker system prune -f

echo "🧹 Removing unused images older than 48 hours..."
docker image prune -f --filter "until=48h"

echo "🧹 Cleaning build cache older than 7 days..."
docker builder prune -f --filter "until=168h"

echo "✅ Docker cleanup complete."
