#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-/docker-configs}"
COMPOSE="$PROJECT/compose/docker-compose.yml"
ENVFILE="$PROJECT/compose/.env"

if [ ! -f "$COMPOSE" ]; then
  echo "docker-compose.yml not found"
  exit 1
fi

if [ ! -f "$ENVFILE" ]; then
  echo ".env not found"
  exit 1
fi

fail() {
  echo "FAIL: $1"
  exit 1
}

# 1) Extract all ${VAR} from docker-compose.yml
REQ_VARS=$(grep -oE '\$\{[A-Z_][A-Z0-9_]*\}' "$COMPOSE" \
  | sed 's/\${//;s/}//' \
  | sort -u)

# 2) Load .env into an associative array
declare -A ENV
while IFS= read -r line; do
  # ignore comments and blanks
  [[ "$line" =~ ^[[:space:]]*# ]] && continue
  [[ -z "$line" ]] && continue
  if [[ "$line" =~ ^([A-Z_][A-Z0-9_]*)= ]]; then
    key="${BASH_REMATCH[1]}"
    # value after first =, preserve quotes
    val="${line#*=}"
    ENV["$key"]="$val"
  fi
done < "$ENVFILE"

# 3) For each required var, check that it's defined and non-empty
for var in $REQ_VARS; do
  if [ -z "${ENV[$var]+x}" ]; then
    echo "WARN: Variable $var is used in docker-compose.yml but missing from .env"
    continue
  fi

  val="${ENV[$var]}"
  # strip leading/trailing quotes (single or double)
  clean_val=$(echo "$val" | sed 's/^[\"'\''"]//;s/[\"'\''"]$//')
  if [ -z "$clean_val" ]; then
    echo "WARN: Variable $var is defined in .env but appears empty"
  fi
done

# 4) Hard requirements: ensure specific secrets are non-empty
for secret in \
    "CF_API_TOKEN" \
    "CF_EMAIL" \
    "RD_API_KEY" \
    "POSTGRES_PASS" \
    "COMET_ADMIN_PASS" \
    "ADMIN_PASSWORD" \
    "RCLONE_URL" \
    "RCLONE_RC_URL"; do
  if [ -z "${ENV[$secret]+x}" ]; then
    fail "Critical secret $secret is missing from .env"
  fi
  val="${ENV[$secret]}"
  clean_val=$(echo "$val" | sed "s/^[\"']//;s/[\"']$//")
  if [ -z "$clean_val" ]; then
    fail "Critical secret $secret is empty in .env"
  fi
done

# 5) Detect obviously leaked secrets in comments (simple heuristic)
if grep -n -E '(RD_API_KEY|CF_API_TOKEN)=\S+' "$COMPOSE" | grep -vE '(\$\{|#)' | grep -q .; then
  fail "Possible hardcoded secret in docker-compose.yml (not using env reference)"
fi

echo ".env vs docker-compose.yml validation passed"
exit 0
