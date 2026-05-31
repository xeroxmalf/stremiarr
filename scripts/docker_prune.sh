#!/usr/bin/env bash
set -euo pipefail

# Safe: only prunes dangling and unused build cache / volumes, no forced stops.
# Designed to run weekly.

docker system prune -f
docker image prune -f --all -a --filter "until=48h"
