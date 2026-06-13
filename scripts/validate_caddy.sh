#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-/docker-configs}"
CADDYFILE="$PROJECT/caddy/Caddyfile"

if [ ! -f "$CADDYFILE" ]; then
  echo "Caddyfile not found at $CADDYFILE"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Basic syntax: no obvious Caddy parse killers (unbalanced braces)
OPEN=$(grep -o '{' "$CADDYFILE" | wc -l)
CLOSE=$(grep -o '}' "$CADDYFILE" | wc -l)
if [ "$OPEN" -ne "$CLOSE" ]; then
  fail "Unbalanced braces in Caddyfile (open=$OPEN close=$CLOSE)"
fi

# 2) Ensure we have key site blocks
for prefix in "stl" "comet"; do
  if ! grep -qE "https?://$prefix\.[a-z0-9.-]+" "$CADDYFILE"; then
    fail "Missing site block for $prefix"
  fi
done

# 3) Ensure Cloudflare DNS module is referenced
if ! grep -qi "cloudflare" "$CADDYFILE"; then
  fail "No reference to Cloudflare DNS in Caddyfile"
fi

# 4) Ensure reverse_proxy directives exist for known sites
for prefix in "stl" "comet"; do
  # Extract the site block and check for reverse_proxy
  block=$(awk "/https?:\/\/$prefix\.[a-z0-9.-]+/,/^[}]/" "$CADDYFILE")
  if ! echo "$block" | grep -qE "reverse_proxy"; then
    fail "No reverse_proxy found for $prefix"
  fi
done

# 5) Ensure no obviously external upstream (block accidental reverse proxy to non-internal IPs)
# Allowed:
#   - 192.168.x.x
#   - 10.x.x.x
#   - 127.0.0.1
# We'll treat any other IP/domain in reverse_proxy as a warning (not fail).
while IFS= read -r line; do
  # Extract first token after reverse_proxy
  target=$(echo "$line" | awk '{print $2}')
  if echo "$target" | grep -qE '^\d+\.\d+\.\d+\.\d+'; then
    if ! echo "$target" | grep -qE '^(192\.168\.|10\.|127\.0\.0\.)'; then
      echo "WARN: Unexpected upstream IP in reverse_proxy: $target"
    fi
  elif echo "$target" | grep -qE '^[a-zA-Z0-9.-]+\.[a-z]{2,}'; then
    echo "WARN: Domain used in reverse_proxy upstream: $target"
  fi
done < <(grep -E "reverse_proxy" "$CADDYFILE" | grep -v "^#")

# 6) Ensure we have streaming optimization imported in stl/comet
for prefix in "stl" "comet"; do
  block=$(awk "/https?:\/\/$prefix\.[a-z0-9.-]+/,/^[}]/" "$CADDYFILE")
  if ! echo "$block" | grep -qE "import streaming_optimization|keepalive|flush_interval"; then
    echo "WARN: Streaming optimization not clearly applied to $prefix"
  fi
done

echo "Caddyfile validation passed"
exit 0
