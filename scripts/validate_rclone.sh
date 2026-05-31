#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
CONF="$PROJECT/rclone/rclone.conf"

if [ ! -f "$CONF" ]; then
  echo "rclone.conf not found at $CONF"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Ensure real-debrid-dav remote exists
if ! grep -q '^\[real-debrid-dav\]' "$CONF"; then
  fail "Missing [real-debrid-dav] remote in rclone.conf"
fi

# 2) Ensure type=webdav
if ! grep -A5 '^\[real-debrid-dav\]' "$CONF" | grep -qE 'type\s*=\s*webdav'; then
  fail "real-debrid-dav not configured as WebDAV"
fi

# 3) Ensure URL points to dav.real-debrid.com
if ! grep -A5 '^\[real-debrid-dav\]' "$CONF" | grep -qE 'dav\.real-debrid\.com'; then
  fail "real-debrid-dav does not point to dav.real-debrid.com"
fi

# 4) Ensure at least one real-debrid remote exists
if ! grep -q '^\[real-debrid\]' "$CONF"; then
  fail "Missing [real-debrid] remote in rclone.conf"
fi

# 5) Optional: if rclone is available, validate remotes list
if command -v rclone >/dev/null 2>&1; then
  if RCLONE_CONFIG="$CONF" rclone listremotes >/dev/null 2>&1; then
    echo "rclone listremotes OK"
  else
    echo "WARN: rclone listremotes failed; skipping runtime validation"
  fi
fi

echo "rclone.conf validation passed"
exit 0
