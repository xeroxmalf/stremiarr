#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG_FILE="$PROJECT/logs/health_watch.log"
ALERT_WEBHOOK="${HEALTH_ALERT_WEBHOOK:-}"
mkdir -p "$PROJECT/logs"

ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "[$ts] Starting health watch..." >> "$LOG_FILE"

if ADMIN_PASSWORD="$(grep 'ADMIN_PASSWORD=' "$PROJECT/compose/.env" | cut -d'=' -f2)" \
   "$SCRIPT_DIR/health_checks.sh" >> "$LOG_FILE" 2>&1; then
  echo "[$ts] Health watch: OK" >> "$LOG_FILE"
else
  echo "[$ts] Health watch: FAILURE" >> "$LOG_FILE"

  # Optional webhook alert (Slack/Teams-style POST)
  if [ -n "$ALERT_WEBHOOK" ]; then
    curl -sf -X POST "$ALERT_WEBHOOK" \
      -H "Content-Type: application/json" \
      -d "{
          \"text\": \"[ALERT] $PROJECT health_watch: FAILED at $ts\"
        }" \
      || true
  fi
fi
