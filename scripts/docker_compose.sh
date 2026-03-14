#!/usr/bin/env bash
set -euo pipefail

if docker compose version >/dev/null 2>&1; then
  if ! docker info >/dev/null 2>&1; then
    echo "ERROR: docker daemon is not reachable; start docker before running compose workflows" >&2
    exit 1
  fi
  exec docker compose "$@"
fi

if command -v docker-compose >/dev/null 2>&1; then
  if ! docker info >/dev/null 2>&1; then
    echo "ERROR: docker daemon is not reachable; start docker before running compose workflows" >&2
    exit 1
  fi
  exec docker-compose "$@"
fi

echo "ERROR: docker compose plugin or docker-compose binary is required" >&2
exit 1
