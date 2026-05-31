#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="$PROJECT/.backups/$(date -u +"%Y%m%dT%H%M%SZ")"
mkdir -p "$BACKUP_DIR"

echo "Starting backup to: $BACKUP_DIR"

# 1) Backup postgres data (if pg_dump available)
if command -v pg_dump >/dev/null 2>&1; then
  echo "Backing up postgres database..."
  if pg_dump -U postgres -h 127.0.0.1 -d comet \
    | gzip > "$BACKUP_DIR/postgres-comet.sql.gz"; then
    # Verify backup is not empty
    if [ -s "$BACKUP_DIR/postgres-comet.sql.gz" ]; then
      echo "  - postgres-comet.sql.gz (Verified)"
    else
      echo "  ❌ ERROR: postgres backup is empty!"
    fi
  else
    echo "  ❌ ERROR: pg_dump failed!"
  fi
else
  echo "WARN: pg_dump not installed; skipping postgres backup"
fi

# 2) Backup comet data (state, config, etc.)
if [ -d "$PROJECT/comet" ]; then
  echo "Backing up comet data..."
  cp -r "$PROJECT/comet" "$BACKUP_DIR/comet"
  echo "  - comet/"
fi

# 3) Backup handoff data (streams.db, mappings.json, etc.)
if [ -d "$PROJECT/handoff/data" ]; then
  echo "Backing up handoff data..."
  cp -r "$PROJECT/handoff/data" "$BACKUP_DIR/handoff-data"
  echo "  - handoff-data/"
fi

# 4) Backup configurations
echo "Backing up core configurations..."
cp "$PROJECT/caddy/Caddyfile" "$BACKUP_DIR/"
cp "$PROJECT/rclone/rclone.conf" "$BACKUP_DIR/"
cp "$PROJECT/compose/docker-compose.yml" "$BACKUP_DIR/"
cp "$PROJECT/compose/.env" "$BACKUP_DIR/"
echo "  - Caddyfile, rclone.conf, docker-compose.yml, .env"

# 5) Backup scripts
echo "Backing up operational scripts..."
cp -r "$PROJECT/scripts" "$BACKUP_DIR/scripts"
echo "  - scripts/"

echo "Backup complete: $BACKUP_DIR"
