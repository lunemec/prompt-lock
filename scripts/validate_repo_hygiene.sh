#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Patterns that should never be committed or left in repo tree.
# Keep implementation portable across GNU/BSD find.
matches=$(find . -path './.git' -prune -o -type f -print | grep -E '(\.syncthing\.|sync-conflict|\.DS_Store$|~$|\.tmp$)' || true)

if [[ -n "${matches}" ]]; then
  echo "Repository hygiene check failed. Forbidden artifacts found:" >&2
  echo "${matches}" >&2
  exit 1
fi

echo "Repository hygiene check passed."
