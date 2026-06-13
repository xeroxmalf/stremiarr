#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-.}"
COMPOSE="$PROJECT/compose/docker-compose.yml"
CADDYFILE="$PROJECT/caddy/Caddyfile"

if [ ! -f "$CADDYFILE" ]; then
  echo "Caddyfile not found"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Ensure ACME is configured
if ! grep -q "ACME_AGREE=true" "$COMPOSE"; then
  fail "ACME_AGREE not set to true in docker-compose.yml"
fi

# 2) Ensure Cloudflare DNS challenge is used
if ! grep -q "cloudflare" "$CADDYFILE"; then
  fail "Cloudflare DNS challenge not configured in Caddyfile"
fi

# 3) Ensure TLS snippets exist and are imported
for prefix in "stl" "comet"; do
  block=$(awk "/https?:\/\/$prefix\.[a-z0-9.-]+/,/^[}]/" "$CADDYFILE")
  if ! echo "$block" | grep -qE "import cloudflare_auth"; then
    echo "WARN: $prefix site not importing cloudflare_auth (TLS/DNS issue)"
  fi
done

echo "TLS/cert validation passed"
exit 0
