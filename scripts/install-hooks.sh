#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
HOOKS_DIR="$PROJECT/.git/hooks"

if [ ! -d "$HOOKS_DIR" ]; then
  echo "No .git/hooks found; not a git repository or hooks dir missing."
  exit 1
fi

HOOK_FILE="$HOOKS_DIR/pre-commit"

echo "Installing pre-commit hook to $HOOK_FILE..."

cat > "$HOOK_FILE" << 'EOF'
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/../scripts" && pwd)"
CI_SCRIPT="$SCRIPT_DIR/ci.sh"

if [ ! -x "$CI_SCRIPT" ]; then
  echo "Pre-commit: ci.sh not found or not executable. Skipping."
  exit 0
fi

echo "Pre-commit: running CI checks (static only)..."

if ! "$CI_SCRIPT"; then
  echo "Pre-commit: CI checks failed. Blocking commit."
  exit 1
fi

echo "Pre-commit: CI checks passed."
exit 0
EOF

chmod +x "$HOOK_FILE"
echo "Pre-commit hook installed."
