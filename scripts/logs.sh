#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
COMPOSE="$PROJECT/compose/docker-compose.yml"
SERVICE="${2:-}"
FOLLOW="${3:-false}"

if [ -z "$SERVICE" ]; then
  echo "Usage: ./scripts/logs.sh [service] [--follow]"
  echo "Examples:"
  echo "  ./scripts/logs.sh caddy"
  echo "  ./scripts/logs.sh handoff --follow"
  exit 0
fi

if [ "$FOLLOW" = "--follow" ] || [ "$FOLLOW" = "-f" ]; then
  docker compose -f "$COMPOSE" logs -f --tail=200 "$SERVICE"
else
  docker compose -f "$COMPOSE" logs --tail=200 "$SERVICE"
fi
