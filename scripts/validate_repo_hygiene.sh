#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Patterns that should never be committed or left in repo tree.
PATTERN='(\.syncthing\.|sync-conflict|\.DS_Store$|~$|\.tmp$)'

matches=$(find . \
  -path './.git' -prune -o \
  -type f -regextype posix-extended -regex ".*${PATTERN}.*" -print)

if [[ -n "${matches}" ]]; then
  echo "Repository hygiene check failed. Forbidden artifacts found:" >&2
  echo "${matches}" >&2
  exit 1
fi

echo "Repository hygiene check passed."
