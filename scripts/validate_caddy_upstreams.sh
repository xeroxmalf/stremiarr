#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
CADDYFILE="$PROJECT/caddy/Caddyfile"
COMPOSE="$PROJECT/compose/docker-compose.yml"

if [ ! -f "$CADDYFILE" ]; then
  echo "Caddyfile not found"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Extract canonical upstream ports from docker-compose.yml healthchecks/env.
# These are what we expect Caddy to proxy to.

# comet main port (from FASTAPI_PORT)
COMET_PORT=$(grep 'FASTAPI_PORT=' "$COMPOSE" | head -1 | cut -d'=' -f2 | xargs)
[ -z "$COMET_PORT" ] && COMET_PORT=8000

# handoff port (from PORT)
HANDOFF_PORT=$(grep 'PORT=' "$COMPOSE" | head -1 | cut -d'=' -f2 | xargs)
[ -z "$HANDOFF_PORT" ] && HANDOFF_PORT=9944

# CometNet WS port (from healthcheck / common configs)
COMETNET_PORT=8765

HOST="127.0.0.1"

# 2) Check stl.defnotmy.site -> handoff
block=$(awk "/https?:\/\/stl\.defnotmy\.site/,/^[}]/" "$CADDYFILE")
if ! echo "$block" | grep -qE "reverse_proxy.*$HOST:$HANDOFF_PORT"; then
  fail "stl.defnotmy.site not reverse proxying to handoff on port $HANDOFF_PORT"
fi

# 3) Check llama.defnotmy.site -> llama backend (expected on 127.0.0.1:8080)
block=$(awk "/https?:\/\/llama\.defnotmy\.site/,/^[}]/" "$CADDYFILE")
if ! echo "$block" | grep -qE "reverse_proxy.*127\.0\.0\.1:8080"; then
  echo "WARN: llama.defnotmy.site upstream is not 127.0.0.1:8080"
fi

# 4) Check comet.defnotmy.site -> comet main
block=$(awk "/https?:\/\/comet\.defnotmy\.site/,/^[}]/" "$CADDYFILE")
if ! echo "$block" | grep -qE "reverse_proxy.*$HOST:$COMET_PORT"; then
  fail "comet.defnotmy.site not reverse proxying to comet on port $COMET_PORT"
fi

# 5) Check cometNet WS route
if ! echo "$block" | grep -qE "/cometnet/ws.*$HOST:$COMETNET_PORT"; then
  echo "WARN: comet.defnotmy.site cometNet WS route not reverse proxying to $COMETNET_PORT"
fi

echo "Caddyfile upstream/service alignment passed"
exit 0
