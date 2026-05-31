#!/usr/bin/env bash
set -euo pipefail

PROJECT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
SRC_DIR="$PROJECT/handoff/src"

if [ ! -d "$SRC_DIR" ]; then
  echo "handoff/src not found at $SRC_DIR"
  exit 1
fi

if [ ! -f "$SRC_DIR/main.go" ]; then
  echo "handoff/src/main.go not found"
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "FAIL: go not installed"
  exit 1
fi

# 1) Ensure go.mod exists; if not, attempt init (non-destructive in CI context)
cd "$SRC_DIR"

if [ ! -f "go.mod" ]; then
  echo "go.mod missing; initializing..."
  go mod init handoff || { echo "FAIL: go mod init failed"; exit 1; }
fi

# 2) Tidy dependencies
go mod tidy || { echo "FAIL: go mod tidy failed"; exit 1; }

# 3) Vet
if ! go vet ./...; then
  echo "FAIL: go vet reported issues"
  exit 1
fi

# 4) Build
if ! go build -o /dev/null ./...; then
  echo "FAIL: go build failed"
  exit 1
fi

# 5) Check Dockerfile is present and references a reasonable Go version
DOCKERFILE="$SRC_DIR/Dockerfile"
if [ ! -f "$DOCKERFILE" ]; then
  echo "FAIL: Dockerfile missing in handoff/src"
  exit 1
fi

if ! grep -qiE "golang:1\.(2[0-9]|3[0-9])" "$DOCKERFILE"; then
  echo "WARN: Dockerfile does not pin a recent Go version (>=1.20)"
fi

echo "Handoff Go validation passed"
exit 0
